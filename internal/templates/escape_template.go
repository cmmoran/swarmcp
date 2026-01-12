package templates

import (
	"fmt"
	"strings"
)

// EscapeTemplate wraps inputs so they are emitted as literal Go template actions.
func EscapeTemplate(inputs ...string) string {
	return escapeTemplateParts(true, inputs...)
}

// EscapeSwarmTemplate wraps inputs so they are emitted as Go template actions for Swarm.
func EscapeSwarmTemplate(inputs ...string) string {
	return escapeTemplateParts(false, inputs...)
}

func escapeTemplateParts(backtick bool, inputs ...string) string {
	if len(inputs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(inputs))
	for _, input := range inputs {
		// Escape backslashes for YAML
		escaped := strings.ReplaceAll(input, `\`, `\\`)
		// Escape double quotes so YAML interprets it correctly
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		if backtick {
			parts = append(parts, fmt.Sprintf("{{`{{ %s }}`}}", escaped))
		} else {
			parts = append(parts, fmt.Sprintf("{{ %s }}", escaped))
		}
	}
	return strings.Join(parts, "")
}
