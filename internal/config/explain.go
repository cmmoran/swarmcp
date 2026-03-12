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
	cfg, trace, err := LoadFilesWithReleaseTrace(opts.ConfigPaths, opts.ReleaseConfigPaths, opts.LoadOptions, segments)
	if err != nil {
		return nil, err
	}
	if opts.Deployment != "" {
		cfg.Project.Deployment = opts.Deployment
	}
	if err := ValidateDeployment(cfg); err != nil {
		return nil, err
	}
	if opts.Partition != "" && !partitionExists(cfg, opts.Partition) {
		return nil, fmt.Errorf("partition %q not found in project.partitions", opts.Partition)
	}
	if opts.Stack != "" {
		if _, ok := cfg.Stacks[opts.Stack]; !ok {
			return nil, fmt.Errorf("stack %q not found in stacks", opts.Stack)
		}
	}
	stackFilters := []string(nil)
	if opts.Stack != "" {
		stackFilters = []string{opts.Stack}
	}
	resolved, err := DebugResolvedMap(cfg, opts.Partition, stackFilters)
	if err != nil {
		return nil, err
	}
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

	layers := append([]ExplainLayer(nil), trace.Layers...)
	layers = append(layers, trace.ImportLayers...)
	layers = append(layers, ResolvedOverlayLayers(cfg, segments, opts.Partition)...)
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
