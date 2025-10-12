package manifest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cmmoran/swarmcp/internal/render"
	"github.com/cmmoran/swarmcp/internal/renderx"
	"github.com/cmmoran/swarmcp/internal/spec"
	"github.com/cmmoran/swarmcp/internal/store"
)

// Renderer kept for compatibility with callers that supply their own renderer.
type Renderer interface {
	RenderTemplateString(name, tpl string, data map[string]any, secretMarker ...any) (string, error)
	RenderFile(path string, data map[string]any, secretMarker ...any) ([]byte, error)
}

func LoadProject(root string) (*spec.Project, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(abs, "project.yaml"))
	if err != nil {
		return nil, err
	}
	var p spec.Project
	if err = yaml.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	if strings.ToLower(p.Kind) != "project" {
		return nil, fmt.Errorf("not a Project kind")
	}
	p.Root = abs
	return &p, nil
}

func LoadStack(dir string) (*spec.Stack, error) {
	b, err := os.ReadFile(filepath.Join(dir, "stack.yaml"))
	if err != nil {
		return nil, err
	}
	var s spec.Stack
	if err = yaml.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	s.Dir = dir
	return &s, nil
}

func LoadService(dir string) (*spec.Service, error) {
	b, err := os.ReadFile(filepath.Join(dir, "service.yaml"))
	if err != nil {
		return nil, err
	}
	var svc spec.Service
	if err = yaml.Unmarshal(b, &svc); err != nil {
		return nil, err
	}
	svc.Dir = dir
	return &svc, nil
}

// ResolveEffective builds the fully merged effective model, renders config templates,
// and resolves SecretsProvider secrets (if vault is configured in the project).
func ResolveEffective(ctx context.Context, p *spec.Project, _ Renderer) (*spec.EffectiveProject, error) {
	if p == nil {
		return nil, errors.New("nil project")
	}

	var scli store.Client
	var err error
	if p.Spec.SecretsProvider.Addr != "" {
		scli, err = store.New(p.Spec.SecretsProvider)
		if err != nil {
			return nil, err
		}
	}

	eproj := &spec.EffectiveProject{Project: p}
	for _, sref := range p.Spec.Stacks {
		stackDir := filepath.Join(p.Root, sref.Path)
		stk, stkerr := LoadStack(stackDir)
		if stkerr != nil {
			return nil, fmt.Errorf("load stack %s: %w", sref.Name, stkerr)
		}

		instances := stk.Spec.Instances
		if strings.ToLower(stk.Spec.Type) == "shared" {
			instances = []spec.InstanceRef{{Name: ""}}
		}

		for i := range instances {
			inst := instances[i]
			es := spec.EffectiveStack{Stack: stk}
			if strings.ToLower(stk.Spec.Type) != "shared" {
				es.Instance = &inst
			}

			for _, ssvc := range stk.Spec.Services {
				serviceDir := filepath.Join(stackDir, ssvc.Path)
				svc, serr := LoadService(serviceDir)
				if serr != nil {
					return nil, fmt.Errorf("load service %s: %w", ssvc.Name, serr)
				}

				ctxMap := map[string]any{
					"project":  map[string]any{"name": p.Metadata.Name, "vars": p.Spec.Vars},
					"stack":    map[string]any{"name": stk.Metadata.Name, "instances": map[string]any{"vars": inst.Vars}},
					"instance": map[string]any{"name": inst.Name, "vars": inst.Vars},
					"service":  map[string]any{"name": svc.Metadata.Name, "env": envSliceToMap(svc.Spec.Env)},
					"git":      map[string]any{"short_sha": ""},
				}

				// Phase 0: precompute file targets (service scope)
				secretTargets := make(map[string]spec.ReferenceFileTarget, len(svc.Spec.Secrets))
				for _, sd := range svc.Spec.Secrets {
					secretTargets[sd.Name] = spec.ResolveFileTarget(sd.Name, sd.File, true)
				}
				configTargets := make(map[string]spec.ReferenceFileTarget, len(svc.Spec.Configs))
				for _, cd := range svc.Spec.Configs {
					configTargets[cd.Name] = spec.ResolveFileTarget(cd.Name, cd.File, false)
				}

				getSecretPath := func(name string) (string, error) {
					if mp, ok := secretTargets[name]; ok && mp.Target != "" {
						return mp.Target, nil
					}
					return "/run/secrets/" + name, nil
				}

				cfgEngine := render.NewEngine(render.WithConfigFuncs(renderx.ConfigFuncMap(getSecretPath)))
				secEngine := render.NewEngine(render.WithSecretFuncs(renderx.SecretFuncMap(ctx, scli, getSecretPath)))

				effsvc := spec.EffectiveService{
					Service:  svc,
					Name:     svc.Metadata.Name,
					Env:      envSliceToMap(svc.Spec.Env), // non-secret env only
					Networks: netAttachNames(svc.Spec.Networks),
				}

				// Phase 1: secrets
				for _, sd := range svc.Spec.Secrets {
					var bytes []byte
					if sd.Template != "" && sd.FromVault == "" {
						path := filepath.Join(serviceDir, sd.Template)
						bytes, err = secEngine.RenderFile(path, ctxMap, true)
						if err != nil {
							return nil, fmt.Errorf("render secret template %s (%s): %w", sd.Name, path, err)
						}
					} else if sd.FromVault != "" && sd.Template == "" {
						bytes, err = scli.ResolveSecret(ctx, strings.TrimSpace(sd.FromVault))
						if err != nil {
							return nil, fmt.Errorf("vault resolve %q: %w", sd.FromVault, err)
						}
					} else {
						return nil, fmt.Errorf("secret %s: only one of FromVault or Template must be set", sd.Name)
					}
					effsvc.Secrets = append(effsvc.Secrets, spec.EffectiveSecret{
						Name: sd.Name,
						Data: bytes,
						File: secretTargets[sd.Name],
					})
				}

				// Phase 2: configs
				for _, cd := range svc.Spec.Configs {
					if cd.Template == "" {
						effsvc.Configs = append(effsvc.Configs, spec.EffectiveConfig{
							Name: cd.Name,
							Data: nil,
							File: configTargets[cd.Name],
						})
						continue
					}
					var bytes []byte
					path := filepath.Join(serviceDir, cd.Template)
					bytes, err = cfgEngine.RenderFile(path, ctxMap)
					if err != nil {
						return nil, fmt.Errorf("render config %s (%s): %w", cd.Name, path, err)
					}
					effsvc.Configs = append(effsvc.Configs, spec.EffectiveConfig{
						Name: cd.Name,
						Data: bytes,
						File: configTargets[cd.Name],
					})
				}

				es.Services = append(es.Services, effsvc)
			}
			eproj.Stacks = append(eproj.Stacks, es)
		}
	}
	return eproj, nil
}

func envSliceToMap(in []spec.EnvVar) map[string]string {
	m := make(map[string]string, len(in))
	for _, e := range in {
		m[e.Name] = e.Value
	}
	return m
}

func netAttachNames(in []spec.NetAttach) []string {
	out := make([]string, 0, len(in))
	for _, n := range in {
		out = append(out, n.Name)
	}
	return out
}
