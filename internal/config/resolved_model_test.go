package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadResolvedModelAndLookupResolvedPath(t *testing.T) {
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
        image: ghcr.io/acme/api:1.2.3
`), 0o644); err != nil {
		t.Fatalf("write release: %v", err)
	}

	resolved, err := LoadResolvedModel(ResolvedModelOptions{
		ConfigPaths:        []string{basePath},
		ReleaseConfigPaths: []string{releasePath},
		LoadOptions:        LoadOptions{},
	})
	if err != nil {
		t.Fatalf("load resolved model: %v", err)
	}
	value, err := LookupResolvedPath(resolved.Model, "stacks.core.services.api.image")
	if err != nil {
		t.Fatalf("lookup resolved path: %v", err)
	}
	if got, ok := value.(string); !ok || got != "ghcr.io/acme/api:1.2.3" {
		t.Fatalf("unexpected resolved value: %#v", value)
	}
}
