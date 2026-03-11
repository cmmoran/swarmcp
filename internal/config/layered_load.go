package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cmmoran/swarmcp/internal/yamlutil"
	"gopkg.in/yaml.v3"
)

func LoadFiles(paths []string) (*Config, error) {
	return LoadFilesWithOptions(paths, LoadOptions{})
}

func LoadFilesWithOptions(paths []string, opts LoadOptions) (*Config, error) {
	return LoadFilesWithReleaseOptions(paths, nil, opts)
}

func LoadFilesWithReleaseOptions(paths []string, releasePaths []string, opts LoadOptions) (*Config, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("project config is required")
	}
	absPaths := make([]string, 0, len(paths))
	var merged map[string]any
	for i, path := range paths {
		absPath := path
		if abs, err := filepath.Abs(path); err == nil {
			absPath = abs
		}
		doc, err := loadConfigDocument(absPath)
		if err != nil {
			return nil, err
		}
		absPaths = append(absPaths, absPath)
		if i == 0 {
			merged = doc
			continue
		}
		if err := validateLayeredOverlayMap(doc, nil); err != nil {
			return nil, fmt.Errorf("config %q: %w", absPath, err)
		}
		merged, err = mergeLayeredConfigMaps(merged, doc, nil)
		if err != nil {
			return nil, fmt.Errorf("config %q: %w", absPath, err)
		}
	}
	for _, path := range releasePaths {
		absPath := path
		if abs, err := filepath.Abs(path); err == nil {
			absPath = abs
		}
		doc, err := loadConfigDocument(absPath)
		if err != nil {
			return nil, err
		}
		if err := validateReleaseOverlayMap(merged, doc, nil); err != nil {
			return nil, fmt.Errorf("release config %q: %w", absPath, err)
		}
		merged, err = mergeReleaseOverlayMap(merged, doc)
		if err != nil {
			return nil, fmt.Errorf("release config %q: %w", absPath, err)
		}
	}

	encoded, err := yaml.Marshal(merged)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(encoded, &cfg); err != nil {
		return nil, err
	}

	SetBaseDir(&cfg, absPaths[0])
	opts = normalizeLoadOptions(opts, cfg.BaseDir)
	cfg.CacheDir = opts.CacheDir
	cfg.Offline = opts.Offline
	cfg.Debug = opts.Debug
	if err := ResolveImports(&cfg, opts); err != nil {
		return nil, err
	}
	SetSourcesBaseDir(&cfg)
	if err := ApplySourceBaseDir(&cfg, opts); err != nil {
		return nil, err
	}
	if err := Validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func mergeReleaseOverlayMap(base map[string]any, overlay map[string]any) (map[string]any, error) {
	merged, err := mergeReleaseOverlayValue(base, overlay)
	if err != nil {
		return nil, err
	}
	mapped, ok := merged.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("release overlay root is not a mapping")
	}
	return mapped, nil
}

func mergeReleaseOverlayValue(base any, overlay any) (any, error) {
	overlayMap, ok := overlay.(map[string]any)
	if !ok {
		return cloneValue(overlay), nil
	}
	baseMap, ok := base.(map[string]any)
	if !ok {
		return cloneValue(overlayMap), nil
	}
	out := cloneValue(baseMap).(map[string]any)
	for key, overlayValue := range overlayMap {
		baseValue, exists := out[key]
		if !exists {
			out[key] = cloneValue(overlayValue)
			continue
		}
		merged, err := mergeReleaseOverlayValue(baseValue, overlayValue)
		if err != nil {
			return nil, err
		}
		out[key] = merged
	}
	return out, nil
}

func loadConfigDocument(configPath string) (map[string]any, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	normalized := normalizeTemplateScalars(string(data))
	var parsed any
	if err := yaml.Unmarshal([]byte(normalized), &parsed); err != nil {
		return nil, err
	}
	value := yamlutil.NormalizeValue(parsed)
	if value == nil {
		return map[string]any{}, nil
	}
	mapped, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("root is not a mapping")
	}
	return mapped, nil
}

func mergeLayeredConfigMaps(base map[string]any, overlay map[string]any, path []string) (map[string]any, error) {
	out := cloneValue(base).(map[string]any)
	for key, overlayValue := range overlay {
		nextPath := appendPath(path, key)
		if invalidLayeredConfigPath(nextPath) {
			return nil, fmt.Errorf("%s is not allowed in later config files", joinPath(nextPath))
		}
		if replaceLayeredConfigPath(nextPath) {
			out[key] = cloneValue(overlayValue)
			continue
		}
		baseValue, ok := out[key]
		if !ok {
			out[key] = cloneValue(overlayValue)
			continue
		}
		merged, err := mergeLayeredConfigValues(baseValue, overlayValue, nextPath)
		if err != nil {
			return nil, err
		}
		out[key] = merged
	}
	return out, nil
}

func validateLayeredOverlayMap(overlay map[string]any, path []string) error {
	for key, value := range overlay {
		nextPath := appendPath(path, key)
		if invalidLayeredConfigPath(nextPath) {
			return fmt.Errorf("%s is not allowed in later config files", joinPath(nextPath))
		}
		overlayMap, ok := value.(map[string]any)
		if !ok || replaceLayeredConfigPath(nextPath) {
			continue
		}
		if err := validateLayeredOverlayMap(overlayMap, nextPath); err != nil {
			return err
		}
	}
	return nil
}

func mergeLayeredConfigValues(base any, overlay any, path []string) (any, error) {
	if replaceLayeredConfigPath(path) {
		return cloneValue(overlay), nil
	}
	overlayMap, ok := overlay.(map[string]any)
	if !ok {
		return cloneValue(overlay), nil
	}
	baseMap, ok := base.(map[string]any)
	if !ok {
		return cloneValue(overlayMap), nil
	}
	return mergeLayeredConfigMaps(baseMap, overlayMap, path)
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = cloneValue(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = cloneValue(item)
		}
		return out
	default:
		return typed
	}
}

func appendPath(path []string, key string) []string {
	next := make([]string, 0, len(path)+1)
	next = append(next, path...)
	next = append(next, key)
	return next
}

func joinPath(path []string) string {
	if len(path) == 0 {
		return "(root)"
	}
	out := path[0]
	for i := 1; i < len(path); i++ {
		out += "." + path[i]
	}
	return out
}

func invalidLayeredConfigPath(path []string) bool {
	return pathMatches(path, "stacks", "*", "overrides") ||
		pathMatches(path, "stacks", "*", "services", "*", "overrides")
}

func replaceLayeredConfigPath(path []string) bool {
	return pathMatches(path, "project", "name") ||
		pathMatches(path, "project", "deployment") ||
		pathMatches(path, "project", "restart_policy") ||
		pathMatches(path, "project", "update_config") ||
		pathMatches(path, "project", "rollback_config") ||
		pathMatches(path, "project", "secrets_engine") ||
		pathMatches(path, "project", "preserve_unused_resources") ||
		pathMatches(path, "project", "partitions") ||
		pathMatches(path, "project", "deployments") ||
		pathMatches(path, "project", "defaults", "networks", "shared") ||
		pathMatches(path, "project", "defaults", "networks", "internal") ||
		pathMatches(path, "project", "defaults", "networks", "egress") ||
		pathMatches(path, "project", "defaults", "networks", "attachable") ||
		pathMatches(path, "project", "defaults", "volumes", "driver") ||
		pathMatches(path, "project", "defaults", "volumes", "base_path") ||
		pathMatches(path, "project", "defaults", "volumes", "layout") ||
		pathMatches(path, "project", "defaults", "volumes", "node_label_key") ||
		pathMatches(path, "project", "defaults", "volumes", "service_standard") ||
		pathMatches(path, "project", "defaults", "volumes", "service_target") ||
		pathMatches(path, "project", "nodes", "*", "roles") ||
		pathMatches(path, "project", "nodes", "*", "volumes") ||
		pathMatches(path, "project", "deployment_targets", "*", "include", "names") ||
		pathMatches(path, "project", "deployment_targets", "*", "exclude", "names") ||
		pathMatches(path, "project", "deployment_targets", "*", "overrides", "*", "roles") ||
		pathMatches(path, "project", "deployment_targets", "*", "overrides", "*", "volumes") ||
		pathMatches(path, "stacks", "*", "source") ||
		pathMatches(path, "stacks", "*", "mode") ||
		pathMatches(path, "stacks", "*", "restart_policy") ||
		pathMatches(path, "stacks", "*", "update_config") ||
		pathMatches(path, "stacks", "*", "rollback_config") ||
		pathMatches(path, "stacks", "*", "partitions", "*", "restart_policy") ||
		pathMatches(path, "stacks", "*", "partitions", "*", "update_config") ||
		pathMatches(path, "stacks", "*", "partitions", "*", "rollback_config") ||
		pathMatches(path, "stacks", "*", "services", "*", "source") ||
		pathMatches(path, "stacks", "*", "services", "*", "image") ||
		pathMatches(path, "stacks", "*", "services", "*", "command") ||
		pathMatches(path, "stacks", "*", "services", "*", "args") ||
		pathMatches(path, "stacks", "*", "services", "*", "workdir") ||
		pathMatches(path, "stacks", "*", "services", "*", "ports") ||
		pathMatches(path, "stacks", "*", "services", "*", "mode") ||
		pathMatches(path, "stacks", "*", "services", "*", "replicas") ||
		pathMatches(path, "stacks", "*", "services", "*", "restart_policy") ||
		pathMatches(path, "stacks", "*", "services", "*", "update_config") ||
		pathMatches(path, "stacks", "*", "services", "*", "rollback_config") ||
		pathMatches(path, "stacks", "*", "services", "*", "healthcheck") ||
		pathMatches(path, "stacks", "*", "services", "*", "depends_on") ||
		pathMatches(path, "stacks", "*", "services", "*", "egress") ||
		pathMatches(path, "stacks", "*", "services", "*", "networks") ||
		pathMatches(path, "stacks", "*", "services", "*", "network_ephemeral") ||
		pathMatches(path, "stacks", "*", "services", "*", "configs") ||
		pathMatches(path, "stacks", "*", "services", "*", "secrets") ||
		pathMatches(path, "stacks", "*", "services", "*", "volumes")
}

func pathMatches(path []string, pattern ...string) bool {
	if len(path) != len(pattern) {
		return false
	}
	for i := range pattern {
		if pattern[i] == "*" {
			continue
		}
		if path[i] != pattern[i] {
			return false
		}
	}
	return true
}

func validateReleaseOverlayMap(base map[string]any, overlay map[string]any, path []string) error {
	for key, value := range overlay {
		nextPath := appendPath(path, key)
		if err := validateReleaseOverlayEntry(base, value, nextPath); err != nil {
			return err
		}
	}
	return nil
}

func validateReleaseOverlayEntry(base map[string]any, value any, path []string) error {
	switch {
	case len(path) == 1:
		if path[0] != "stacks" {
			return fmt.Errorf("%s is not allowed in release config files", joinPath(path))
		}
		mapped, err := requireMap(value, path)
		if err != nil {
			return err
		}
		return validateReleaseOverlayMap(base, mapped, path)
	case len(path) == 2 && path[0] == "stacks":
		if _, ok := lookupExistingMap(base, []string{"stacks", path[1]}); !ok {
			return fmt.Errorf("%s does not exist in the base config", joinPath(path))
		}
		mapped, err := requireMap(value, path)
		if err != nil {
			return err
		}
		return validateReleaseOverlayMap(base, mapped, path)
	case len(path) == 3 && path[0] == "stacks":
		switch path[2] {
		case "source", "services":
			mapped, err := requireMap(value, path)
			if err != nil {
				return err
			}
			return validateReleaseOverlayMap(base, mapped, path)
		default:
			return fmt.Errorf("%s is not allowed in release config files", joinPath(path))
		}
	case len(path) == 4 && path[0] == "stacks" && path[2] == "source":
		if path[3] != "ref" {
			return fmt.Errorf("%s is not allowed in release config files", joinPath(path))
		}
		if _, ok := lookupExistingMap(base, path[:3]); !ok {
			return fmt.Errorf("%s does not exist in the base config", joinPath(path[:3]))
		}
		if _, ok := value.(map[string]any); ok {
			return fmt.Errorf("%s must be a scalar value", joinPath(path))
		}
		return nil
	case len(path) == 4 && path[0] == "stacks" && path[2] == "services":
		serviceName := path[3]
		if _, ok := lookupExistingMap(base, []string{"stacks", path[1], "services", serviceName}); !ok {
			return fmt.Errorf("%s does not exist in the base config", joinPath(path))
		}
		mapped, err := requireMap(value, path)
		if err != nil {
			return err
		}
		return validateReleaseOverlayMap(base, mapped, path)
	case len(path) == 5 && path[0] == "stacks" && path[2] == "services":
		switch path[4] {
		case "image", "replicas":
			if _, ok := value.(map[string]any); ok {
				return fmt.Errorf("%s must be a scalar value", joinPath(path))
			}
			return nil
		case "env", "labels":
			mapped, err := requireMap(value, path)
			if err != nil {
				return err
			}
			return validateReleaseScalarMap(mapped, path)
		case "update_config", "rollback_config":
			mapped, err := requireMap(value, path)
			if err != nil {
				return err
			}
			return validateReleaseUpdatePolicyMap(mapped, path)
		default:
			return fmt.Errorf("%s is not allowed in release config files", joinPath(path))
		}
	default:
		return fmt.Errorf("%s is not allowed in release config files", joinPath(path))
	}
}

func requireMap(value any, path []string) (map[string]any, error) {
	mapped, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a mapping", joinPath(path))
	}
	return mapped, nil
}

func lookupExistingMap(base map[string]any, path []string) (map[string]any, bool) {
	current := any(base)
	for _, segment := range path {
		mapped, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := mapped[segment]
		if !ok {
			return nil, false
		}
		current = next
	}
	mapped, ok := current.(map[string]any)
	return mapped, ok
}

func validateReleaseScalarMap(mapped map[string]any, path []string) error {
	for key, value := range mapped {
		if key == "" {
			return fmt.Errorf("%s contains an empty key", joinPath(path))
		}
		if _, ok := value.(map[string]any); ok {
			return fmt.Errorf("%s.%s must be a scalar value", joinPath(path), key)
		}
	}
	return nil
}

func validateReleaseUpdatePolicyMap(mapped map[string]any, path []string) error {
	for key, value := range mapped {
		switch key {
		case "parallelism", "delay", "failure_action", "monitor", "max_failure_ratio", "order":
		default:
			return fmt.Errorf("%s.%s is not allowed in release config files", joinPath(path), key)
		}
		if _, ok := value.(map[string]any); ok {
			return fmt.Errorf("%s.%s must be a scalar value", joinPath(path), key)
		}
	}
	return nil
}
