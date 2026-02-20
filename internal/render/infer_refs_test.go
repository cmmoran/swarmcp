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

func TestInferTemplateRefDepsConfigRefsGlob(t *testing.T) {
	dir := t.TempDir()
	aPath := filepath.Join(dir, "a.tmpl")
	bPath := filepath.Join(dir, "b.tmpl")
	cPath := filepath.Join(dir, "c.tmpl")

	if err := os.WriteFile(aPath, []byte(`{{ range (config_refs "db_*") }}{{ . }}{{ end }}`), 0o600); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(bPath, []byte(`hello`), 0o600); err != nil {
		t.Fatalf("write b: %v", err)
	}
	if err := os.WriteFile(cPath, []byte(`world`), 0o600); err != nil {
		t.Fatalf("write c: %v", err)
	}

	cfg := &config.Config{
		BaseDir: dir,
		Project: config.Project{
			Name: "primary",
			Configs: map[string]config.ConfigDef{
				"a":       {Source: "a.tmpl"},
				"db_main": {Source: "b.tmpl"},
				"db_rep":  {Source: "c.tmpl"},
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
	if _, ok := extraConfigs["db_main"]; !ok {
		t.Fatalf("expected db_main dep, got %#v", extraConfigs)
	}
	if _, ok := extraConfigs["db_rep"]; !ok {
		t.Fatalf("expected db_rep dep, got %#v", extraConfigs)
	}
}
