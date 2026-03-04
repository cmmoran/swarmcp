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
