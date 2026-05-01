package config

import (
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v4"
)

func LoadFiles(paths []string) (*Config, error) {
	return LoadFilesWithOptions(paths, LoadOptions{})
}

func LoadFilesWithOptions(paths []string, opts LoadOptions) (*Config, error) {
	return LoadFilesWithReleaseOptions(paths, nil, opts)
}

func LoadFilesWithReleaseOptions(paths []string, releasePaths []string, opts LoadOptions) (*Config, error) {
	cfg, _, err := loadFilesWithReleaseTrace(paths, releasePaths, opts, nil)
	return cfg, err
}

func LoadFilesWithReleaseTrace(paths []string, releasePaths []string, opts LoadOptions, fieldPath []string) (*Config, *LoadTrace, error) {
	trace := &LoadTrace{FieldPath: append([]string(nil), fieldPath...)}
	return loadFilesWithReleaseTrace(paths, releasePaths, opts, trace)
}

func loadFilesWithReleaseTrace(paths []string, releasePaths []string, opts LoadOptions, trace *LoadTrace) (*Config, *LoadTrace, error) {
	if len(paths) == 0 {
		return nil, nil, fmt.Errorf("project config is required")
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
			return nil, nil, err
		}
		trace.record("config "+absPath, doc)
		absPaths = append(absPaths, absPath)
		if i == 0 {
			merged = doc
			continue
		}
		if err := validateLayeredOverlayMap(doc, nil); err != nil {
			return nil, nil, fmt.Errorf("config %q: %w", absPath, err)
		}
		merged, err = mergeLayeredConfigMaps(merged, doc, nil)
		if err != nil {
			return nil, nil, fmt.Errorf("config %q: %w", absPath, err)
		}
	}
	for _, path := range releasePaths {
		absPath := path
		if abs, err := filepath.Abs(path); err == nil {
			absPath = abs
		}
		doc, err := loadConfigDocument(absPath)
		if err != nil {
			return nil, nil, err
		}
		trace.record("release config "+absPath, doc)
		if err := validateReleaseOverlayShape(doc); err != nil {
			return nil, nil, fmt.Errorf("release config %q: %w", absPath, err)
		}
		if err := validateReleaseSourceRefs(merged, doc); err != nil {
			return nil, nil, fmt.Errorf("release config %q: %w", absPath, err)
		}
		sourceRefs := releaseSourceRefOverlay(doc)
		if len(sourceRefs) == 0 {
			continue
		}
		merged, err = mergeReleaseOverlayMap(merged, sourceRefs)
		if err != nil {
			return nil, nil, fmt.Errorf("release config %q: %w", absPath, err)
		}
	}

	encoded, err := yaml.Marshal(merged)
	if err != nil {
		return nil, nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(encoded, &cfg); err != nil {
		return nil, nil, err
	}

	SetBaseDir(&cfg, absPaths[0])
	opts = normalizeLoadOptions(opts, cfg.BaseDir)
	cfg.CacheDir = opts.CacheDir
	cfg.Offline = opts.Offline
	cfg.Debug = opts.Debug
	if err := resolveImportsWithTrace(&cfg, opts, trace); err != nil {
		return nil, nil, err
	}
	for _, path := range releasePaths {
		absPath := path
		if abs, err := filepath.Abs(path); err == nil {
			absPath = abs
		}
		doc, err := loadConfigDocument(absPath)
		if err != nil {
			return nil, nil, err
		}
		if err := applyReleaseServiceOverlay(&cfg, doc); err != nil {
			return nil, nil, fmt.Errorf("release config %q: %w", absPath, err)
		}
	}
	SetSourcesBaseDir(&cfg)
	if err := ApplySourceBaseDir(&cfg, opts); err != nil {
		return nil, nil, err
	}
	if err := Validate(&cfg); err != nil {
		return nil, nil, err
	}
	if trace != nil {
		finalDoc, err := configToDocument(&cfg)
		if err != nil {
			return nil, nil, err
		}
		trace.MergedDoc = finalDoc
	}

	return &cfg, trace, nil
}

func configToDocument(cfg *Config) (map[string]any, error) {
	encoded, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	return parseNormalizedYAMLDocument(encoded)
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
	return parseNormalizedYAMLDocument(data)
}

func mergeLayeredConfigMaps(base map[string]any, overlay map[string]any, path []string) (map[string]any, error) {
	out := cloneValue(base).(map[string]any)
	for key, overlayValue := range overlay {
		nextPath := appendPath(path, key)
		switch layeredPolicyForPath(nextPath) {
		case layeredPolicyInvalid:
			return nil, fmt.Errorf("%s is not allowed in later config files", joinPath(nextPath))
		case layeredPolicyReplace:
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
		switch layeredPolicyForPath(nextPath) {
		case layeredPolicyInvalid:
			return fmt.Errorf("%s is not allowed in later config files", joinPath(nextPath))
		case layeredPolicyReplace:
			continue
		}
		overlayMap, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if err := validateLayeredOverlayMap(overlayMap, nextPath); err != nil {
			return err
		}
	}
	return nil
}

func mergeLayeredConfigValues(base any, overlay any, path []string) (any, error) {
	if layeredPolicyForPath(path) == layeredPolicyReplace {
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

func validateReleaseOverlayShape(overlay map[string]any) error {
	return validateReleaseOverlayNode(nil, overlay, nil, releasePolicyRoot, false)
}

func validateReleaseOverlayMap(base map[string]any, overlay map[string]any, path []string) error {
	return validateReleaseOverlayNode(base, overlay, nil, releasePolicyRoot, true)
}

func validateReleaseOverlayNode(base map[string]any, value any, path []string, node *releasePolicyNode, requireExisting bool) error {
	if node == nil {
		return fmt.Errorf("%s is not allowed in release config files", joinPath(path))
	}
	if requireExisting && node.requireExistingMap {
		if _, ok := lookupExistingMap(base, path); !ok {
			return fmt.Errorf("%s does not exist in the base config", joinPath(path))
		}
	}
	switch node.kind {
	case releaseValueMap:
		mapped, err := requireMap(value, path)
		if err != nil {
			return err
		}
		for key, childValue := range mapped {
			nextPath := appendPath(path, key)
			child := releasePolicyChild(node, key)
			if child == nil {
				return fmt.Errorf("%s is not allowed in release config files", joinPath(nextPath))
			}
			if err := validateReleaseOverlayNode(base, childValue, nextPath, child, requireExisting); err != nil {
				return err
			}
		}
		return nil
	case releaseValueScalar:
		if _, ok := value.(map[string]any); ok {
			return fmt.Errorf("%s must be a scalar value", joinPath(path))
		}
		return nil
	case releaseValueScalarMap:
		mapped, err := requireMap(value, path)
		if err != nil {
			return err
		}
		return validateReleaseScalarMap(mapped, path)
	case releaseValueUpdatePolicyMap:
		mapped, err := requireMap(value, path)
		if err != nil {
			return err
		}
		return validateReleaseUpdatePolicyMap(mapped, path)
	default:
		return fmt.Errorf("%s is not allowed in release config files", joinPath(path))
	}
}

func validateReleaseSourceRefs(base map[string]any, release map[string]any) error {
	stacks, ok := release["stacks"].(map[string]any)
	if !ok {
		return nil
	}
	for stackName, stackValue := range stacks {
		stackMap, ok := stackValue.(map[string]any)
		if !ok {
			continue
		}
		sourceMap, ok := stackMap["source"].(map[string]any)
		if !ok {
			continue
		}
		if _, hasRef := sourceMap["ref"]; !hasRef {
			continue
		}
		stackPath := []string{"stacks", stackName}
		if _, ok := lookupExistingMap(base, stackPath); !ok {
			return fmt.Errorf("%s does not exist in the base config", joinPath(stackPath))
		}
		sourcePath := []string{"stacks", stackName, "source"}
		if _, ok := lookupExistingMap(base, sourcePath); !ok {
			return fmt.Errorf("%s does not exist in the base config", joinPath(sourcePath))
		}
	}
	return nil
}

func releaseSourceRefOverlay(release map[string]any) map[string]any {
	stacks, ok := release["stacks"].(map[string]any)
	if !ok {
		return nil
	}
	outStacks := map[string]any{}
	for stackName, stackValue := range stacks {
		stackMap, ok := stackValue.(map[string]any)
		if !ok {
			continue
		}
		sourceMap, ok := stackMap["source"].(map[string]any)
		if !ok {
			continue
		}
		ref, hasRef := sourceMap["ref"]
		if !hasRef {
			continue
		}
		outStacks[stackName] = map[string]any{
			"source": map[string]any{
				"ref": cloneValue(ref),
			},
		}
	}
	if len(outStacks) == 0 {
		return nil
	}
	return map[string]any{"stacks": outStacks}
}

func applyReleaseServiceOverlay(cfg *Config, release map[string]any) error {
	stacks, ok := release["stacks"].(map[string]any)
	if !ok {
		return nil
	}
	for stackName, stackValue := range stacks {
		stackMap, ok := stackValue.(map[string]any)
		if !ok {
			continue
		}
		services, ok := stackMap["services"].(map[string]any)
		if !ok {
			continue
		}
		stack, ok := cfg.Stacks[stackName]
		if !ok {
			return fmt.Errorf("stacks.%s does not exist in the resolved config", stackName)
		}
		for serviceName, serviceValue := range services {
			serviceOverlay, ok := serviceValue.(map[string]any)
			if !ok {
				return fmt.Errorf("stacks.%s.services.%s must be a mapping", stackName, serviceName)
			}
			service, ok := stack.Services[serviceName]
			if !ok {
				return fmt.Errorf("stacks.%s.services.%s does not exist in the resolved config", stackName, serviceName)
			}
			mergedService, err := mergeReleaseService(service, serviceOverlay)
			if err != nil {
				return fmt.Errorf("stacks.%s.services.%s: %w", stackName, serviceName, err)
			}
			stack.Services[serviceName] = mergedService
		}
		cfg.Stacks[stackName] = stack
	}
	return nil
}

func mergeReleaseService(base Service, overlay map[string]any) (Service, error) {
	encoded, err := yaml.Marshal(overlay)
	if err != nil {
		return Service{}, err
	}
	var patch Service
	if err := yaml.Unmarshal(encoded, &patch); err != nil {
		return Service{}, err
	}
	if _, ok := overlay["image"]; ok {
		base.Image = patch.Image
	}
	if _, ok := overlay["replicas"]; ok {
		base.Replicas = patch.Replicas
	}
	if patch.Env != nil {
		if base.Env == nil {
			base.Env = map[string]string{}
		}
		for key, value := range patch.Env {
			base.Env[key] = value
		}
	}
	if patch.Labels != nil {
		if base.Labels == nil {
			base.Labels = map[string]string{}
		}
		for key, value := range patch.Labels {
			base.Labels[key] = value
		}
	}
	if patch.UpdateConfig != nil {
		base.UpdateConfig = MergeUpdatePolicies(base.UpdateConfig, patch.UpdateConfig)
	}
	if patch.RollbackConfig != nil {
		base.RollbackConfig = MergeUpdatePolicies(base.RollbackConfig, patch.RollbackConfig)
	}
	return base, nil
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
