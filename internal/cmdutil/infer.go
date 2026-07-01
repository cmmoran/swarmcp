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

func InferValuesSources(cfg *config.Config, configPath string, values []string) ([]string, error) {
	if len(values) > 0 {
		return values, nil
	}
	if cfg != nil && len(cfg.Project.Values) > 0 {
		out := make([]string, 0, len(cfg.Project.Values))
		for _, ref := range cfg.Project.Values {
			source, err := config.ResolveSourceRef(config.SourceRef{URL: ref.URL, Ref: ref.Ref, Path: ref.Path}, cfg.BaseDir, config.LoadOptions{CacheDir: cfg.CacheDir, Offline: cfg.Offline, Debug: cfg.Debug})
			if err != nil {
				return nil, err
			}
			out = append(out, source)
		}
		return out, nil
	}
	return InferValuesFiles(configPath, values), nil
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
