package cmdutil

import (
	"path/filepath"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/fsutil"
)

func InferValuesFiles(configPath string, values []string) []string {
	if len(values) > 0 {
		return values
	}
	base := filepath.Dir(configPath)
	candidates := []string{
		filepath.Join(base, "values", "values.yaml.tmpl"),
		filepath.Join(base, "values", "values.yml.tmpl"),
		filepath.Join(base, "values", "values.json.tmpl"),
		filepath.Join(base, "values", "values.yaml"),
		filepath.Join(base, "values", "values.yml"),
		filepath.Join(base, "values", "values.json"),
	}
	for _, candidate := range candidates {
		if fsutil.FileExists(candidate) {
			return []string{candidate}
		}
	}
	return values
}

func InferSecretsFile(cfg *config.Config, configPath string, secretsFile string) string {
	if secretsFile != "" {
		return secretsFile
	}
	if cfg != nil && cfg.Project.SecretsEngine != nil {
		return ""
	}
	base := filepath.Dir(configPath)
	candidates := []string{
		filepath.Join(base, "secrets.yaml"),
		filepath.Join(base, "secrets.yml"),
	}
	for _, candidate := range candidates {
		if fsutil.FileExists(candidate) {
			return candidate
		}
	}
	return ""
}
