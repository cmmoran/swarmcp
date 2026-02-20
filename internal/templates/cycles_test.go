package templates

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cmmoran/swarmcp/internal/config"
)

func TestDetectCyclesConfigCycle(t *testing.T) {
	dir := t.TempDir()
	aPath := filepath.Join(dir, "a.tmpl")
	bPath := filepath.Join(dir, "b.tmpl")

	if err := os.WriteFile(aPath, []byte(`{{ config_value "b" }}`), 0o600); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(bPath, []byte(`{{ config_value "a" }}`), 0o600); err != nil {
		t.Fatalf("write b: %v", err)
	}

	cfg := &config.Config{
		Project: config.Project{
			Name: "primary",
			Configs: map[string]config.ConfigDef{
				"a": {Source: aPath},
				"b": {Source: bPath},
			},
		},
	}

	if _, err := DetectCycles(cfg, false); err == nil {
		t.Fatalf("expected cycle error")
	}
}

func TestDetectCyclesSecretRefMissing(t *testing.T) {
	dir := t.TempDir()
	sPath := filepath.Join(dir, "s.tmpl")

	if err := os.WriteFile(sPath, []byte(`{{ secret_ref "missing" }}`), 0o600); err != nil {
		t.Fatalf("write s: %v", err)
	}

	cfg := &config.Config{
		Project: config.Project{
			Name: "primary",
			Secrets: map[string]config.SecretDef{
				"s": {Source: sPath},
			},
		},
	}

	if _, err := DetectCycles(cfg, false); err == nil {
		t.Fatalf("expected missing secret_ref error")
	}
}

func TestDetectCyclesConfigSecretRefAllowed(t *testing.T) {
	dir := t.TempDir()
	cPath := filepath.Join(dir, "c.tmpl")

	if err := os.WriteFile(cPath, []byte(`{{ secret_ref "token" }}`), 0o600); err != nil {
		t.Fatalf("write c: %v", err)
	}

	cfg := &config.Config{
		Project: config.Project{
			Name: "primary",
			Configs: map[string]config.ConfigDef{
				"c": {Source: cPath},
			},
			Secrets: map[string]config.SecretDef{
				"token": {Source: "ignored"},
			},
		},
	}

	if _, err := DetectCycles(cfg, false); err != nil {
		t.Fatalf("DetectCycles: %v", err)
	}
}

func TestDetectCyclesDynamicRefWarning(t *testing.T) {
	dir := t.TempDir()
	cPath := filepath.Join(dir, "c.tmpl")

	if err := os.WriteFile(cPath, []byte(`{{ config_value .Name }}`), 0o600); err != nil {
		t.Fatalf("write c: %v", err)
	}

	cfg := &config.Config{
		Project: config.Project{
			Name: "primary",
			Configs: map[string]config.ConfigDef{
				"c": {Source: cPath},
			},
		},
	}

	warnings, err := DetectCycles(cfg, false)
	if err != nil {
		t.Fatalf("DetectCycles: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatalf("expected dynamic ref warning")
	}
}

func TestDetectCyclesSecretValueMissingAllowed(t *testing.T) {
	dir := t.TempDir()
	sPath := filepath.Join(dir, "s.tmpl")

	if err := os.WriteFile(sPath, []byte(`{{ secret_value "missing" }}`), 0o600); err != nil {
		t.Fatalf("write s: %v", err)
	}

	cfg := &config.Config{
		Project: config.Project{
			Name: "primary",
			Secrets: map[string]config.SecretDef{
				"s": {Source: sPath},
			},
		},
	}

	if _, err := DetectCycles(cfg, false); err != nil {
		t.Fatalf("DetectCycles: %v", err)
	}
}

func TestDetectCyclesConfigValueMissingAllowed(t *testing.T) {
	dir := t.TempDir()
	cPath := filepath.Join(dir, "c.tmpl")

	if err := os.WriteFile(cPath, []byte(`{{ config_value "missing" }}`), 0o600); err != nil {
		t.Fatalf("write c: %v", err)
	}

	cfg := &config.Config{
		Project: config.Project{
			Name: "primary",
			Configs: map[string]config.ConfigDef{
				"c": {Source: cPath},
			},
		},
	}

	if _, err := DetectCycles(cfg, false); err != nil {
		t.Fatalf("DetectCycles: %v", err)
	}
}

func TestDetectCyclesConfigValueIndexMissingAllowed(t *testing.T) {
	cfg := &config.Config{
		Project: config.Project{
			Name: "primary",
		},
		Stacks: map[string]config.Stack{
			"core": {
				Services: map[string]config.Service{
					"api": {
						Configs: []config.ConfigRef{
							{
								Name:   "config",
								Source: "values#/config",
							},
						},
					},
				},
			},
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml.tmpl")
	if err := os.WriteFile(path, []byte(`{{ config_value_index "missing" 0 }}`), 0o600); err != nil {
		t.Fatalf("write config template: %v", err)
	}

	cfg.Stacks["core"].Services["api"].Configs[0].Source = path
	config.SetBaseDir(cfg, dir)

	if _, err := DetectCycles(cfg, true); err != nil {
		t.Fatalf("DetectCycles: %v", err)
	}
}

func TestDetectCyclesConfigValueGetMissingAllowed(t *testing.T) {
	cfg := &config.Config{
		Project: config.Project{
			Name: "primary",
		},
		Stacks: map[string]config.Stack{
			"core": {
				Services: map[string]config.Service{
					"api": {
						Configs: []config.ConfigRef{
							{
								Name:   "config",
								Source: "values#/config",
							},
						},
					},
				},
			},
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml.tmpl")
	if err := os.WriteFile(path, []byte(`{{ config_value_get "missing" "key" }}`), 0o600); err != nil {
		t.Fatalf("write config template: %v", err)
	}

	cfg.Stacks["core"].Services["api"].Configs[0].Source = path
	config.SetBaseDir(cfg, dir)

	if _, err := DetectCycles(cfg, true); err != nil {
		t.Fatalf("DetectCycles: %v", err)
	}
}

func TestDetectCyclesConfigRefsGlobAllowed(t *testing.T) {
	dir := t.TempDir()
	cPath := filepath.Join(dir, "c.tmpl")

	if err := os.WriteFile(cPath, []byte(`{{ range (config_refs "db_*") }}{{ . }}{{ end }}`), 0o600); err != nil {
		t.Fatalf("write c: %v", err)
	}

	cfg := &config.Config{
		Project: config.Project{
			Name: "primary",
			Configs: map[string]config.ConfigDef{
				"c":       {Source: cPath},
				"db_main": {Source: "ignored"},
			},
		},
	}

	if _, err := DetectCycles(cfg, false); err != nil {
		t.Fatalf("DetectCycles: %v", err)
	}
}
