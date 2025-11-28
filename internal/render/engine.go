package render

import (
	"bytes"
	"os"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	. "github.com/samber/lo"
)

type Option func(*options)

type options struct {
	configFuncs template.FuncMap
	secretFuncs template.FuncMap
	engine      *Engine
}

func WithConfigFuncs(fn template.FuncMap) Option {
	return func(o *options) { o.configFuncs = fn }
}

func WithSecretFuncs(fn template.FuncMap) Option {
	return func(o *options) { o.secretFuncs = fn }
}

func WithParent(e *Engine) Option {
	return func(o *options) {
		o.engine = e
	}
}

type Engine struct {
	parent                 *Engine
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
		parent: opts.engine,
		configRoot: If(
			opts.configFuncs != nil,
			template.New("").Funcs(txtfuncMap).Funcs(opts.configFuncs),
		).
			Else(template.New("").Funcs(txtfuncMap)),
		secretRoot: If(
			opts.secretFuncs != nil,
			template.New("").Funcs(txtfuncMap).Funcs(opts.secretFuncs),
		).
			Else(template.New("").Funcs(txtfuncMap)),
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
