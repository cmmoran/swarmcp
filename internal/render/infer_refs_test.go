package render

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/templates"
)

func TestInferTemplateRefDepsConfigRef(t *testing.T) {
	dir := t.TempDir()
	aPath := filepath.Join(dir, "a.tmpl")
	bPath := filepath.Join(dir, "b.tmpl")

	if err := os.WriteFile(aPath, []byte(`{{ config_ref "b" }}`), 0o600); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(bPath, []byte(`hello`), 0o600); err != nil {
		t.Fatalf("write b: %v", err)
	}

	cfg := &config.Config{
		BaseDir: dir,
		Project: config.Project{
			Name: "primary",
			Configs: map[string]config.ConfigDef{
				"a": {Source: "a.tmpl"},
				"b": {Source: "b.tmpl"},
			},
		},
	}

	scope := templates.Scope{Project: "primary", Stack: "core", Service: "api"}
	extraConfigs, extraSecrets, err := InferTemplateRefDeps(cfg, scope, []config.ConfigRef{{Name: "a"}}, nil)
	if err != nil {
		t.Fatalf("InferTemplateRefDeps: %v", err)
	}
	if len(extraSecrets) != 0 {
		t.Fatalf("expected no secret deps, got %#v", extraSecrets)
	}
	if _, ok := extraConfigs["b"]; !ok {
		t.Fatalf("expected config dep on b, got %#v", extraConfigs)
	}
}
