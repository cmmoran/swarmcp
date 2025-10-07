package manifest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func LoadProject(root string) (*Project, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(abs, "project.yaml"))
	if err != nil {
		return nil, err
	}
	var p Project
	if err := yaml.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	if strings.ToLower(p.Kind) != "project" {
		return nil, fmt.Errorf("not a Project kind")
	}
	p.Root = abs
	return &p, nil
}

func LoadStack(dir string) (*Stack, error) {
	b, err := os.ReadFile(filepath.Join(dir, "stack.yaml"))
	if err != nil {
		return nil, err
	}
	var s Stack
	if err := yaml.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	s.Dir = dir
	return &s, nil
}

func LoadService(dir string) (*Service, error) {
	b, err := os.ReadFile(filepath.Join(dir, "service.yaml"))
	if err != nil {
		return nil, err
	}
	var svc Service
	if err := yaml.Unmarshal(b, &svc); err != nil {
		return nil, err
	}
	svc.Dir = dir
	return &svc, nil
}

type Renderer interface {
	RenderString(name, tpl string, data map[string]any) (string, error)
	RenderFile(path string, data map[string]any) ([]byte, error)
}

func ResolveEffective(ctx context.Context, p *Project, r Renderer) (*EffectiveProject, error) {
	if p == nil {
		return nil, errors.New("nil project")
	}
	eproj := &EffectiveProject{Project: p}
	for _, sref := range p.Spec.Stacks {
		stackDir := filepath.Join(p.Root, sref.Path)
		stk, err := LoadStack(stackDir)
		if err != nil {
			return nil, fmt.Errorf("load stack %s: %w", sref.Name, err)
		}

		instances := stk.Spec.Instances
		if strings.ToLower(stk.Spec.Type) == "shared" {
			instances = []InstanceRef{{Name: ""}}
		}

		for i := range instances {
			inst := instances[i]
			es := EffectiveStack{Stack: stk}
			if strings.ToLower(stk.Spec.Type) != "shared" {
				es.Instance = &inst
			}

			for _, ssvc := range stk.Spec.Services {
				serviceDir := filepath.Join(stackDir, ssvc.Path)
				svc, err := LoadService(serviceDir)
				if err != nil {
					return nil, fmt.Errorf("load service %s: %w", ssvc.Name, err)
				}

				ctxMap := map[string]any{
					"project":  map[string]any{"name": p.Metadata.Name, "vars": p.Spec.Vars},
					"stack":    map[string]any{"name": stk.Metadata.Name, "instances": map[string]any{"vars": inst.Vars}},
					"instance": map[string]any{"name": inst.Name, "vars": inst.Vars},
					"service":  map[string]any{"name": svc.Metadata.Name},
					"git":      map[string]any{"short_sha": ""},
				}

				merged := svc.Spec
				merged.Networks = mergeNetworks(p.Spec.Defaults.Networks, stk.Spec.Defaults.Networks, svc.Spec.Networks)
				merged.Deploy.Resources = mergeResources(p.Spec.Defaults.Resources, svc.Spec.Deploy.Resources)

				rendered := map[string][]byte{}
				for _, c := range svc.Spec.Configs {
					if c.Template == "" {
						continue
					}
					path := filepath.Join(serviceDir, c.Template)
					b, err := r.RenderFile(path, ctxMap)
					if err != nil {
						return nil, fmt.Errorf("render %s: %w", path, err)
					}
					rendered[c.Name] = b
				}

				effsvc := EffectiveService{
					Service:         svc,
					Name:            svc.Metadata.Name,
					RenderedConfigs: rendered,
					ResolvedSecrets: map[string][]byte{},
					EffectiveEnv:    envSliceToMap(svc.Spec.Env),
					EffectiveNets:   netAttachNames(merged.Networks),
				}
				es.Services = append(es.Services, effsvc)
			}
			eproj.Stacks = append(eproj.Stacks, es)
		}
	}
	return eproj, nil
}

func envSliceToMap(in []EnvVar) map[string]string {
	m := make(map[string]string, len(in))
	for _, e := range in {
		m[e.Name] = e.Value
	}
	return m
}

func netAttachNames(in []NetAttach) []string {
	out := make([]string, 0, len(in))
	for _, n := range in {
		out = append(out, n.Name)
	}
	return out
}

func mergeNetworks(proj map[string]NetworkDef, stack map[string]NetworkDef, svc []NetAttach) []NetAttach {
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
	atts := make([]NetAttach, 0, len(names))
	for _, n := range names {
		atts = append(atts, NetAttach{Name: n})
	}
	return atts
}

func mergeResources(def Resources, svc Resources) Resources {
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
