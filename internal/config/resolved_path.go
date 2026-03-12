package config

import (
	"fmt"
	"strings"
)

func LookupResolvedPath(root map[string]any, fieldPath string) (any, error) {
	segments := splitFieldPath(fieldPath)
	if len(segments) == 0 {
		return nil, fmt.Errorf("field path is required")
	}
	value, ok, detail := lookupPathValueDetailed(root, segments)
	if !ok {
		if detail != "" {
			return nil, fmt.Errorf("invalid field path %q: %s", fieldPath, detail)
		}
		return nil, fmt.Errorf("field path %q not found", fieldPath)
	}
	return value, nil
}

func splitFieldPath(path string) []string {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(path, ".") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil
		}
		out = append(out, part)
	}
	return out
}

func lookupPathValue(root map[string]any, path []string) (any, bool) {
	value, ok, _ := lookupPathValueDetailed(root, path)
	return value, ok
}

func lookupPathValueDetailed(root map[string]any, path []string) (any, bool, string) {
	if root == nil {
		return nil, false, "root is empty"
	}
	var current any = root
	for i, segment := range path {
		mapped, ok := current.(map[string]any)
		if !ok {
			prefix := strings.Join(path[:i], ".")
			if prefix == "" {
				prefix = "(root)"
			}
			return nil, false, fmt.Sprintf("cannot traverse %q through non-map value at %s", segment, prefix)
		}
		current, ok = mapped[segment]
		if !ok {
			return nil, false, ""
		}
	}
	return current, true, ""
}

func lookupPathMap(root map[string]any, path []string) (map[string]any, bool) {
	value, ok := lookupPathValue(root, path)
	if !ok {
		return nil, false
	}
	mapped, ok := value.(map[string]any)
	return mapped, ok
}
