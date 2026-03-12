package config

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/cmmoran/swarmcp/internal/yamlutil"
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
	resolvedModel, err := LoadResolvedModelTrace(ResolvedModelOptions{
		ConfigPaths:        opts.ConfigPaths,
		ReleaseConfigPaths: opts.ReleaseConfigPaths,
		Deployment:         opts.Deployment,
		Partition:          opts.Partition,
		Stack:              opts.Stack,
		LoadOptions:        opts.LoadOptions,
	}, segments)
	if err != nil {
		return nil, err
	}
	finalValue, ok, detail := lookupPathValueDetailed(resolvedModel.Model, segments)
	if !ok {
		if detail != "" {
			return nil, fmt.Errorf("invalid field path %q: %s", fieldPath, detail)
		}
		suggestions := explainPathSuggestions(resolvedModel.Model, segments)
		if len(suggestions) == 0 {
			return nil, fmt.Errorf("field path %q not found", fieldPath)
		}
		return nil, fmt.Errorf("field path %q not found (available: %s)", fieldPath, strings.Join(suggestions, ", "))
	}

	layers := append([]ExplainLayer(nil), resolvedModel.Trace.Layers...)
	layers = append(layers, resolvedModel.Trace.ImportLayers...)
	layers = append(layers, resolvedModel.Trace.OverlayLayers...)
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
	finalKey := explainValueKey(finalValue)
	for i, layer := range out {
		if explainValueKey(layer.Value) == finalKey {
			lastMatch = i
		}
	}
	if lastMatch >= 0 {
		return out[:lastMatch+1]
	}
	return out
}

func explainValueKey(value any) string {
	normalized := yamlutil.NormalizeValue(value)
	data, err := json.Marshal(normalized)
	if err != nil {
		return fmt.Sprintf("%#v", normalized)
	}
	return string(data)
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
