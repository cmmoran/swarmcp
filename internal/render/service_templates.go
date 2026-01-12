package render

import (
	"fmt"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/templates"
)

func NewServiceTemplateEngine(cfg *config.Config, scope templates.Scope, values any, allowMissingRefs bool, trace func(templates.TraceCall)) (*templates.ScopeResolver, *templates.Engine, TemplateData) {
	scope = withNetworkScope(cfg, scope)
	data := TemplateData{
		Project:    scope.Project,
		Deployment: scope.Deployment,
		Stack:      scope.Stack,
		Partition:  scope.Partition,
		Service:    scope.Service,
	}
	resolver := templates.NewScopeResolverWithTrace(cfg, scope, false, allowMissingRefs, data, nil, values, trace)
	return resolver, templates.New(resolver), data
}

func withNetworkScope(cfg *config.Config, scope templates.Scope) templates.Scope {
	scope.NetworksShared = config.NetworksSharedString(cfg, scope.Partition)
	if scope.Stack == "" || scope.Service == "" {
		return scope
	}
	stack, ok := cfg.Stacks[scope.Stack]
	if !ok {
		return scope
	}
	services, err := cfg.StackServices(scope.Stack, scope.Partition)
	if err != nil {
		return scope
	}
	service, ok := services[scope.Service]
	if !ok || service.NetworkEphemeral == nil {
		return scope
	}
	scope.NetworkEphemeral = config.EphemeralNetworkName(cfg, scope.Stack, stack.Mode, scope.Partition, scope.Service)
	return scope
}

func RenderServiceTemplates(engine *templates.Engine, data TemplateData, service config.Service) (config.Service, error) {
	rendered := service
	var err error

	if rendered.Image, err = RenderTemplateString(engine, data, "image", rendered.Image); err != nil {
		return config.Service{}, err
	}
	if rendered.Workdir, err = RenderTemplateString(engine, data, "workdir", rendered.Workdir); err != nil {
		return config.Service{}, err
	}
	if rendered.Mode, err = RenderTemplateString(engine, data, "mode", rendered.Mode); err != nil {
		return config.Service{}, err
	}
	if rendered.Command, err = renderTemplateStrings(engine, data, "command", rendered.Command); err != nil {
		return config.Service{}, err
	}
	if rendered.Args, err = renderTemplateStrings(engine, data, "args", rendered.Args); err != nil {
		return config.Service{}, err
	}
	if rendered.DependsOn, err = renderTemplateStrings(engine, data, "depends_on", rendered.DependsOn); err != nil {
		return config.Service{}, err
	}
	if rendered.Placement.Constraints, err = renderTemplateStrings(engine, data, "placement.constraints", rendered.Placement.Constraints); err != nil {
		return config.Service{}, err
	}
	if rendered.Networks, err = renderTemplateStrings(engine, data, "networks", rendered.Networks); err != nil {
		return config.Service{}, err
	}
	if rendered.Env, err = renderTemplateStringMap(engine, data, "env", rendered.Env); err != nil {
		return config.Service{}, err
	}
	if rendered.Ports, err = renderTemplatePorts(engine, data, rendered.Ports); err != nil {
		return config.Service{}, err
	}
	if rendered.Configs, err = renderTemplateConfigRefs(engine, data, rendered.Configs); err != nil {
		return config.Service{}, err
	}
	if rendered.Secrets, err = renderTemplateSecretRefs(engine, data, rendered.Secrets); err != nil {
		return config.Service{}, err
	}
	if rendered.Volumes, err = renderTemplateVolumeRefs(engine, data, rendered.Volumes); err != nil {
		return config.Service{}, err
	}
	rendered.NetworkEphemeral = service.NetworkEphemeral

	return rendered, nil
}

func RenderTemplateString(engine *templates.Engine, data TemplateData, name string, value string) (string, error) {
	if value == "" {
		return "", nil
	}
	rendered, err := engine.Render("service."+name, value, data)
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return rendered, nil
}

func renderTemplateStrings(engine *templates.Engine, data TemplateData, name string, values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	rendered := make([]string, 0, len(values))
	for i, value := range values {
		item, err := RenderTemplateString(engine, data, fmt.Sprintf("%s[%d]", name, i), value)
		if err != nil {
			return nil, err
		}
		if item == "" {
			continue
		}
		rendered = append(rendered, item)
	}
	return rendered, nil
}

func renderTemplateStringMap(engine *templates.Engine, data TemplateData, name string, values map[string]string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	rendered := make(map[string]string, len(values))
	for key, value := range values {
		item, err := RenderTemplateString(engine, data, fmt.Sprintf("%s.%s", name, key), value)
		if err != nil {
			return nil, err
		}
		rendered[key] = item
	}
	return rendered, nil
}

func renderTemplatePorts(engine *templates.Engine, data TemplateData, ports []config.Port) ([]config.Port, error) {
	if len(ports) == 0 {
		return nil, nil
	}
	rendered := make([]config.Port, 0, len(ports))
	for i, port := range ports {
		var err error
		if port.Protocol, err = RenderTemplateString(engine, data, fmt.Sprintf("ports[%d].protocol", i), port.Protocol); err != nil {
			return nil, err
		}
		if port.Mode, err = RenderTemplateString(engine, data, fmt.Sprintf("ports[%d].mode", i), port.Mode); err != nil {
			return nil, err
		}
		rendered = append(rendered, port)
	}
	return rendered, nil
}

func renderTemplateConfigRefs(engine *templates.Engine, data TemplateData, refs []config.ConfigRef) ([]config.ConfigRef, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	rendered := make([]config.ConfigRef, 0, len(refs))
	for i, ref := range refs {
		var err error
		if ref.Target, err = RenderTemplateString(engine, data, fmt.Sprintf("configs[%d].target", i), ref.Target); err != nil {
			return nil, err
		}
		if ref.UID, err = RenderTemplateString(engine, data, fmt.Sprintf("configs[%d].uid", i), ref.UID); err != nil {
			return nil, err
		}
		if ref.GID, err = RenderTemplateString(engine, data, fmt.Sprintf("configs[%d].gid", i), ref.GID); err != nil {
			return nil, err
		}
		if ref.Mode, err = RenderTemplateString(engine, data, fmt.Sprintf("configs[%d].mode", i), ref.Mode); err != nil {
			return nil, err
		}
		rendered = append(rendered, ref)
	}
	return rendered, nil
}

func renderTemplateSecretRefs(engine *templates.Engine, data TemplateData, refs []config.SecretRef) ([]config.SecretRef, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	rendered := make([]config.SecretRef, 0, len(refs))
	for i, ref := range refs {
		var err error
		if ref.Target, err = RenderTemplateString(engine, data, fmt.Sprintf("secrets[%d].target", i), ref.Target); err != nil {
			return nil, err
		}
		if ref.UID, err = RenderTemplateString(engine, data, fmt.Sprintf("secrets[%d].uid", i), ref.UID); err != nil {
			return nil, err
		}
		if ref.GID, err = RenderTemplateString(engine, data, fmt.Sprintf("secrets[%d].gid", i), ref.GID); err != nil {
			return nil, err
		}
		if ref.Mode, err = RenderTemplateString(engine, data, fmt.Sprintf("secrets[%d].mode", i), ref.Mode); err != nil {
			return nil, err
		}
		rendered = append(rendered, ref)
	}
	return rendered, nil
}

func renderTemplateVolumeRefs(engine *templates.Engine, data TemplateData, refs []config.VolumeRef) ([]config.VolumeRef, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	rendered := make([]config.VolumeRef, 0, len(refs))
	for i, ref := range refs {
		var err error
		if ref.Standard, err = RenderTemplateString(engine, data, fmt.Sprintf("volumes[%d].standard", i), ref.Standard); err != nil {
			return nil, err
		}
		if ref.Source, err = RenderTemplateString(engine, data, fmt.Sprintf("volumes[%d].source", i), ref.Source); err != nil {
			return nil, err
		}
		if ref.Target, err = RenderTemplateString(engine, data, fmt.Sprintf("volumes[%d].target", i), ref.Target); err != nil {
			return nil, err
		}
		if ref.Subpath, err = RenderTemplateString(engine, data, fmt.Sprintf("volumes[%d].subpath", i), ref.Subpath); err != nil {
			return nil, err
		}
		if ref.Category, err = RenderTemplateString(engine, data, fmt.Sprintf("volumes[%d].category", i), ref.Category); err != nil {
			return nil, err
		}
		rendered = append(rendered, ref)
	}
	return rendered, nil
}
