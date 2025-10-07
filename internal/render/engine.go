package render

import (
	"bytes"
	"os"
	"text/template"
)

type Options struct{}

type Engine struct {
	funcs template.FuncMap
}

func NewEngine(_ Options) *Engine {
	fm := template.FuncMap{
		"default": func(def any, v any) any {
			if v == nil {
				return def
			}
			switch x := v.(type) {
			case string:
				if x == "" {
					return def
				}
			case []byte:
				if len(x) == 0 {
					return def
				}
			}
			return v
		},
	}
	return &Engine{funcs: fm}
}

func (e *Engine) RenderString(name, tpl string, data map[string]any) (string, error) {
	t, err := template.New(name).Funcs(e.funcs).Parse(tpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (e *Engine) RenderFile(path string, data map[string]any) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s, err := e.RenderString(path, string(b), data)
	if err != nil {
		return nil, err
	}
	return []byte(s), nil
}
