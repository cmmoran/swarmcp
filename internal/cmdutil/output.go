package cmdutil

import (
	"fmt"
	"io"
)

func PrintWarnings(out io.Writer, warnings []string) {
	if len(warnings) == 0 {
		return
	}
	_, _ = fmt.Fprintln(out, "warnings:")
	for _, warning := range warnings {
		_, _ = fmt.Fprintf(out, "  - %s\n", warning)
	}
}

func FormatConfigItem(name string, labels map[string]string) string {
	logical := labels["swarmcp.name"]
	hash := labels["swarmcp.hash"]
	if logical == "" {
		return name
	}
	if hash == "" {
		return fmt.Sprintf("%s (logical %s)", name, logical)
	}
	return fmt.Sprintf("%s (logical %s hash %s)", name, logical, hash)
}
