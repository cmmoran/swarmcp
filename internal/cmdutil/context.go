package cmdutil

import "github.com/cmmoran/swarmcp/internal/config"

func ResolveContext(cfg *config.Config, override string) string {
	if override != "" {
		return override
	}
	if cfg.Project.Deployment == "" {
		return ""
	}
	if cfg.Project.Contexts == nil {
		return ""
	}
	return cfg.Project.Contexts[cfg.Project.Deployment]
}

func PartitionInProject(cfg *config.Config, name string) bool {
	for _, partition := range cfg.Project.Partitions {
		if partition == name {
			return true
		}
	}
	return false
}

func DeploymentPartitions(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}
	deployment := cfg.Project.Deployment
	if deployment == "" {
		return append([]string(nil), cfg.Project.Partitions...)
	}
	target, ok := cfg.Project.Targets[deployment]
	if !ok || len(target.Partitions) == 0 {
		return append([]string(nil), cfg.Project.Partitions...)
	}
	return append([]string(nil), target.Partitions...)
}

func PartitionAllowedForDeployment(cfg *config.Config, partition string) bool {
	if partition == "" {
		return true
	}
	if !PartitionInProject(cfg, partition) {
		return false
	}
	allowed := DeploymentPartitions(cfg)
	if len(allowed) == 0 {
		return true
	}
	for _, candidate := range allowed {
		if candidate == partition {
			return true
		}
	}
	return false
}

func FilterDeploymentPartitions(cfg *config.Config, filters []string) []string {
	allowed := DeploymentPartitions(cfg)
	if len(filters) == 0 {
		return allowed
	}
	if len(allowed) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(filters))
	for _, filter := range filters {
		if filter == "" {
			continue
		}
		set[filter] = struct{}{}
	}
	var out []string
	for _, partition := range allowed {
		if _, ok := set[partition]; ok {
			out = append(out, partition)
		}
	}
	return out
}

func StackInProject(cfg *config.Config, name string) bool {
	if cfg == nil {
		return false
	}
	if name == "" {
		return false
	}
	_, ok := cfg.Stacks[name]
	return ok
}
