package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExplainCmdOutputsLayers(t *testing.T) {
	prev := opts
	t.Cleanup(func() { opts = prev })

	dir := t.TempDir()
	basePath := filepath.Join(dir, "project.yaml")
	releasePath := filepath.Join(dir, "release.yaml")
	if err := os.WriteFile(basePath, []byte(`
project:
  name: demo
stacks:
  core:
    services:
      api:
        image: ghcr.io/acme/api:main
`), 0o644); err != nil {
		t.Fatalf("write base: %v", err)
	}
	if err := os.WriteFile(releasePath, []byte(`
stacks:
  core:
    services:
      api:
        image: ghcr.io/acme/api:1.8.4
`), 0o644); err != nil {
		t.Fatalf("write release: %v", err)
	}

	opts.ConfigPaths = []string{basePath, releasePath}
	var out bytes.Buffer
	explainCmd.SetOut(&out)
	explainCmd.SetErr(&out)
	if err := explainCmd.RunE(explainCmd, []string{"stacks.core.services.api.image"}); err != nil {
		t.Fatalf("explain: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "explain OK") || !strings.Contains(got, "winner:") {
		t.Fatalf("unexpected output: %s", got)
	}
	if !strings.Contains(got, "config "+basePath) || !strings.Contains(got, "config "+releasePath) {
		t.Fatalf("expected config layers in output: %s", got)
	}
}

func TestExplainCmdRejectsMultiTargetSelectors(t *testing.T) {
	prev := opts
	t.Cleanup(func() { opts = prev })
	opts.ConfigPaths = []string{"project.yaml"}
	opts.Deployments = []string{"qa", "prod"}

	err := explainCmd.RunE(explainCmd, []string{"project.deployment"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "single-target only") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExplainCmdUnknownPathSuggestsAvailableFields(t *testing.T) {
	prev := opts
	t.Cleanup(func() { opts = prev })

	dir := t.TempDir()
	projectPath := filepath.Join(dir, "project.yaml")
	if err := os.WriteFile(projectPath, []byte(`
project:
  name: demo
stacks:
  core:
    services:
      api:
        image: ghcr.io/acme/api:main
`), 0o644); err != nil {
		t.Fatalf("write project: %v", err)
	}

	opts.ConfigPaths = []string{projectPath}
	err := explainCmd.RunE(explainCmd, []string{"stacks.core.services.api.imag"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "available:") || !strings.Contains(err.Error(), "image") {
		t.Fatalf("unexpected error: %v", err)
	}
}
