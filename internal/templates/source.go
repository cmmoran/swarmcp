package templates

import (
	"path/filepath"
	"strings"
)

const valuesPrefix = "values#"

func SplitSource(source string) (string, string) {
	if source == "" {
		return source, ""
	}
	if idx := strings.Index(source, "#"); idx != -1 {
		return source[:idx], source[idx:]
	}
	return source, ""
}

func IsValuesSource(source string) bool {
	return strings.HasPrefix(source, valuesPrefix)
}

func ValuesFragment(source string) (string, bool) {
	if !IsValuesSource(source) {
		return "", false
	}
	fragment := strings.TrimPrefix(source, "values")
	if fragment == "" {
		fragment = "#"
	}
	return fragment, true
}

func IsTemplateSource(path string) bool {
	base, _ := SplitSource(path)
	return filepath.Ext(base) == ".tmpl" || filepath.Ext(base) == ".tpl"
}
