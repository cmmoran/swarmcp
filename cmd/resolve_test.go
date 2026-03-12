package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveCmdPrintsResolvedModel(t *testing.T) {
	prevOpts := opts
	prevOutput := resolveOutput
	prevPath := resolvePath
	t.Cleanup(func() {
		opts = prevOpts
		resolveOutput = prevOutput
		resolvePath = prevPath
	})

	dir := t.TempDir()
	projectPath := filepath.Join(dir, "project.yaml")
	if err := os.WriteFile(projectPath, []byte(`
project:
  name: demo
  partitions: [dev, prod]
stacks:
  core:
    services:
      api:
        image: ghcr.io/acme/api:main
`), 0o644); err != nil {
		t.Fatalf("write project: %v", err)
	}

	opts.ConfigPaths = []string{projectPath}
	var out bytes.Buffer
	resolveCmd.SetOut(&out)
	resolveCmd.SetErr(&out)
	if err := resolveCmd.RunE(resolveCmd, nil); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "project:") || !strings.Contains(got, "stacks:") {
		t.Fatalf("unexpected output: %s", got)
	}
	if !strings.Contains(got, "image: ghcr.io/acme/api:main") {
		t.Fatalf("expected service image in output: %s", got)
	}
}

func TestResolveCmdPrintsSubtreePath(t *testing.T) {
	prevOpts := opts
	prevOutput := resolveOutput
	prevPath := resolvePath
	t.Cleanup(func() {
		opts = prevOpts
		resolveOutput = prevOutput
		resolvePath = prevPath
	})

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
        replicas: 2
`), 0o644); err != nil {
		t.Fatalf("write project: %v", err)
	}

	opts.ConfigPaths = []string{projectPath}
	resolvePath = "stacks.core.services.api.image"
	var out bytes.Buffer
	resolveCmd.SetOut(&out)
	resolveCmd.SetErr(&out)
	if err := resolveCmd.RunE(resolveCmd, nil); err != nil {
		t.Fatalf("resolve path: %v", err)
	}
	got := strings.TrimSpace(out.String())
	if got != "ghcr.io/acme/api:main" {
		t.Fatalf("expected scalar subtree, got %q", got)
	}
}

func TestResolveCmdPrintsJSON(t *testing.T) {
	prevOpts := opts
	prevOutput := resolveOutput
	prevPath := resolvePath
	t.Cleanup(func() {
		opts = prevOpts
		resolveOutput = prevOutput
		resolvePath = prevPath
	})

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
	resolveOutput = "json"
	var out bytes.Buffer
	resolveCmd.SetOut(&out)
	resolveCmd.SetErr(&out)
	if err := resolveCmd.RunE(resolveCmd, nil); err != nil {
		t.Fatalf("resolve json: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, out.String())
	}
	if _, ok := decoded["project"]; !ok {
		t.Fatalf("expected project key in json output: %s", out.String())
	}
}

func TestResolveCmdRejectsRepeatedSelectors(t *testing.T) {
	prevOpts := opts
	prevOutput := resolveOutput
	prevPath := resolvePath
	t.Cleanup(func() {
		opts = prevOpts
		resolveOutput = prevOutput
		resolvePath = prevPath
	})

	opts.ConfigPaths = []string{"project.yaml"}
	opts.Partitions = []string{"dev", "prod"}

	err := resolveCmd.RunE(resolveCmd, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "multiple --partition values are not supported") {
		t.Fatalf("unexpected error: %v", err)
	}
}
