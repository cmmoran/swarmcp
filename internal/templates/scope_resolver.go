package templates

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/yamlutil"
	"gopkg.in/yaml.v3"
)

type Scope struct {
	Project          string
	Deployment       string
	Stack            string
	Partition        string
	Service          string
	NetworksShared   string
	NetworkEphemeral string
}

type ScopeResolver struct {
	cfg          *config.Config
	scope        Scope
	allowSecrets bool
	allowMissing bool
	data         any
	values       any
	inProgress   map[string]bool
	secretValue  func(Scope, string) (string, error)
	trace        func(TraceCall)
}

type TraceCall struct {
	Func    string
	Kind    string
	Name    string
	From    Scope
	To      Scope
	Self    bool
	Missing bool
}

func NewScopeResolver(cfg *config.Config, scope Scope, allowSecrets bool, allowMissing bool, data any, secretValue func(Scope, string) (string, error), values any) *ScopeResolver {
	return NewScopeResolverWithTrace(cfg, scope, allowSecrets, allowMissing, data, secretValue, values, nil)
}

func NewScopeResolverWithTrace(cfg *config.Config, scope Scope, allowSecrets bool, allowMissing bool, data any, secretValue func(Scope, string) (string, error), values any, trace func(TraceCall)) *ScopeResolver {
	return &ScopeResolver{
		cfg:          cfg,
		scope:        scope,
		allowSecrets: allowSecrets,
		allowMissing: allowMissing,
		data:         data,
		values:       values,
		inProgress:   make(map[string]bool),
		secretValue:  secretValue,
		trace:        trace,
	}
}

func (r *ScopeResolver) ConfigValue(name string) (any, error) {
	def, resolvedScope, ok := r.resolveConfigWithScope(name)
	if !ok {
		return r.fallbackConfigValue(name)
	}
	r.emitTrace("config_value", "config", name, resolvedScope, false)
	if def.Source == "" {
		return "", fmt.Errorf("config %q is missing source", name)
	}
	if r.inProgress[name] {
		return "", fmt.Errorf("config %q has a recursive reference", name)
	}
	r.inProgress[name] = true
	defer delete(r.inProgress, name)

	engine := New(r)
	rendered, err := ResolveSource(def.Source, r.scope, r.data, engine, r.values, r.cfg.BaseDir, config.LoadOptions{Offline: r.cfg.Offline, CacheDir: r.cfg.CacheDir, Debug: r.cfg.Debug})
	if err != nil {
		return "", err
	}
	var parsed any
	if err := yaml.Unmarshal([]byte(rendered), &parsed); err != nil {
		return "", err
	}
	normalized := yamlutil.NormalizeValue(parsed)
	switch normalized.(type) {
	case map[string]any, []any:
		return normalized, nil
	case nil:
		return rendered, nil
	default:
		return rendered, nil
	}
}

func (r *ScopeResolver) fallbackConfigValue(name string) (any, error) {
	if r.values == nil {
		return "", fmt.Errorf("config %q not found", name)
	}
	r.emitTrace("config_value", "config", name, r.scope, true)
	fragment := "#/" + name
	value, err := ResolveValuesFragmentValue(r.values, fragment, r.scope)
	if err != nil {
		return "", err
	}
	normalized := yamlutil.NormalizeValue(value)
	switch normalized.(type) {
	case map[string]any, []any:
		return normalized, nil
	case nil:
		return "", nil
	default:
		return formatFragmentValue(normalized)
	}
}

func (r *ScopeResolver) ConfigRef(name string) (string, error) {
	def, resolvedScope, ok := r.resolveConfigWithScope(name)
	if !ok {
		if r.allowMissing {
			r.emitTrace("config_ref", "config", name, r.scope, true)
			return "/" + name, nil
		}
		return "", fmt.Errorf("config %q not found", name)
	}
	r.emitTrace("config_ref", "config", name, resolvedScope, false)
	if def.Target != "" {
		return ExpandPathTokens(def.Target, r.scope), nil
	}
	return "/" + name, nil
}

func (r *ScopeResolver) SecretValue(name string) (string, error) {
	if !r.allowSecrets {
		return "", fmt.Errorf("secret_value is not allowed in config templates")
	}
	if r.secretValue == nil {
		return "", fmt.Errorf("secret_value resolver not configured")
	}
	_, resolvedScope, ok := r.resolveSecretWithScope(name)
	if ok {
		r.emitTrace("secret_value", "secret", name, resolvedScope, false)
		return r.secretValue(resolvedScope, name)
	}
	r.emitTrace("secret_value", "secret", name, r.scope, true)
	return r.secretValue(r.scope, name)
}

func (r *ScopeResolver) SecretRef(name string) (string, error) {
	def, resolvedScope, ok := r.resolveSecretWithScope(name)
	if !ok {
		if r.allowMissing {
			r.emitTrace("secret_ref", "secret", name, r.scope, true)
			return "/run/secrets/" + name, nil
		}
		return "", fmt.Errorf("secret %q not found", name)
	}
	r.emitTrace("secret_ref", "secret", name, resolvedScope, false)
	if def.Target != "" {
		return ExpandPathTokens(def.Target, r.scope), nil
	}
	return "/run/secrets/" + name, nil
}

func (r *ScopeResolver) RuntimeValue(args ...string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	if len(args) == 1 {
		return ExpandTokens(args[0], r.scope), nil
	}
	kind := strings.TrimSpace(args[0])
	switch kind {
	case "standard_volumes":
		return r.runtimeStandardVolumes(args[1:])
	default:
		return "", fmt.Errorf("runtime_value %q: unsupported kind", kind)
	}
}

func (r *ScopeResolver) runtimeStandardVolumes(args []string) (string, error) {
	if r.cfg == nil {
		return "", nil
	}
	stackName := r.scope.Stack
	serviceName := r.scope.Service
	if stackName == "" || serviceName == "" {
		return "", nil
	}
	stack, ok := r.cfg.Stacks[stackName]
	if !ok {
		return "", nil
	}
	services, err := r.cfg.StackServices(stackName, r.scope.Partition)
	if err != nil {
		return "", nil
	}
	service, ok := services[serviceName]
	if !ok {
		return "", nil
	}

	opts, err := parseRuntimeOptions(args)
	if err != nil {
		return "", err
	}
	format := opts.format
	if format == "" {
		format = "csv"
	}

	engine := New(r)
	filterStandard := opts.standards
	filterCategory := opts.categories

	serviceStandard := config.ServiceStandardName(r.cfg)
	serviceTarget := config.ServiceTarget(r.cfg)
	basePath := strings.TrimSpace(r.cfg.Project.Defaults.Volumes.BasePath)
	if basePath != "" {
		basePath, err = renderRuntimeString(engine, r.data, "runtime.standard_volumes.base_path", basePath)
		if err != nil {
			return "", err
		}
	}

	var pairs []string
	for _, ref := range service.Volumes {
		if strings.TrimSpace(ref.Standard) == "" {
			continue
		}
		if len(filterStandard) > 0 {
			if _, ok := filterStandard[ref.Standard]; !ok {
				continue
			}
		}
		if len(filterCategory) > 0 {
			if strings.TrimSpace(ref.Category) == "" {
				continue
			}
			if _, ok := filterCategory[ref.Category]; !ok {
				continue
			}
		}

		if ref.Standard == serviceStandard {
			if basePath == "" {
				return "", fmt.Errorf("runtime_value standard_volumes: project.defaults.volumes.base_path is required")
			}
			target := firstNonEmpty(ref.Target, serviceTarget)
			target, err = renderRuntimeString(engine, r.data, "runtime.standard_volumes.target", target)
			if err != nil {
				return "", err
			}
			if strings.TrimSpace(target) == "" {
				return "", fmt.Errorf("runtime_value standard_volumes: target is required")
			}
			target = ExpandPathTokens(target, r.scope)
			subpath, err := renderRuntimeString(engine, r.data, "runtime.standard_volumes.subpath", ref.Subpath)
			if err != nil {
				return "", err
			}
			source := config.StackVolumeSource(basePath, r.cfg.Project.Name, stackName, stack.Mode, r.scope.Partition, serviceName, "", "", subpath, false)
			pairs = append(pairs, source+":"+target)
			continue
		}

		standard, ok := r.cfg.Project.Defaults.Volumes.Standards[ref.Standard]
		if !ok {
			return "", fmt.Errorf("runtime_value standard_volumes: standard %q not found", ref.Standard)
		}
		source, err := renderRuntimeString(engine, r.data, fmt.Sprintf("runtime.standard_volumes.%s.source", ref.Standard), standard.Source)
		if err != nil {
			return "", err
		}
		target, err := renderRuntimeString(engine, r.data, fmt.Sprintf("runtime.standard_volumes.%s.target", ref.Standard), standard.Target)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(source) == "" || strings.TrimSpace(target) == "" {
			return "", fmt.Errorf("runtime_value standard_volumes: standard %q source/target is required", ref.Standard)
		}
		pairs = append(pairs, source+":"+target)
	}

	switch format {
	case "csv":
		return strings.Join(pairs, ","), nil
	case "json":
		encoded, err := json.Marshal(pairs)
		if err != nil {
			return "", err
		}
		return string(encoded), nil
	case "yaml":
		encoded, err := yaml.Marshal(pairs)
		if err != nil {
			return "", err
		}
		return strings.TrimSuffix(string(encoded), "\n"), nil
	default:
		return "", fmt.Errorf("runtime_value standard_volumes: unknown _format %q", format)
	}
}

type runtimeOptions struct {
	format     string
	standards  map[string]struct{}
	categories map[string]struct{}
}

func parseRuntimeOptions(args []string) (runtimeOptions, error) {
	var opts runtimeOptions
	for _, raw := range args {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		parts := strings.SplitN(raw, "=", 2)
		if len(parts) != 2 {
			return opts, fmt.Errorf("runtime_value: invalid option %q", raw)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		switch key {
		case "_format", "format":
			opts.format = value
		case "standard":
			opts.standards = parseRuntimeSet(value)
		case "category":
			opts.categories = parseRuntimeSet(value)
		default:
			return opts, fmt.Errorf("runtime_value: unknown option %q", key)
		}
	}
	return opts, nil
}

func parseRuntimeSet(value string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out[item] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func renderRuntimeString(engine *Engine, data any, name string, value string) (string, error) {
	if value == "" {
		return "", nil
	}
	return engine.Render(name, value, data)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (r *ScopeResolver) resolveConfig(name string) (config.ConfigDef, bool) {
	def, _, ok := r.resolveConfigWithScope(name)
	return def, ok
}

func (r *ScopeResolver) resolveSecret(name string) (config.SecretDef, bool) {
	def, _, ok := r.resolveSecretWithScope(name)
	return def, ok
}

func (r *ScopeResolver) resolveServiceConfig(name string) (config.ConfigDef, bool) {
	if r.scope.Service == "" {
		return config.ConfigDef{}, false
	}
	services, err := r.cfg.StackServices(r.scope.Stack, r.scope.Partition)
	if err != nil {
		return config.ConfigDef{}, false
	}
	service, ok := services[r.scope.Service]
	if !ok {
		return config.ConfigDef{}, false
	}
	for _, ref := range service.Configs {
		if ref.Name != name {
			continue
		}
		if ref.Source == "" {
			continue
		}
		return config.ConfigDef{
			Source: ref.Source,
			Target: ref.Target,
			UID:    ref.UID,
			GID:    ref.GID,
			Mode:   ref.Mode,
		}, true
	}
	return config.ConfigDef{}, false
}

func (r *ScopeResolver) resolveServiceSecret(name string, allowEmpty bool) (config.SecretDef, bool) {
	if r.scope.Service == "" {
		return config.SecretDef{}, false
	}
	services, err := r.cfg.StackServices(r.scope.Stack, r.scope.Partition)
	if err != nil {
		return config.SecretDef{}, false
	}
	service, ok := services[r.scope.Service]
	if !ok {
		return config.SecretDef{}, false
	}
	for _, ref := range service.Secrets {
		if ref.Name != name {
			continue
		}
		if ref.Source == "" && !allowEmpty {
			continue
		}
		return config.SecretDef{
			Source: config.DefaultSecretSource(ref.Name, ref.Source),
			Target: ref.Target,
			UID:    ref.UID,
			GID:    ref.GID,
			Mode:   ref.Mode,
		}, true
	}
	return config.SecretDef{}, false
}

func (r *ScopeResolver) resolveConfigWithScope(name string) (config.ConfigDef, Scope, bool) {
	if r.scope.Stack != "" {
		if def, ok := r.resolveServiceConfig(name); ok {
			return def, Scope{Project: r.scope.Project, Deployment: r.scope.Deployment, Stack: r.scope.Stack, Partition: r.scope.Partition, Service: r.scope.Service}, true
		}
		if def, ok := r.resolvePartitionConfig(name); ok {
			return def, Scope{Project: r.scope.Project, Deployment: r.scope.Deployment, Stack: r.scope.Stack, Partition: r.scope.Partition}, true
		}
		if def, ok := r.resolveStackConfig(name); ok {
			return def, Scope{Project: r.scope.Project, Deployment: r.scope.Deployment, Stack: r.scope.Stack}, true
		}
	}
	if def, ok := r.cfg.ProjectConfigDefs(r.scope.Partition)[name]; ok {
		return def, Scope{Project: r.scope.Project, Deployment: r.scope.Deployment}, true
	}
	return config.ConfigDef{}, Scope{}, false
}

func (r *ScopeResolver) resolveSecretWithScope(name string) (config.SecretDef, Scope, bool) {
	if r.scope.Stack != "" {
		if def, ok := r.resolveServiceSecret(name, false); ok {
			return def, Scope{Project: r.scope.Project, Deployment: r.scope.Deployment, Stack: r.scope.Stack, Partition: r.scope.Partition, Service: r.scope.Service}, true
		}
		if def, ok := r.resolvePartitionSecret(name); ok {
			return def, Scope{Project: r.scope.Project, Deployment: r.scope.Deployment, Stack: r.scope.Stack, Partition: r.scope.Partition}, true
		}
		if def, ok := r.resolveStackSecret(name); ok {
			return def, Scope{Project: r.scope.Project, Deployment: r.scope.Deployment, Stack: r.scope.Stack}, true
		}
	}
	if def, ok := r.cfg.ProjectSecretDefs(r.scope.Partition)[name]; ok {
		return def, Scope{Project: r.scope.Project, Deployment: r.scope.Deployment}, true
	}
	if r.scope.Stack != "" {
		if def, ok := r.resolveServiceSecret(name, true); ok {
			return def, Scope{Project: r.scope.Project, Deployment: r.scope.Deployment, Stack: r.scope.Stack, Partition: r.scope.Partition, Service: r.scope.Service}, true
		}
	}
	return config.SecretDef{}, Scope{}, false
}

func (r *ScopeResolver) ResolveConfigWithScope(name string) (config.ConfigDef, Scope, bool) {
	return r.resolveConfigWithScope(name)
}

func (r *ScopeResolver) ResolveSecretWithScope(name string) (config.SecretDef, Scope, bool) {
	return r.resolveSecretWithScope(name)
}

func (r *ScopeResolver) emitTrace(fn string, kind string, name string, resolved Scope, missing bool) {
	if r.trace == nil {
		return
	}
	self := r.scope == resolved
	r.trace(TraceCall{
		Func:    fn,
		Kind:    kind,
		Name:    name,
		From:    r.scope,
		To:      resolved,
		Self:    self,
		Missing: missing,
	})
}

func (r *ScopeResolver) resolvePartitionConfig(name string) (config.ConfigDef, bool) {
	if r.scope.Partition == "" {
		return config.ConfigDef{}, false
	}
	defs := r.cfg.StackPartitionConfigDefs(r.scope.Stack, r.scope.Partition)
	def, ok := defs[name]
	return def, ok
}

func (r *ScopeResolver) resolvePartitionSecret(name string) (config.SecretDef, bool) {
	if r.scope.Partition == "" {
		return config.SecretDef{}, false
	}
	defs := r.cfg.StackPartitionSecretDefs(r.scope.Stack, r.scope.Partition)
	def, ok := defs[name]
	return def, ok
}

func (r *ScopeResolver) resolveStackConfig(name string) (config.ConfigDef, bool) {
	defs := r.cfg.StackConfigDefs(r.scope.Stack, r.scope.Partition)
	def, ok := defs[name]
	return def, ok
}

func (r *ScopeResolver) resolveStackSecret(name string) (config.SecretDef, bool) {
	defs := r.cfg.StackSecretDefs(r.scope.Stack, r.scope.Partition)
	def, ok := defs[name]
	return def, ok
}
