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

func TestLoadResolvedModelTraceIncludesOverlayLayers(t *testing.T) {
	dir := t.TempDir()
	projectPath := filepath.Join(dir, "project.yaml")
	if err := os.WriteFile(projectPath, []byte(`
project:
  name: demo
  deployment: qa
stacks:
  core:
    services:
      api:
        image: ghcr.io/acme/api:base
overlays:
  deployments:
    qa:
      stacks:
        core:
          services:
            api:
              image: ghcr.io/acme/api:overlay
`), 0o644); err != nil {
		t.Fatalf("write project: %v", err)
	}

	resolved, err := LoadResolvedModelTrace(ResolvedModelOptions{
		ConfigPaths: []string{projectPath},
		LoadOptions: LoadOptions{},
	}, []string{"stacks", "core", "services", "api", "image"})
	if err != nil {
		t.Fatalf("load resolved model trace: %v", err)
	}
	if resolved.Trace == nil {
		t.Fatalf("expected trace")
	}
	if len(resolved.Trace.OverlayLayers) != 1 {
		t.Fatalf("expected 1 overlay layer, got %#v", resolved.Trace.OverlayLayers)
	}
	if resolved.Trace.OverlayLayers[0].Label != "project deployment overlay" {
		t.Fatalf("unexpected overlay label: %#v", resolved.Trace.OverlayLayers[0])
	}
	if resolved.Trace.OverlayLayers[0].Value != "ghcr.io/acme/api:overlay" {
		t.Fatalf("unexpected overlay value: %#v", resolved.Trace.OverlayLayers[0])
	}
}

func TestLoadResolvedModelTraceIncludesResolvedSourcesOverlayLayers(t *testing.T) {
	dir := t.TempDir()
	projectPath := filepath.Join(dir, "project.yaml")
	if err := os.WriteFile(projectPath, []byte(`
project:
  name: demo
  deployment: qa
stacks:
  core:
    sources:
      ref: stack-base
    overlays:
      deployments:
        qa:
          sources:
            ref: stack-qa
`), 0o644); err != nil {
		t.Fatalf("write project: %v", err)
	}

	resolved, err := LoadResolvedModelTrace(ResolvedModelOptions{
		ConfigPaths: []string{projectPath},
		LoadOptions: LoadOptions{},
	}, []string{"stacks", "core", "sources", "ref"})
	if err != nil {
		t.Fatalf("load resolved model trace: %v", err)
	}
	if resolved.Trace == nil {
		t.Fatalf("expected trace")
	}
	if len(resolved.Trace.OverlayLayers) != 1 {
		t.Fatalf("expected 1 overlay layer, got %#v", resolved.Trace.OverlayLayers)
	}
	if resolved.Trace.OverlayLayers[0].Label != "stack deployment overlay" {
		t.Fatalf("unexpected overlay label: %#v", resolved.Trace.OverlayLayers[0])
	}
	if resolved.Trace.OverlayLayers[0].Value != "stack-qa" {
		t.Fatalf("unexpected overlay value: %#v", resolved.Trace.OverlayLayers[0])
	}
}

func TestLoadResolvedModelTraceIncludesConfigOverlayLayers(t *testing.T) {
	dir := t.TempDir()
	projectPath := filepath.Join(dir, "project.yaml")
	baseConfigPath := filepath.Join(dir, "base.yaml")
	overlayConfigPath := filepath.Join(dir, "overlay.yaml")
	if err := os.WriteFile(baseConfigPath, []byte("base: true\n"), 0o644); err != nil {
		t.Fatalf("write base config: %v", err)
	}
	if err := os.WriteFile(overlayConfigPath, []byte("overlay: true\n"), 0o644); err != nil {
		t.Fatalf("write overlay config: %v", err)
	}
	if err := os.WriteFile(projectPath, []byte(`
project:
  name: demo
  deployment: qa
stacks:
  core:
    configs:
      app:
        source: base.yaml
    overlays:
      deployments:
        qa:
          configs:
            app:
              source: overlay.yaml
`), 0o644); err != nil {
		t.Fatalf("write project: %v", err)
	}

	resolved, err := LoadResolvedModelTrace(ResolvedModelOptions{
		ConfigPaths: []string{projectPath},
		LoadOptions: LoadOptions{},
	}, []string{"stacks", "core", "configs", "app", "source"})
	if err != nil {
		t.Fatalf("load resolved model trace: %v", err)
	}
	if resolved.Trace == nil {
		t.Fatalf("expected trace")
	}
	if len(resolved.Trace.OverlayLayers) != 1 {
		t.Fatalf("expected 1 overlay layer, got %#v", resolved.Trace.OverlayLayers)
	}
	if resolved.Trace.OverlayLayers[0].Label != "stack deployment overlay" {
		t.Fatalf("unexpected overlay label: %#v", resolved.Trace.OverlayLayers[0])
	}
	if resolved.Trace.OverlayLayers[0].Value != overlayConfigPath {
		t.Fatalf("unexpected overlay value: %#v", resolved.Trace.OverlayLayers[0])
	}
}
