package templates

import (
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func ResolveSource(source string, scope Scope, data any, engine *Engine, values any, baseDir string) (string, error) {
	if source == "" {
		return "", nil
	}
	if strings.HasPrefix(source, "inline:") {
		content := strings.TrimPrefix(source, "inline:")
		content = strings.TrimSpace(content)
		return engine.Render("inline", content, data)
	}
	if fragment, ok := ValuesFragment(source); ok {
		if values == nil {
			return "", fmt.Errorf("values store not configured")
		}
		fragment = ExpandTokens(fragment, scope)
		return ResolveValuesFragment(values, fragment, scope)
	}
	templatePath := ExpandSourcePathTokens(source, scope)
	if baseDir != "" {
		basePath, fragment := SplitSource(templatePath)
		if !filepath.IsAbs(basePath) {
			basePath = filepath.Join(baseDir, basePath)
		}
		templatePath = basePath + fragment
	}
	return engine.RenderFile(templatePath, data)
}

func ResolveFragment(root any, fragment string) (string, error) {
	if fragment == "" || fragment == "#" {
		if root == nil {
			return "", nil
		}
		encoded, err := yaml.Marshal(root)
		if err != nil {
			return "", err
		}
		return strings.TrimRight(string(encoded), "\n"), nil
	}
	if !strings.HasPrefix(fragment, "#/") {
		return "", fmt.Errorf("unsupported fragment %q", fragment)
	}
	cursor := root
	parts := strings.Split(fragment[2:], "/")
	for _, part := range parts {
		if part == "" {
			continue
		}
		key := decodePointerToken(part)
		next, ok := lookupFragment(cursor, key)
		if !ok {
			return "", fmt.Errorf("fragment %q not found", fragment)
		}
		cursor = next
	}
	return formatFragmentValue(cursor)
}

func ResolveValuesFragment(values any, fragment string, scope Scope) (string, error) {
	if fragment == "" || fragment == "#" || fragment == "#/" {
		return ResolveFragment(values, fragment)
	}
	if isExplicitValuesRoot(fragment) {
		return ResolveFragment(values, fragment)
	}
	if !strings.HasPrefix(fragment, "#/") {
		return "", fmt.Errorf("unsupported fragment %q", fragment)
	}
	path := strings.TrimPrefix(fragment, "#/")
	candidates := make([]string, 0, 6)
	if scope.Stack != "" && scope.Partition != "" {
		candidates = append(candidates, fmt.Sprintf("#/stacks/%s/partitions/%s/%s", scope.Stack, scope.Partition, path))
	}
	if scope.Stack != "" {
		candidates = append(candidates, fmt.Sprintf("#/stacks/%s/%s", scope.Stack, path))
	}
	if scope.Partition != "" {
		candidates = append(candidates, fmt.Sprintf("#/partitions/%s/%s", scope.Partition, path))
	}
	if scope.Deployment != "" {
		candidates = append(candidates, fmt.Sprintf("#/deployments/%s/%s", scope.Deployment, path))
	}
	candidates = append(candidates, fmt.Sprintf("#/global/%s", path), "#/"+path)
	for _, candidate := range candidates {
		value, ok, err := tryResolveFragment(values, candidate)
		if err != nil {
			return "", err
		}
		if ok {
			return value, nil
		}
	}
	return "", fmt.Errorf("fragment %q not found", fragment)
}

func ResolveValuesFragmentValue(values any, fragment string, scope Scope) (any, error) {
	if fragment == "" || fragment == "#" || fragment == "#/" {
		return values, nil
	}
	if isExplicitValuesRoot(fragment) {
		value, _, err := tryResolveFragmentValue(values, fragment)
		return value, err
	}
	if !strings.HasPrefix(fragment, "#/") {
		return nil, fmt.Errorf("unsupported fragment %q", fragment)
	}
	path := strings.TrimPrefix(fragment, "#/")
	candidates := make([]string, 0, 6)
	if scope.Stack != "" && scope.Partition != "" {
		candidates = append(candidates, fmt.Sprintf("#/stacks/%s/partitions/%s/%s", scope.Stack, scope.Partition, path))
	}
	if scope.Stack != "" {
		candidates = append(candidates, fmt.Sprintf("#/stacks/%s/%s", scope.Stack, path))
	}
	if scope.Partition != "" {
		candidates = append(candidates, fmt.Sprintf("#/partitions/%s/%s", scope.Partition, path))
	}
	if scope.Deployment != "" {
		candidates = append(candidates, fmt.Sprintf("#/deployments/%s/%s", scope.Deployment, path))
	}
	candidates = append(candidates, fmt.Sprintf("#/global/%s", path), "#/"+path)
	for _, candidate := range candidates {
		value, ok, err := tryResolveFragmentValue(values, candidate)
		if err != nil {
			return nil, err
		}
		if ok {
			return value, nil
		}
	}
	return nil, fmt.Errorf("fragment %q not found", fragment)
}

func tryResolveFragment(root any, fragment string) (string, bool, error) {
	if root == nil {
		return "", false, nil
	}
	if fragment == "" || fragment == "#" || fragment == "#/" {
		value, err := ResolveFragment(root, fragment)
		return value, true, err
	}
	if !strings.HasPrefix(fragment, "#/") {
		return "", false, fmt.Errorf("unsupported fragment %q", fragment)
	}
	cursor := root
	parts := strings.Split(fragment[2:], "/")
	for _, part := range parts {
		if part == "" {
			continue
		}
		key := decodePointerToken(part)
		next, ok := lookupFragment(cursor, key)
		if !ok {
			return "", false, nil
		}
		cursor = next
	}
	value, err := formatFragmentValue(cursor)
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func tryResolveFragmentValue(root any, fragment string) (any, bool, error) {
	if root == nil {
		return nil, false, nil
	}
	if fragment == "" || fragment == "#" || fragment == "#/" {
		return root, true, nil
	}
	if !strings.HasPrefix(fragment, "#/") {
		return nil, false, fmt.Errorf("unsupported fragment %q", fragment)
	}
	cursor := root
	parts := strings.Split(fragment[2:], "/")
	for _, part := range parts {
		if part == "" {
			continue
		}
		key := decodePointerToken(part)
		next, ok := lookupFragment(cursor, key)
		if !ok {
			return nil, false, nil
		}
		cursor = next
	}
	return cursor, true, nil
}

func isExplicitValuesRoot(fragment string) bool {
	return strings.HasPrefix(fragment, "#/global") ||
		strings.HasPrefix(fragment, "#/deployments") ||
		strings.HasPrefix(fragment, "#/partitions") ||
		strings.HasPrefix(fragment, "#/stacks")
}
