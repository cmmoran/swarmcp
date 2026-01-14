package templates

import (
	"net/url"
	pathpkg "path"
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

func IsTemplateSource(p string) bool {
	base, _ := SplitSource(p)
	if strings.HasPrefix(base, "git:") {
		parts := strings.SplitN(strings.TrimPrefix(base, "git:"), "|", 3)
		if len(parts) == 3 {
			if decoded, err := url.QueryUnescape(parts[2]); err == nil {
				ext := pathpkg.Ext(decoded)
				return ext == ".tmpl" || ext == ".tpl"
			}
		}
	}
	return filepath.Ext(base) == ".tmpl" || filepath.Ext(base) == ".tpl"
}
