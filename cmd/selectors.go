package cmd

import (
	"fmt"
	"strings"
)

func normalizeSelectors(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func singleSelector(name string, values []string) (string, error) {
	normalized := normalizeSelectors(values)
	if len(normalized) == 0 {
		return "", nil
	}
	if len(normalized) > 1 {
		return "", fmt.Errorf("multiple --%s values are not supported for this command", name)
	}
	return normalized[0], nil
}

func deploymentTargets(values []string) []string {
	normalized := normalizeSelectors(values)
	if len(normalized) == 0 {
		return []string{""}
	}
	return normalized
}
