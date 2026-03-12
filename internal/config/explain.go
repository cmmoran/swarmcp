package config

import (
	"fmt"
	"sort"
	"strings"
)

type ExplainLayer struct {
	Label string
	Value any
}

type ExplainResult struct {
	Path   string
	Final  any
	Layers []ExplainLayer
	Winner string
}

type ExplainOptions struct {
	ConfigPaths        []string
	ReleaseConfigPaths []string
	Deployment         string
	Partition          string
	Stack              string
	LoadOptions        LoadOptions
}

func ExplainConfigPath(opts ExplainOptions, fieldPath string) (*ExplainResult, error) {
	segments := splitFieldPath(fieldPath)
	if len(segments) == 0 {
		return nil, fmt.Errorf("field path is required")
	}
	resolvedModel, err := LoadResolvedModel(ResolvedModelOptions{
		ConfigPaths:        opts.ConfigPaths,
		ReleaseConfigPaths: opts.ReleaseConfigPaths,
		Deployment:         opts.Deployment,
		Partition:          opts.Partition,
		Stack:              opts.Stack,
		LoadOptions:        opts.LoadOptions,
	})
	if err != nil {
		return nil, err
	}
	cfg := resolvedModel.Config
	resolved := resolvedModel.Model
	finalValue, ok, detail := lookupPathValueDetailed(resolved, segments)
	if !ok {
		if detail != "" {
			return nil, fmt.Errorf("invalid field path %q: %s", fieldPath, detail)
		}
		suggestions := explainPathSuggestions(resolved, segments)
		if len(suggestions) == 0 {
			return nil, fmt.Errorf("field path %q not found", fieldPath)
		}
		return nil, fmt.Errorf("field path %q not found (available: %s)", fieldPath, strings.Join(suggestions, ", "))
	}

	docLayers, mergedDoc, err := explainConfigDocumentLayers(opts.ConfigPaths, opts.ReleaseConfigPaths, segments)
	if err != nil {
		return nil, err
	}
	layers := append([]ExplainLayer(nil), docLayers...)
	layers = append(layers, explainImportLayers(cfg, mergedDoc, segments, opts.Partition, opts.LoadOptions)...)
	layers = append(layers, explainOverlayLayers(cfg, segments, opts.Partition)...)
	layers = filterExplainLayers(layers, finalValue)
	if len(layers) == 0 {
		layers = []ExplainLayer{{Label: "resolved config", Value: finalValue}}
	}

	return &ExplainResult{
		Path:   fieldPath,
		Final:  finalValue,
		Layers: layers,
		Winner: layers[len(layers)-1].Label,
	}, nil
}

func explainConfigDocumentLayers(paths []string, releasePaths []string, fieldPath []string) ([]ExplainLayer, map[string]any, error) {
	var layers []ExplainLayer
	var merged map[string]any
	for i, path := range paths {
		doc, err := loadConfigDocument(path)
		if err != nil {
			return nil, nil, err
		}
		if value, ok := lookupPathValue(doc, fieldPath); ok {
			layers = append(layers, ExplainLayer{
				Label: "config " + path,
				Value: value,
			})
		}
		if i == 0 {
			merged = doc
			continue
		}
		merged, err = mergeLayeredConfigMaps(merged, doc, nil)
		if err != nil {
			return nil, nil, err
		}
	}
	for _, path := range releasePaths {
		doc, err := loadConfigDocument(path)
		if err != nil {
			return nil, nil, err
		}
		if value, ok := lookupPathValue(doc, fieldPath); ok {
			layers = append(layers, ExplainLayer{
				Label: "release config " + path,
				Value: value,
			})
		}
		merged, err = mergeReleaseOverlayMap(merged, doc)
		if err != nil {
			return nil, nil, err
		}
	}
	return layers, merged, nil
}

func explainImportLayers(cfg *Config, mergedDoc map[string]any, fieldPath []string, partition string, loadOpts LoadOptions) []ExplainLayer {
	if len(fieldPath) < 2 || fieldPath[0] != "stacks" {
		return nil
	}
	stackName := fieldPath[1]
	stackMap, ok := lookupPathMap(mergedDoc, []string{"stacks", stackName})
	if !ok {
		return nil
	}

	if len(fieldPath) >= 4 && fieldPath[2] == "services" {
		serviceName := fieldPath[3]
		relPath := fieldPath[4:]
		stackResolved, stackLayers, err := explainImportedStack(stackMap, cfg.BaseDir, append([]string{"services", serviceName}, relPath...), loadOpts)
		if err == nil {
			if serviceMap, ok := lookupPathMap(stackResolved, []string{"services", serviceName}); ok {
				serviceLayers, _ := explainImportedService(serviceMap, stackResolvedBaseDir(stackResolved, cfg.BaseDir), relPath, loadOpts)
				return append(stackLayers, serviceLayers...)
			}
			return stackLayers
		}
		return nil
	}

	relPath := fieldPath[2:]
	_, stackLayers, err := explainImportedStack(stackMap, cfg.BaseDir, relPath, loadOpts)
	if err != nil {
		return nil
	}
	return stackLayers
}

func explainImportedStack(stackMap map[string]any, baseDir string, relPath []string, loadOpts LoadOptions) (map[string]any, []ExplainLayer, error) {
	sourceMap, ok := lookupPathMap(stackMap, []string{"source"})
	if !ok {
		return stackMap, nil, nil
	}
	ref, err := decodeSourceRefMap(sourceMap)
	if err != nil {
		return nil, nil, err
	}
	baseMap, basePath, err := loadSourceMap(ref, baseDir, loadOpts)
	if err != nil {
		return nil, nil, err
	}
	var layers []ExplainLayer
	if value, ok := lookupPathValue(baseMap, relPath); ok {
		layers = append(layers, ExplainLayer{
			Label: "import stack source " + sourceLabel(basePath),
			Value: value,
		})
	}
	var remoteOverrides map[string]any
	if ref.OverridesPath != "" {
		remoteOverrides, _, err = loadSourceMap(SourceRef{
			URL:  ref.URL,
			Ref:  ref.Ref,
			Path: ref.OverridesPath,
		}, baseDir, loadOpts)
		if err != nil {
			return nil, nil, err
		}
		if value, ok := lookupPathValue(remoteOverrides, relPath); ok {
			layers = append(layers, ExplainLayer{
				Label: "import stack overrides " + ref.OverridesPath,
				Value: value,
			})
		}
	}
	localOverrides, _ := lookupPathMap(stackMap, []string{"overrides"})
	if value, ok := lookupPathValue(localOverrides, relPath); ok {
		layers = append(layers, ExplainLayer{
			Label: "stack import overrides",
			Value: value,
		})
	}
	merged, err := mergeMaps(baseMap, remoteOverrides, localOverrides)
	if err != nil {
		return nil, nil, err
	}
	merged["__base_dir"] = stackResolvedBaseDirFromPath(basePath)
	return merged, layers, nil
}

func explainImportedService(serviceMap map[string]any, baseDir string, relPath []string, loadOpts LoadOptions) ([]ExplainLayer, error) {
	sourceMap, ok := lookupPathMap(serviceMap, []string{"source"})
	if !ok {
		return nil, nil
	}
	ref, err := decodeSourceRefMap(sourceMap)
	if err != nil {
		return nil, err
	}
	baseMap, basePath, err := loadSourceMap(ref, baseDir, loadOpts)
	if err != nil {
		return nil, err
	}
	var layers []ExplainLayer
	if value, ok := lookupPathValue(baseMap, relPath); ok {
		layers = append(layers, ExplainLayer{
			Label: "import service source " + sourceLabel(basePath),
			Value: value,
		})
	}
	if ref.OverridesPath != "" {
		overrideMap, _, err := loadSourceMap(SourceRef{
			URL:  ref.URL,
			Ref:  ref.Ref,
			Path: ref.OverridesPath,
		}, baseDir, loadOpts)
		if err != nil {
			return nil, err
		}
		if value, ok := lookupPathValue(overrideMap, relPath); ok {
			layers = append(layers, ExplainLayer{
				Label: "import service overrides " + ref.OverridesPath,
				Value: value,
			})
		}
	}
	localOverrides, _ := lookupPathMap(serviceMap, []string{"overrides"})
	if value, ok := lookupPathValue(localOverrides, relPath); ok {
		layers = append(layers, ExplainLayer{
			Label: "service import overrides",
			Value: value,
		})
	}
	_ = basePath
	return layers, nil
}

func explainOverlayLayers(cfg *Config, fieldPath []string, partition string) []ExplainLayer {
	if len(fieldPath) == 0 {
		return nil
	}
	switch fieldPath[0] {
	case "project":
		return explainProjectOverlayLayers(cfg, fieldPath[1:], partition)
	case "stacks":
		if len(fieldPath) < 2 {
			return nil
		}
		stackName := fieldPath[1]
		if len(fieldPath) >= 4 && fieldPath[2] == "services" {
			return explainServiceOverlayLayers(cfg, stackName, fieldPath[3], fieldPath[4:], partition)
		}
		return explainStackOverlayLayers(cfg, stackName, fieldPath[2:], partition)
	default:
		return nil
	}
}

func explainProjectOverlayLayers(cfg *Config, relPath []string, partition string) []ExplainLayer {
	var layers []ExplainLayer
	if overlay := cfg.deploymentOverlay(); overlay != nil {
		if value, ok := lookupStructPathValue(overlay.Project, relPath); ok {
			layers = append(layers, ExplainLayer{Label: "project deployment overlay", Value: value})
		}
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		if value, ok := lookupStructPathValue(overlay.Project, relPath); ok {
			layers = append(layers, ExplainLayer{Label: "project partition overlay", Value: value})
		}
	}
	return layers
}

func explainStackOverlayLayers(cfg *Config, stackName string, relPath []string, partition string) []ExplainLayer {
	var layers []ExplainLayer
	if overlay := cfg.deploymentOverlay(); overlay != nil {
		if stack := overlayStack(overlay, stackName); stack != nil {
			if value, ok := lookupStackOverlayValue(*stack, relPath); ok {
				layers = append(layers, ExplainLayer{Label: "project deployment overlay", Value: value})
			}
		}
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		if stack := overlayStack(&overlay, stackName); stack != nil {
			if value, ok := lookupStackOverlayValue(*stack, relPath); ok {
				layers = append(layers, ExplainLayer{Label: "project partition overlay", Value: value})
			}
		}
	}
	if overlay := cfg.stackDeploymentOverlay(stackName); overlay != nil {
		if value, ok := lookupStackOverlayValue(*overlay, relPath); ok {
			layers = append(layers, ExplainLayer{Label: "stack deployment overlay", Value: value})
		}
	}
	for _, overlay := range cfg.stackPartitionOverlays(stackName, partition) {
		if value, ok := lookupStackOverlayValue(overlay, relPath); ok {
			layers = append(layers, ExplainLayer{Label: "stack partition overlay", Value: value})
		}
	}
	return layers
}

func explainServiceOverlayLayers(cfg *Config, stackName string, serviceName string, relPath []string, partition string) []ExplainLayer {
	var layers []ExplainLayer
	if overlay := cfg.deploymentOverlay(); overlay != nil {
		if value, ok := lookupOverlayServiceValue(overlayStackServices(overlay, stackName), serviceName, relPath); ok {
			layers = append(layers, ExplainLayer{Label: "project deployment overlay", Value: value})
		}
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		if value, ok := lookupOverlayServiceValue(overlayStackServices(&overlay, stackName), serviceName, relPath); ok {
			layers = append(layers, ExplainLayer{Label: "project partition overlay", Value: value})
		}
	}
	if overlay := cfg.stackDeploymentOverlay(stackName); overlay != nil {
		if value, ok := lookupOverlayServiceValue(overlay.Services, serviceName, relPath); ok {
			layers = append(layers, ExplainLayer{Label: "stack deployment overlay", Value: value})
		}
	}
	for _, overlay := range cfg.stackPartitionOverlays(stackName, partition) {
		if value, ok := lookupOverlayServiceValue(overlay.Services, serviceName, relPath); ok {
			layers = append(layers, ExplainLayer{Label: "stack partition overlay", Value: value})
		}
	}
	if value, ok := lookupOverlayServiceValue(cfg.serviceDeploymentOverlays(stackName, false), serviceName, relPath); ok {
		layers = append(layers, ExplainLayer{Label: "service deployment overlay", Value: value})
	}
	for _, overlayMap := range cfg.servicePartitionOverlayMaps(stackName, partition, false) {
		if value, ok := lookupOverlayServiceValue(overlayMap, serviceName, relPath); ok {
			layers = append(layers, ExplainLayer{Label: "service partition overlay", Value: value})
		}
	}
	for _, overlayMap := range cfg.servicePartitionOverlayMaps(stackName, partition, true) {
		if value, ok := lookupOverlayServiceValue(overlayMap, serviceName, relPath); ok {
			layers = append(layers, ExplainLayer{Label: "service partition overlay", Value: value})
		}
	}
	if value, ok := lookupOverlayServiceValue(cfg.serviceDeploymentOverlays(stackName, true), serviceName, relPath); ok {
		layers = append(layers, ExplainLayer{Label: "service deployment overlay", Value: value})
	}
	return layers
}

func lookupOverlayServiceValue(overlays map[string]OverlayService, serviceName string, relPath []string) (any, bool) {
	if len(overlays) == 0 {
		return nil, false
	}
	overlay, ok := overlays[serviceName]
	if !ok {
		return nil, false
	}
	return lookupPathValue(overlay.Fields, relPath)
}

func lookupStackOverlayValue(overlay OverlayStack, relPath []string) (any, bool) {
	mapped, err := structToMap(overlay)
	if err != nil {
		return nil, false
	}
	delete(mapped, "services")
	return lookupPathValue(mapped, relPath)
}

func lookupStructPathValue(value any, relPath []string) (any, bool) {
	mapped, err := structToMap(value)
	if err != nil {
		return nil, false
	}
	return lookupPathValue(mapped, relPath)
}

func decodeSourceRefMap(mapped map[string]any) (SourceRef, error) {
	ref := SourceRef{}
	if url, ok := mapped["url"].(string); ok {
		ref.URL = url
	}
	if refVal, ok := mapped["ref"].(string); ok {
		ref.Ref = refVal
	}
	if pathVal, ok := mapped["path"].(string); ok {
		ref.Path = pathVal
	}
	if overridesPath, ok := mapped["overrides_path"].(string); ok {
		ref.OverridesPath = overridesPath
	}
	if ref.Path == "" {
		return SourceRef{}, fmt.Errorf("source.path is required")
	}
	return ref, nil
}

func filterExplainLayers(layers []ExplainLayer, finalValue any) []ExplainLayer {
	if len(layers) == 0 {
		return nil
	}
	out := make([]ExplainLayer, 0, len(layers))
	for _, layer := range layers {
		if layer.Label == "" {
			continue
		}
		out = append(out, layer)
	}
	if len(out) == 0 {
		return nil
	}
	lastMatch := -1
	for i, layer := range out {
		if fmt.Sprint(layer.Value) == fmt.Sprint(finalValue) {
			lastMatch = i
		}
	}
	if lastMatch >= 0 {
		return out[:lastMatch+1]
	}
	return out
}

func sourceLabel(path string) string {
	return strings.TrimSpace(path)
}

func stackResolvedBaseDir(mapped map[string]any, fallback string) string {
	if baseDir, ok := mapped["__base_dir"].(string); ok && baseDir != "" {
		return baseDir
	}
	return fallback
}

func stackResolvedBaseDirFromPath(basePath string) string {
	baseDir, err := sourceBaseDirFromPath(basePath)
	if err != nil {
		return ""
	}
	return baseDir
}

func partitionExists(cfg *Config, partition string) bool {
	for _, name := range cfg.Project.Partitions {
		if name == partition {
			return true
		}
	}
	return false
}

func explainPathSuggestions(root map[string]any, path []string) []string {
	if len(path) == 0 || root == nil {
		return nil
	}
	var current any = root
	for i, segment := range path {
		mapped, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		next, ok := mapped[segment]
		if ok {
			current = next
			continue
		}
		keys := mapKeys(mapped)
		if len(keys) == 0 {
			return nil
		}
		if i == len(path)-1 {
			return rankedSuggestions(segment, keys)
		}
		return keys
	}
	if mapped, ok := current.(map[string]any); ok {
		return mapKeys(mapped)
	}
	return nil
}

func mapKeys(mapped map[string]any) []string {
	if len(mapped) == 0 {
		return nil
	}
	keys := make([]string, 0, len(mapped))
	for key := range mapped {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func rankedSuggestions(target string, keys []string) []string {
	if len(keys) == 0 {
		return nil
	}
	target = strings.ToLower(target)
	var prefix []string
	var contains []string
	for _, key := range keys {
		lower := strings.ToLower(key)
		switch {
		case strings.HasPrefix(lower, target):
			prefix = append(prefix, key)
		case strings.Contains(lower, target):
			contains = append(contains, key)
		}
	}
	if len(prefix) > 0 {
		return prefix
	}
	if len(contains) > 0 {
		return contains
	}
	if len(keys) > 8 {
		return keys[:8]
	}
	return keys
}
