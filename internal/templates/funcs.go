package templates

import (
	"strings"
	"text/template"
)

func JoinCompact(sep string, values ...string) string {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		filtered = append(filtered, trimmed)
	}
	return strings.Join(filtered, sep)
}

func withCommonTemplateFuncs(funcs template.FuncMap) template.FuncMap {
	if funcs == nil {
		funcs = template.FuncMap{}
	}
	funcs["joincompact"] = func(sep string, values ...string) string {
		return JoinCompact(sep, values...)
	}
	return funcs
}
