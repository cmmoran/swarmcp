package templates

import (
	"bytes"
	"testing"
	"text/template"
)

type testHeader struct {
	values map[string]string
}

func (h testHeader) Get(key string) string {
	return h.values[key]
}

type testContext struct {
	Header testHeader
	Name   string
}

func renderTemplate(t *testing.T, text string, funcs template.FuncMap, data any) string {
	t.Helper()

	tpl, err := template.New("t").Funcs(funcs).Parse(text)
	if err != nil {
		t.Fatalf("parse template: %v", err)
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		t.Fatalf("execute template: %v", err)
	}

	return buf.String()
}

func renderTemplateWithDelims(t *testing.T, text string, funcs template.FuncMap, data any, open, close string) string {
	t.Helper()

	tpl, err := template.New("t").Delims(open, close).Funcs(funcs).Parse(text)
	if err != nil {
		t.Fatalf("parse template: %v", err)
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		t.Fatalf("execute template: %v", err)
	}

	return buf.String()
}

func TestEscapeTemplate(t *testing.T) {
	funcs := template.FuncMap{
		"escape_template": func(args ...string) string {
			return EscapeTemplate(args...)
		},
	}

	firstPass := renderTemplate(
		t,
		`before {{ escape_template `+"`default (uuidv4) (.Header.Get \"X-Correlation-ID\")`"+` }} after`,
		funcs,
		nil,
	)

	wantFirstPass := `before {{ default (uuidv4) (.Header.Get "X-Correlation-ID") }} after`
	if firstPass != wantFirstPass {
		t.Fatalf("unexpected escaped output: %q", firstPass)
	}

	downstream := template.FuncMap{
		"default": func(def, val string) string {
			if val == "" {
				return def
			}
			return val
		},
		"uuidv4": func() string {
			return "uuid-1"
		},
	}

	withHeader := testContext{
		Header: testHeader{values: map[string]string{"X-Correlation-ID": "trace-123"}},
	}
	got := renderTemplate(t, firstPass, downstream, withHeader)
	want := "before trace-123 after"
	if got != want {
		t.Fatalf("unexpected rendered output with header: %q", got)
	}

	withoutHeader := testContext{
		Header: testHeader{values: map[string]string{}},
	}
	got = renderTemplate(t, firstPass, downstream, withoutHeader)
	want = "before uuid-1 after"
	if got != want {
		t.Fatalf("unexpected rendered output without header: %q", got)
	}
}

func TestEscapeTemplate_MultipleLevels(t *testing.T) {
	firstPass := renderTemplate(
		t,
		`before {{ escape_template `+"`default (uuidv4) (.Header.Get \"X-Correlation-ID\")`"+` "2" }} after`,
		template.FuncMap{
			"escape_template": func(args ...string) string {
				return EscapeTemplate(args...)
			},
		},
		nil,
	)

	wantFirstPass := `before {{ "{{ default (uuidv4) (.Header.Get \"X-Correlation-ID\") }}" }} after`
	if firstPass != wantFirstPass {
		t.Fatalf("unexpected escaped output: %q", firstPass)
	}

	secondPass := renderTemplate(t, firstPass, template.FuncMap{}, nil)
	wantSecondPass := `before {{ default (uuidv4) (.Header.Get "X-Correlation-ID") }} after`
	if secondPass != wantSecondPass {
		t.Fatalf("unexpected second-pass output: %q", secondPass)
	}

	withHeader := testContext{
		Header: testHeader{values: map[string]string{"X-Correlation-ID": "trace-999"}},
	}
	final := renderTemplate(t, secondPass, template.FuncMap{
		"default": func(def, val string) string {
			if val == "" {
				return def
			}
			return val
		},
		"uuidv4": func() string {
			return "uuid-9"
		},
	}, withHeader)
	if final != "before trace-999 after" {
		t.Fatalf("unexpected final output: %q", final)
	}
}

func TestEscapeTemplate_MultipleExpressions(t *testing.T) {
	firstPass := renderTemplate(
		t,
		`start {{ escape_template `+"`before {{ .Name }} then {{ .Age }} end`"+` }} finish`,
		template.FuncMap{
			"escape_template": func(args ...string) string {
				return EscapeTemplate(args...)
			},
		},
		nil,
	)

	wantFirstPass := `start before {{ .Name }} then {{ .Age }} end finish`
	if firstPass != wantFirstPass {
		t.Fatalf("unexpected escaped output: %q", firstPass)
	}

	got := renderTemplate(t, firstPass, template.FuncMap{}, struct {
		Name string
		Age  int
	}{Name: "Ada", Age: 38})

	if got != "start before Ada then 38 end finish" {
		t.Fatalf("unexpected rendered output: %q", got)
	}
}

func TestEscapeTemplate_CustomDelimiters(t *testing.T) {
	firstPass := renderTemplate(
		t,
		`{{ escape_template `+"`hello <% .Name %>`"+` "2" "<%" "%>" }}`,
		template.FuncMap{
			"escape_template": func(args ...string) string {
				return EscapeTemplate(args...)
			},
		},
		nil,
	)

	if firstPass != `hello <% "<% .Name %>" %>` {
		t.Fatalf("unexpected escaped output: %q", firstPass)
	}

	secondPass := renderTemplateWithDelims(t, firstPass, template.FuncMap{}, nil, "<%", "%>")
	if secondPass != `hello <% .Name %>` {
		t.Fatalf("unexpected rendered output with custom delimiters: %q", secondPass)
	}

	got := renderTemplateWithDelims(t, secondPass, template.FuncMap{}, testContext{Name: "Ada"}, "<%", "%>")
	if got != "hello Ada" {
		t.Fatalf("unexpected final rendered output with custom delimiters: %q", got)
	}
}
