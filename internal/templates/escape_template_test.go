package templates

import "testing"

func TestEscapeTemplate(t *testing.T) {
	out := EscapeTemplate(`default (uuidv4) (.Header.Get "X-Correlation-ID")`)
	want := `{{` + "`" + `{{ default (uuidv4) (.Header.Get \"X-Correlation-ID\") }}` + "`" + `}}`
	if out != want {
		t.Fatalf("unexpected output: %q", out)
	}
}
