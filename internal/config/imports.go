package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cmmoran/swarmcp/internal/yamlutil"
	"gopkg.in/yaml.v3"
)

func ResolveImports(cfg *Config, opts LoadOptions) error {
	if cfg == nil {
		return nil
	}
	for stackName, stack := range cfg.Stacks {
		if stack.BaseDir == "" {
			stack.BaseDir = cfg.BaseDir
		}
		if stack.Source == nil {
			if stack.Overrides != nil {
				return fmt.Errorf("stack %q: overrides require source", stackName)
			}
			cfg.Stacks[stackName] = applyStackBaseDirs(stack)
			continue
		}
		if hasStackLocalFields(stack) {
			return fmt.Errorf("stack %q: local fields are not allowed when source is set", stackName)
		}
		stackDef, baseDir, err := loadStackFromSource(stack.Source, stack.BaseDir, stack.Overrides, opts)
		if err != nil {
			return fmt.Errorf("stack %q: %w", stackName, err)
		}
		stackDef.BaseDir = baseDir
		cfg.Stacks[stackName] = applyStackBaseDirs(stackDef)
	}
	for stackName, stack := range cfg.Stacks {
		if err := resolveServiceImports(stackName, &stack, opts); err != nil {
			return err
		}
		cfg.Stacks[stackName] = stack
	}
	return nil
}

func loadStackFromSource(ref *SourceRef, baseDir string, overrides map[string]any, opts LoadOptions) (Stack, string, error) {
	if ref == nil {
		return Stack{}, "", fmt.Errorf("stack source is nil")
	}
	baseMap, basePath, err := loadSourceMap(*ref, baseDir, opts)
	if err != nil {
		return Stack{}, "", err
	}
	var overlay map[string]any
	if ref.OverridesPath != "" {
		overlay, _, err = loadSourceMap(SourceRef{
			URL:  ref.URL,
			Ref:  ref.Ref,
			Path: ref.OverridesPath,
		}, baseDir, opts)
		if err != nil {
			return Stack{}, "", err
		}
	}
	localOverrides, err := normalizeOverrideMap(overrides)
	if err != nil {
		return Stack{}, "", err
	}
	merged, err := mergeMaps(baseMap, overlay, localOverrides)
	if err != nil {
		return Stack{}, "", err
	}
	stack, err := decodeStackMap(merged)
	if err != nil {
		return Stack{}, "", err
	}
	if stack.Source != nil || stack.Overrides != nil {
		return Stack{}, "", fmt.Errorf("imported stack must not define source or overrides")
	}
	return stack, filepath.Dir(basePath), nil
}

func resolveServiceImports(stackName string, stack *Stack, opts LoadOptions) error {
	if stack == nil {
		return nil
	}
	for serviceName, service := range stack.Services {
		if service.Source == nil {
			if service.Overrides != nil {
				return fmt.Errorf("stack %q service %q: overrides require source", stackName, serviceName)
			}
			if service.BaseDir == "" {
				service.BaseDir = stack.BaseDir
				stack.Services[serviceName] = service
			}
			continue
		}
		if hasServiceLocalFields(service) {
			return fmt.Errorf("stack %q service %q: local fields are not allowed when source is set", stackName, serviceName)
		}
		serviceDef, baseDir, err := loadServiceFromSource(service.Source, stack.BaseDir, service.Overrides, opts)
		if err != nil {
			return fmt.Errorf("stack %q service %q: %w", stackName, serviceName, err)
		}
		serviceDef.BaseDir = baseDir
		stack.Services[serviceName] = serviceDef
	}
	return nil
}

func loadServiceFromSource(ref *SourceRef, baseDir string, overrides map[string]any, opts LoadOptions) (Service, string, error) {
	if ref == nil {
		return Service{}, "", fmt.Errorf("service source is nil")
	}
	baseMap, basePath, err := loadSourceMap(*ref, baseDir, opts)
	if err != nil {
		return Service{}, "", err
	}
	var overlay map[string]any
	if ref.OverridesPath != "" {
		overlay, _, err = loadSourceMap(SourceRef{
			URL:  ref.URL,
			Ref:  ref.Ref,
			Path: ref.OverridesPath,
		}, baseDir, opts)
		if err != nil {
			return Service{}, "", err
		}
	}
	localOverrides, err := normalizeOverrideMap(overrides)
	if err != nil {
		return Service{}, "", err
	}
	merged, err := mergeMaps(baseMap, overlay, localOverrides)
	if err != nil {
		return Service{}, "", err
	}
	service, err := decodeServiceMap(merged)
	if err != nil {
		return Service{}, "", err
	}
	if service.Source != nil || service.Overrides != nil {
		return Service{}, "", fmt.Errorf("imported service must not define source or overrides")
	}
	return service, filepath.Dir(basePath), nil
}

func loadSourceMap(ref SourceRef, baseDir string, opts LoadOptions) (map[string]any, string, error) {
	if ref.Path == "" {
		return nil, "", fmt.Errorf("source.path is required")
	}
	path, err := resolveSourceFile(ref, baseDir, opts)
	if err != nil {
		return nil, "", err
	}
	data, err := ReadSourceFile(path, "", opts)
	if err != nil {
		return nil, "", err
	}
	normalizedText := normalizeTemplateScalars(string(data))
	var parsed any
	if err := yaml.Unmarshal([]byte(normalizedText), &parsed); err != nil {
		return nil, "", err
	}
	normalized := yamlutil.NormalizeValue(parsed)
	if normalized == nil {
		return map[string]any{}, path, nil
	}
	out, ok := normalized.(map[string]any)
	if !ok {
		return nil, "", fmt.Errorf("expected map at root of %q", path)
	}
	return out, path, nil
}

func resolveSourceFile(ref SourceRef, baseDir string, opts LoadOptions) (string, error) {
	root := baseDir
	if ref.URL != "" {
		var err error
		root, err = FetchRepoRoot(ref.URL, ref.Ref, opts)
		if err != nil {
			return "", err
		}
	}
	if filepath.IsAbs(ref.Path) {
		if ref.URL != "" {
			return "", fmt.Errorf("source.path must be relative for git sources")
		}
		return resolveAbsolutePath(ref.Path)
	}
	return resolvePathWithin(root, ref.Path, opts)
}

func resolveAbsolutePath(path string) (string, error) {
	eval, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(eval); err != nil {
		return "", err
	}
	return eval, nil
}

func normalizeOverrideMap(overrides map[string]any) (map[string]any, error) {
	if overrides == nil {
		return nil, nil
	}
	normalized := yamlutil.NormalizeValue(overrides)
	out, ok := normalized.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("overrides must be a map")
	}
	return out, nil
}

func decodeStackMap(doc map[string]any) (Stack, error) {
	encoded, err := yaml.Marshal(doc)
	if err != nil {
		return Stack{}, err
	}
	var out Stack
	if err := yaml.Unmarshal(encoded, &out); err != nil {
		return Stack{}, err
	}
	return out, nil
}

func decodeServiceMap(doc map[string]any) (Service, error) {
	encoded, err := yaml.Marshal(doc)
	if err != nil {
		return Service{}, err
	}
	var out Service
	if err := yaml.Unmarshal(encoded, &out); err != nil {
		return Service{}, err
	}
	return out, nil
}

func applyStackBaseDirs(stack Stack) Stack {
	if stack.BaseDir == "" {
		return stack
	}
	for serviceName, service := range stack.Services {
		if service.BaseDir == "" {
			service.BaseDir = stack.BaseDir
			stack.Services[serviceName] = service
		}
	}
	return stack
}

func hasStackLocalFields(stack Stack) bool {
	if stack.Mode != "" ||
		len(stack.Partitions) > 0 ||
		hasSources(stack.Sources) ||
		len(stack.Configs.Defs) > 0 ||
		len(stack.Secrets.Defs) > 0 ||
		len(stack.Volumes) > 0 ||
		len(stack.Services) > 0 {
		return true
	}
	return false
}

func hasServiceLocalFields(service Service) bool {
	if service.Image != "" ||
		len(service.Command) > 0 ||
		len(service.Args) > 0 ||
		service.Workdir != "" ||
		len(service.Env) > 0 ||
		len(service.Ports) > 0 ||
		service.Mode != "" ||
		service.Replicas != 0 ||
		len(service.Labels) > 0 ||
		len(service.Placement.Constraints) > 0 ||
		service.Healthcheck != nil ||
		len(service.DependsOn) > 0 ||
		service.Egress ||
		len(service.Networks) > 0 ||
		service.NetworkEphemeral != nil ||
		len(service.Configs) > 0 ||
		len(service.Secrets) > 0 ||
		len(service.Volumes) > 0 ||
		hasSources(service.Sources) {
		return true
	}
	return false
}
