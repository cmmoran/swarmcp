package config

import "fmt"

func DefaultSecretSource(name, source string) string {
	if source != "" {
		return source
	}
	return fmt.Sprintf("inline:\n  {{ secret_value %q }}", name)
}

func applySecretDefDefaults(defs map[string]SecretDef) map[string]SecretDef {
	if len(defs) == 0 {
		return defs
	}
	out := make(map[string]SecretDef, len(defs))
	for name, def := range defs {
		def.Source = DefaultSecretSource(name, def.Source)
		out[name] = def
	}
	return out
}
