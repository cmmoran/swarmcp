package manifest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cmmoran/swarmcp/internal/spec"
	"github.com/cmmoran/swarmcp/internal/vault"
	"gopkg.in/yaml.v3"
)

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

// Renderer is a tiny interface so manifest does not import render package heavy types
// (plug by adapter in main)
type Renderer interface {
	RenderTemplateString(name, tpl string, data map[string]any, secretMarker ...any) (string, error)
	RenderFile(path string, data map[string]any, secretMarker ...any) ([]byte, error)
}

// ResolveEffective builds the fully merged effective model, renders config templates,
// and resolves Vault secrets (if vault is configured in the project).
func ResolveEffective(ctx context.Context, p *spec.Project, r Renderer) (*spec.EffectiveProject, error) {
	return resolveEffectiveWithVault(ctx, p, r)
}

func resolveEffectiveWithVault(ctx context.Context, p *spec.Project, r Renderer) (*spec.EffectiveProject, error) {
	if p == nil {
		return nil, errors.New("nil project")
	}
	var vcli vault.Client = vault.NewNoop()
	var err error
	// Initialize Vault only once per project if configured
	if p.Spec.Vault.Addr != "" {
		vcli, err = vault.NewFromProject(ctx, p)
		if err != nil {
			return nil, err
		}
		defer vcli.Close()
	}

	eproj := &spec.EffectiveProject{Project: p}
	for _, sref := range p.Spec.Stacks {
		stackDir := filepath.Join(p.Root, sref.Path)
		stk, err := LoadStack(stackDir)
		if err != nil {
			return nil, fmt.Errorf("load stack %s: %w", sref.Name, err)
		}

		instances := stk.Spec.Instances
		if strings.ToLower(stk.Spec.Type) == "shared" {
			instances = []spec.InstanceRef{{Name: ""}} // a single shared instance with empty name
		}

		for i := range instances {
			inst := instances[i] // capture
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

				// Build context for templates
				ctxMap := map[string]any{
					"project":  map[string]any{"name": p.Metadata.Name, "vars": p.Spec.Vars},
					"stack":    map[string]any{"name": stk.Metadata.Name, "instances": map[string]any{"vars": inst.Vars}},
					"instance": map[string]any{"name": inst.Name, "vars": inst.Vars},
					"service":  map[string]any{"name": svc.Metadata.Name, "env": envSliceToMap(svc.Spec.Env)},
					"git":      map[string]any{"short_sha": ""},
				}

				// Merge defaults: project.defaults → stack.defaults → service.spec
				merged := svc.Spec // start from service
				merged.Networks = mergeNetworks(p.Spec.Defaults.Networks, stk.Spec.Defaults.Networks, svc.Spec.Networks)
				merged.Deploy.Resources = mergeResources(p.Spec.Defaults.Resources, svc.Spec.Deploy.Resources)
				// For MVP, keep service env as-is.

				effsvc := spec.EffectiveService{
					Service:  svc,
					Name:     svc.Metadata.Name,
					Env:      envSliceToMap(svc.Spec.Env),
					Networks: netAttachNames(merged.Networks),
				}

				for _, c := range svc.Spec.Configs {
					if c.Template == "" {
						continue
					}
					path := filepath.Join(serviceDir, c.Template)
					b, berr := r.RenderFile(path, ctxMap)
					if berr != nil {
						return nil, fmt.Errorf("render config %s - %s: %w", c.Name, path, berr)
					}
					effsvc.Configs = append(effsvc.Configs, spec.EffectiveConfig{
						Name: c.Name,
						Data: b,
						File: spec.ResolveFileTarget(c.Name, c.File, false),
					})
				}

				for _, sd := range svc.Spec.Secrets {
					var (
						b     []byte
						sberr error
					)
					if len(sd.Template) == 0 && len(sd.FromVault) != 0 {
						b, sberr = vcli.ResolveSecret(ctx, strings.TrimSpace(sd.FromVault))
						if sberr != nil {
							return nil, fmt.Errorf("vault resolve %q: %w", sd.FromVault, sberr)
						}
					} else if len(sd.Template) != 0 && len(sd.FromVault) == 0 {
						fromPath, fperr := r.RenderFile(sd.Template, ctxMap, true)
						if fperr != nil {
							return nil, fmt.Errorf("vault path render: %w", fperr)
						}
						b = fromPath
					} else {
						return nil, fmt.Errorf("secret %s: only one of FromVault or Template can be set", sd.Name)
					}
					effsvc.Secrets = append(effsvc.Secrets, spec.EffectiveSecret{
						Name: sd.Name,
						Data: b,
						File: spec.ResolveFileTarget(sd.Name, sd.File, true),
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

func mergeNetworks(proj map[string]spec.NetworkDef, stack map[string]spec.NetworkDef, svc []spec.NetAttach) []spec.NetAttach {
	if len(svc) > 0 {
		return svc
	}
	union := map[string]struct{}{}
	for k := range proj {
		union[k] = struct{}{}
	}
	for k := range stack {
		union[k] = struct{}{}
	}
	names := make([]string, 0, len(union))
	for k := range union {
		names = append(names, k)
	}
	atts := make([]spec.NetAttach, 0, len(names))
	for _, n := range names {
		atts = append(atts, spec.NetAttach{Name: n})
	}
	return atts
}

func mergeResources(def spec.Resources, svc spec.Resources) spec.Resources {
	out := svc
	if out.Reservations.CPUs == "" {
		out.Reservations.CPUs = def.Reservations.CPUs
	}
	if out.Reservations.Memory == "" {
		out.Reservations.Memory = def.Reservations.Memory
	}
	if out.Limits.CPUs == "" {
		out.Limits.CPUs = def.Limits.CPUs
	}
	if out.Limits.Memory == "" {
		out.Limits.Memory = def.Limits.Memory
	}
	return out
}
