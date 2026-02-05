package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const valuesPrefix = "values#"

func SetSourcesBaseDir(cfg *Config) {
	if cfg == nil {
		return
	}
	cfg.Project.Sources.Base = cfg.BaseDir
	for stackName, stack := range cfg.Stacks {
		if stack.BaseDir == "" {
			stack.BaseDir = cfg.BaseDir
		}
		stack.Sources.Base = stack.BaseDir
		for partitionName, partition := range stack.Partitions {
			partition.Sources.Base = stack.BaseDir
			stack.Partitions[partitionName] = partition
		}
		for serviceName, service := range stack.Services {
			if service.BaseDir == "" {
				service.BaseDir = stack.BaseDir
			}
			service.Sources.Base = service.BaseDir
			stack.Services[serviceName] = service
		}
		for name, overlay := range stack.Overlays.Deployments {
			overlay.Sources.Base = stack.BaseDir
			for partitionName, partition := range overlay.Partitions {
				partition.Sources.Base = stack.BaseDir
				overlay.Partitions[partitionName] = partition
			}
			stack.Overlays.Deployments[name] = overlay
		}
		for i, rule := range stack.Overlays.Partitions.Rules {
			rule.Sources.Base = stack.BaseDir
			for partitionName, partition := range rule.Partitions {
				partition.Sources.Base = stack.BaseDir
				rule.Partitions[partitionName] = partition
			}
			stack.Overlays.Partitions.Rules[i] = rule
		}
		cfg.Stacks[stackName] = stack
	}
	for name, overlay := range cfg.Overlays.Deployments {
		overlay.Project.Sources.Base = cfg.BaseDir
		for stackName, stack := range overlay.Stacks {
			stack.Sources.Base = cfg.BaseDir
			for partitionName, partition := range stack.Partitions {
				partition.Sources.Base = cfg.BaseDir
				stack.Partitions[partitionName] = partition
			}
			overlay.Stacks[stackName] = stack
		}
		cfg.Overlays.Deployments[name] = overlay
	}
	for i, rule := range cfg.Overlays.Partitions.Rules {
		rule.Project.Sources.Base = cfg.BaseDir
		for stackName, stack := range rule.Stacks {
			stack.Sources.Base = cfg.BaseDir
			for partitionName, partition := range stack.Partitions {
				partition.Sources.Base = cfg.BaseDir
				stack.Partitions[partitionName] = partition
			}
			rule.Stacks[stackName] = stack
		}
		cfg.Overlays.Partitions.Rules[i] = rule
	}
}

func ApplySourceBaseDir(cfg *Config, opts LoadOptions) error {
	if cfg == nil {
		return nil
	}
	projectSources := effectiveSources(cfg.Project.Sources, Sources{})
	projectRoot, err := resolveSourcesRoot(projectSources, opts)
	if err != nil {
		return err
	}
	cfg.Project.Configs, err = applyConfigDefSources(cfg.Project.Configs, projectRoot, opts)
	if err != nil {
		return err
	}
	cfg.Project.Secrets, err = applySecretDefSources(cfg.Project.Secrets, projectRoot, opts)
	if err != nil {
		return err
	}

	for stackName, stack := range cfg.Stacks {
		stackSources := effectiveSources(stack.Sources, projectSources)
		stackRoot, err := resolveSourcesRoot(stackSources, opts)
		if err != nil {
			return err
		}
		stack.Configs.Defs, err = applyConfigDefSources(stack.Configs.Defs, stackRoot, opts)
		if err != nil {
			return err
		}
		stack.Secrets.Defs, err = applySecretDefSources(stack.Secrets.Defs, stackRoot, opts)
		if err != nil {
			return err
		}
		stack.Partitions, err = applyPartitionSources(stack.Partitions, stackSources, opts)
		if err != nil {
			return err
		}
		stack.Services, err = applyServiceSources(stack.Services, stackSources, opts)
		if err != nil {
			return err
		}
		stack.Overlays.Deployments, err = applyStackOverlaySources(stack.Overlays.Deployments, stackSources, opts)
		if err != nil {
			return err
		}
		stack.Overlays.Partitions, err = applyStackPartitionOverlaySources(stack.Overlays.Partitions, stackSources, opts)
		if err != nil {
			return err
		}
		cfg.Stacks[stackName] = stack
	}

	cfg.Overlays.Deployments, err = applyOverlaySources(cfg.Overlays.Deployments, projectSources, opts)
	if err != nil {
		return err
	}
	cfg.Overlays.Partitions, err = applyPartitionOverlaySources(cfg.Overlays.Partitions, projectSources, opts)
	if err != nil {
		return err
	}
	return nil
}

func effectiveSources(primary Sources, fallback Sources) Sources {
	if hasSources(primary) {
		return primary
	}
	if primary.Base != "" && primary.Base != fallback.Base {
		return Sources{Base: primary.Base}
	}
	if primary.Base != "" {
		fallback.Base = primary.Base
	}
	return fallback
}

func hasSources(s Sources) bool {
	return s.URL != "" || s.Ref != "" || s.Path != ""
}

func applyOverlaySources(overlays map[string]Overlay, fallback Sources, opts LoadOptions) (map[string]Overlay, error) {
	if len(overlays) == 0 {
		return overlays, nil
	}
	for name, overlay := range overlays {
		projectSources := effectiveSources(overlay.Project.Sources, fallback)
		projectRoot, err := resolveSourcesRoot(projectSources, opts)
		if err != nil {
			return nil, err
		}
		overlay.Project.Configs, err = applyConfigDefSources(overlay.Project.Configs, projectRoot, opts)
		if err != nil {
			return nil, err
		}
		overlay.Project.Secrets, err = applySecretDefSources(overlay.Project.Secrets, projectRoot, opts)
		if err != nil {
			return nil, err
		}
		for stackName, stack := range overlay.Stacks {
			stackSources := effectiveSources(stack.Sources, projectSources)
			stackRoot, err := resolveSourcesRoot(stackSources, opts)
			if err != nil {
				return nil, err
			}
			stack.Configs.Defs, err = applyConfigDefSources(stack.Configs.Defs, stackRoot, opts)
			if err != nil {
				return nil, err
			}
			stack.Secrets.Defs, err = applySecretDefSources(stack.Secrets.Defs, stackRoot, opts)
			if err != nil {
				return nil, err
			}
			for partitionName, partition := range stack.Partitions {
				partitionSources := effectiveSources(partition.Sources, stackSources)
				partitionRoot, err := resolveSourcesRoot(partitionSources, opts)
				if err != nil {
					return nil, err
				}
				partition.Configs.Defs, err = applyConfigDefSources(partition.Configs.Defs, partitionRoot, opts)
				if err != nil {
					return nil, err
				}
				partition.Secrets.Defs, err = applySecretDefSources(partition.Secrets.Defs, partitionRoot, opts)
				if err != nil {
					return nil, err
				}
				stack.Partitions[partitionName] = partition
			}
			overlay.Stacks[stackName] = stack
		}
		overlays[name] = overlay
	}
	return overlays, nil
}

func applyPartitionOverlaySources(overlays PartitionOverlays, fallback Sources, opts LoadOptions) (PartitionOverlays, error) {
	if len(overlays.Rules) == 0 {
		return overlays, nil
	}
	for i, rule := range overlays.Rules {
		overlay, err := applyOverlaySources(map[string]Overlay{"rule": rule.Overlay}, fallback, opts)
		if err != nil {
			return overlays, err
		}
		if applied, ok := overlay["rule"]; ok {
			rule.Overlay = applied
		}
		overlays.Rules[i] = rule
	}
	return overlays, nil
}

func applyStackOverlaySources(overlays map[string]OverlayStack, fallback Sources, opts LoadOptions) (map[string]OverlayStack, error) {
	if len(overlays) == 0 {
		return overlays, nil
	}
	for name, overlay := range overlays {
		stackSources := effectiveSources(overlay.Sources, fallback)
		stackRoot, err := resolveSourcesRoot(stackSources, opts)
		if err != nil {
			return nil, err
		}
		overlay.Configs.Defs, err = applyConfigDefSources(overlay.Configs.Defs, stackRoot, opts)
		if err != nil {
			return nil, err
		}
		overlay.Secrets.Defs, err = applySecretDefSources(overlay.Secrets.Defs, stackRoot, opts)
		if err != nil {
			return nil, err
		}
		for partitionName, partition := range overlay.Partitions {
			partitionSources := effectiveSources(partition.Sources, stackSources)
			partitionRoot, err := resolveSourcesRoot(partitionSources, opts)
			if err != nil {
				return nil, err
			}
			partition.Configs.Defs, err = applyConfigDefSources(partition.Configs.Defs, partitionRoot, opts)
			if err != nil {
				return nil, err
			}
			partition.Secrets.Defs, err = applySecretDefSources(partition.Secrets.Defs, partitionRoot, opts)
			if err != nil {
				return nil, err
			}
			overlay.Partitions[partitionName] = partition
		}
		overlays[name] = overlay
	}
	return overlays, nil
}

func applyStackPartitionOverlaySources(overlays StackPartitionOverlays, fallback Sources, opts LoadOptions) (StackPartitionOverlays, error) {
	if len(overlays.Rules) == 0 {
		return overlays, nil
	}
	for i, rule := range overlays.Rules {
		overlay, err := applyStackOverlaySources(map[string]OverlayStack{"rule": rule.OverlayStack}, fallback, opts)
		if err != nil {
			return overlays, err
		}
		if applied, ok := overlay["rule"]; ok {
			rule.OverlayStack = applied
		}
		overlays.Rules[i] = rule
	}
	return overlays, nil
}

func applyPartitionSources(partitions map[string]StackPartition, fallback Sources, opts LoadOptions) (map[string]StackPartition, error) {
	if len(partitions) == 0 {
		return partitions, nil
	}
	for name, partition := range partitions {
		partitionSources := effectiveSources(partition.Sources, fallback)
		partitionRoot, err := resolveSourcesRoot(partitionSources, opts)
		if err != nil {
			return nil, err
		}
		partition.Configs.Defs, err = applyConfigDefSources(partition.Configs.Defs, partitionRoot, opts)
		if err != nil {
			return nil, err
		}
		partition.Secrets.Defs, err = applySecretDefSources(partition.Secrets.Defs, partitionRoot, opts)
		if err != nil {
			return nil, err
		}
		partitions[name] = partition
	}
	return partitions, nil
}

func applyServiceSources(services map[string]Service, fallback Sources, opts LoadOptions) (map[string]Service, error) {
	if len(services) == 0 {
		return services, nil
	}
	for name, service := range services {
		serviceSources := effectiveSources(service.Sources, fallback)
		serviceRoot, err := resolveSourcesRoot(serviceSources, opts)
		if err != nil {
			return nil, err
		}
		if len(service.Configs) > 0 {
			for i, ref := range service.Configs {
				ref.Source, err = makeSourceAbsolute(ref.Source, serviceRoot, opts)
				if err != nil {
					return nil, err
				}
				service.Configs[i] = ref
			}
		}
		if len(service.Secrets) > 0 {
			for i, ref := range service.Secrets {
				ref.Source, err = makeSourceAbsolute(ref.Source, serviceRoot, opts)
				if err != nil {
					return nil, err
				}
				service.Secrets[i] = ref
			}
		}
		services[name] = service
	}
	return services, nil
}

func applyConfigDefSources(defs map[string]ConfigDef, root string, opts LoadOptions) (map[string]ConfigDef, error) {
	if len(defs) == 0 {
		return defs, nil
	}
	for name, def := range defs {
		var err error
		def.Source, err = makeSourceAbsolute(def.Source, root, opts)
		if err != nil {
			return nil, err
		}
		defs[name] = def
	}
	return defs, nil
}

func applySecretDefSources(defs map[string]SecretDef, root string, opts LoadOptions) (map[string]SecretDef, error) {
	if len(defs) == 0 {
		return defs, nil
	}
	for name, def := range defs {
		var err error
		def.Source, err = makeSourceAbsolute(def.Source, root, opts)
		if err != nil {
			return nil, err
		}
		defs[name] = def
	}
	return defs, nil
}

func makeSourceAbsolute(source string, root string, opts LoadOptions) (string, error) {
	if source == "" || root == "" {
		return source, nil
	}
	if strings.HasPrefix(source, "inline:") || strings.HasPrefix(source, valuesPrefix) {
		return source, nil
	}
	base, fragment := splitSource(source)
	if base == "" {
		return source, nil
	}
	resolved, err := resolvePathWithin(root, base, opts)
	if err != nil {
		return "", err
	}
	return resolved + fragment, nil
}

func splitSource(source string) (string, string) {
	if source == "" {
		return source, ""
	}
	if idx := strings.Index(source, "#"); idx != -1 {
		return source[:idx], source[idx:]
	}
	return source, ""
}

func resolveSourcesRoot(s Sources, opts LoadOptions) (string, error) {
	base := s.Base
	if base == "" {
		return "", nil
	}
	if s.URL != "" {
		repoRoot, err := FetchRepoRoot(s.URL, s.Ref, opts)
		if err != nil {
			return "", err
		}
		if s.Path == "" {
			return repoRoot, nil
		}
		return resolvePathWithin(repoRoot, s.Path, opts)
	}
	if s.Path == "" {
		return base, nil
	}
	if filepath.IsAbs(s.Path) {
		return resolveAbsolutePath(s.Path)
	}
	return resolvePathWithin(base, s.Path, opts)
}

// ResolveSourcesRoot resolves the effective root path for a Sources definition.
func ResolveSourcesRoot(s Sources, opts LoadOptions) (string, error) {
	return resolveSourcesRoot(s, opts)
}

func resolvePathWithin(root string, path string, opts LoadOptions) (string, error) {
	if root == "" {
		return "", fmt.Errorf("source root is empty")
	}
	if path == "" {
		return root, nil
	}
	if IsGitSource(root) {
		return resolveGitPathWithin(root, path, opts)
	}
	candidate := path
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(root, path)
	}
	rootEval, err := filepath.EvalSymlinks(root)
	if err != nil {
		rootEval = filepath.Clean(root)
	}
	candidateEval, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootEval, candidateEval)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("source path %q escapes root %q", path, root)
	}
	if _, err := os.Stat(candidateEval); err != nil {
		return "", err
	}
	return candidateEval, nil
}
