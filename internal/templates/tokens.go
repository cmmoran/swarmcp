package templates

import (
	"path/filepath"
	"strings"
)

// ExpandTokens replaces scope tokens in a string without additional normalization.
func ExpandTokens(input string, scope Scope) string {
	if input == "" {
		return input
	}

	replacer := strings.NewReplacer(
		"{project}", scope.Project,
		"{deployment}", scope.Deployment,
		"{stack}", scope.Stack,
		"{partition}", scope.Partition,
		"{service}", scope.Service,
		"{networks_shared}", scope.NetworksShared,
		"{network_ephemeral}", scope.NetworkEphemeral,
	)
	return replacer.Replace(input)
}

// ExpandPathTokens replaces scope tokens in a path-like string and normalizes
// empty segments (e.g., omitted partition for shared stacks).
func ExpandPathTokens(input string, scope Scope) string {
	if input == "" {
		return input
	}
	return normalizePath(ExpandTokens(input, scope))
}

func ExpandSourcePathTokens(input string, scope Scope) string {
	if input == "" {
		return input
	}
	base, fragment := SplitSource(input)
	if base == "" {
		return input
	}
	return ExpandPathTokens(base, scope) + fragment
}

func normalizePath(path string) string {
	if path == "" {
		return path
	}

	sep := string(filepath.Separator)
	absolute := strings.HasPrefix(path, sep)
	parts := strings.Split(path, sep)
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		clean = append(clean, part)
	}
	out := strings.Join(clean, sep)
	if absolute {
		out = sep + out
	}
	return out
}
