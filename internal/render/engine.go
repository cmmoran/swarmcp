package render

import (
	"bytes"
	"os"
	"text/template"

	"github.com/Masterminds/sprig/v3"
)

type Option func(*options)

type options struct {
	configFuncs template.FuncMap
	secretFuncs template.FuncMap
}

func WithConfigFuncs(fn template.FuncMap) Option {
	return func(o *options) { o.configFuncs = fn }
}

func WithSecretFuncs(fn template.FuncMap) Option {
	return func(o *options) { o.secretFuncs = fn }
}

type Engine struct {
	configRoot, secretRoot *template.Template
	funcs                  template.FuncMap
}

func NewEngine(o ...Option) *Engine {
	opts := &options{}
	for _, opt := range o {
		opt(opts)
	}
	txtfuncMap := sprig.TxtFuncMap()
	e := &Engine{
		configRoot: template.New("").Funcs(txtfuncMap),
		secretRoot: template.New("").Funcs(txtfuncMap),
	}
	if opts.configFuncs != nil {
		e.configRoot.Funcs(opts.configFuncs)
	}
	if opts.secretFuncs != nil {
		e.secretRoot.Funcs(opts.secretFuncs)
	}
	return e
}

func (e *Engine) RenderTemplateString(name, tpl string, data map[string]any, secretMarker ...any) (string, error) {
	var err error
	lt := e.configRoot
	if len(secretMarker) > 0 {
		lt = e.secretRoot
	}
	t := lt.Lookup(name)
	if t == nil {
		if t, err = lt.New(name).Parse(tpl); err != nil {
			return "", err
		}
	}
	var buf bytes.Buffer
	if err = t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (e *Engine) RenderFile(path string, data map[string]any, secretMarker ...any) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s, err := e.RenderTemplateString(path, string(b), data, secretMarker...)
	if err != nil {
		return nil, err
	}
	return []byte(s), nil
}
