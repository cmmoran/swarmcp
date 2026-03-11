package cmdutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cmmoran/swarmcp/internal/swarm"
)

func TestLoadProjectContextPartitionValidation(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "project.yaml")
	writeFile(t, configPath, "project:\n  name: demo\n  partitions:\n    - a\n")

	_, err := LoadProjectContext(ProjectOptions{
		ConfigPath: configPath,
		Partition:  "missing",
	}, false, false)
	if err == nil {
		t.Fatalf("expected partition validation error")
	}

	ctx, err := LoadProjectContext(ProjectOptions{
		ConfigPath: configPath,
		Partition:  "a",
	}, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.Partition != "a" {
		t.Fatalf("expected partition 'a', got %q", ctx.Partition)
	}
}

func TestLoadProjectContextValuesAndSecrets(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "project.yaml")
	writeFile(t, configPath, "project:\n  name: demo\n")

	valuesDir := filepath.Join(dir, "values")
	if err := os.MkdirAll(valuesDir, 0o755); err != nil {
		t.Fatalf("values dir: %v", err)
	}
	writeFile(t, filepath.Join(valuesDir, "values.yaml"), "global:\n  foo: bar\n")
	writeFile(t, filepath.Join(dir, "secrets.yaml"), "values:\n  token: abc\n")

	ctx, err := LoadProjectContext(ProjectOptions{
		ConfigPath: configPath,
	}, true, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.Secrets == nil || ctx.Secrets.Values["token"] != "abc" {
		t.Fatalf("expected secrets to be loaded")
	}
	values, ok := ctx.Values.(map[string]any)
	if !ok {
		t.Fatalf("expected values map, got %T", ctx.Values)
	}
	global, ok := values["global"].(map[string]any)
	if !ok || global["foo"] != "bar" {
		t.Fatalf("expected values.global.foo to be 'bar'")
	}
}

func TestProjectContextSwarmClientWrap(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "project.yaml")
	writeFile(t, configPath, "project:\n  name: demo\n")

	ctx, err := LoadProjectContext(ProjectOptions{
		ConfigPath:    configPath,
		Context:       "demo",
		ClientFactory: func(string) (swarm.Client, error) { return nil, swarm.ErrNotImplemented },
	}, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = ctx.SwarmClient()
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "swarm client not implemented") {
		t.Fatalf("expected wrapped swarm client error, got %q", err.Error())
	}
}

func TestLoadProjectContextAppliesReleaseConfigValidationAndMerge(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "project.yaml")
	releasePath := filepath.Join(dir, "release.yaml")
	writeFile(t, configPath, `project:
  name: demo
stacks:
  core:
    services:
      api:
        image: ghcr.io/acme/api:main
        replicas: 1
`)
	writeFile(t, releasePath, `stacks:
  core:
    services:
      api:
        image: ghcr.io/acme/api:1.2.3
        replicas: 2
`)

	ctx, err := LoadProjectContext(ProjectOptions{
		ConfigPaths:        []string{configPath},
		ReleaseConfigPaths: []string{releasePath},
		ConfigPath:         configPath,
	}, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	service := ctx.Config.Stacks["core"].Services["api"]
	if service.Image != "ghcr.io/acme/api:1.2.3" || service.Replicas != 2 {
		t.Fatalf("expected release override, got %#v", service)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
