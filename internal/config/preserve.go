package config

const DefaultPreserveUnusedResources = 5

func PreserveUnusedResources(cfg *Config) int {
	if cfg == nil {
		return DefaultPreserveUnusedResources
	}
	if cfg.Project.PreserveUnusedResources != nil {
		return *cfg.Project.PreserveUnusedResources
	}
	return DefaultPreserveUnusedResources
}
