package templates

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cmmoran/swarmcp/internal/config"
)

type stubResolver struct{}

func (s stubResolver) ConfigValue(name string) (any, error)  { return "", nil }
func (s stubResolver) ConfigRef(name string) (string, error) { return "", nil }
func (s stubResolver) ConfigRefs(pattern string) ([]string, error) {
	return nil, nil
}
func (s stubResolver) SecretValue(name string) (string, error) { return "", nil }
func (s stubResolver) SecretRef(name string) (string, error)   { return "", nil }
func (s stubResolver) SecretRefs(pattern string) ([]string, error) {
	return nil, nil
}
func (s stubResolver) RuntimeValue(args ...string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	return args[0], nil
}

func TestResolveSourceInline(t *testing.T) {
	engine := New(stubResolver{})
	data := map[string]string{"Name": "world"}
	out, err := ResolveSource("inline:hello {{ .Name }}", Scope{Project: "demo"}, data, engine, nil, "", config.LoadOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "hello world" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestResolveSourceInlineTrimBlock(t *testing.T) {
	engine := New(stubResolver{})
	data := map[string]string{"Name": "block"}
	source := "inline:\n  hello {{ .Name }}\n"
	out, err := ResolveSource(source, Scope{Project: "demo"}, data, engine, nil, "", config.LoadOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "hello block" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestResolveSourceUsesBaseDir(t *testing.T) {
	engine := New(stubResolver{})
	data := map[string]string{"Name": "base"}
	baseDir := t.TempDir()
	path := filepath.Join(baseDir, "configs", "app.tmpl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("hello {{ .Name }}"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	out, err := ResolveSource("configs/app.tmpl", Scope{Project: "demo"}, data, engine, nil, baseDir, config.LoadOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "hello base" {
		t.Fatalf("unexpected output: %q", out)
	}
}
