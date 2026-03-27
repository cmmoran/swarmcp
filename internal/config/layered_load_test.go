package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFilesWithOptionsMergesRepeatedConfigs(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "project.yaml")
	overlayPath := filepath.Join(dir, "release.yaml")
	if err := os.WriteFile(basePath, []byte(`
project:
  name: demo
stacks:
  core:
    services:
      api:
        image: ghcr.io/acme/api:main
        command: ["serve"]
        env:
          LOG_LEVEL: info
`), 0o644); err != nil {
		t.Fatalf("write base: %v", err)
	}
	if err := os.WriteFile(overlayPath, []byte(`
stacks:
  core:
    services:
      api:
        image: ghcr.io/acme/api:1.8.4
        command: ["serve", "--debug"]
        env:
          FEATURE_FLAG_X: "true"
`), 0o644); err != nil {
		t.Fatalf("write overlay: %v", err)
	}

	cfg, err := LoadFilesWithOptions([]string{basePath, overlayPath}, LoadOptions{})
	if err != nil {
		t.Fatalf("load configs: %v", err)
	}
	service := cfg.Stacks["core"].Services["api"]
	if service.Image != "ghcr.io/acme/api:1.8.4" {
		t.Fatalf("expected overlay image, got %q", service.Image)
	}
	if got := strings.Join(service.Command, ","); got != "serve,--debug" {
		t.Fatalf("expected list replace, got %q", got)
	}
	if service.Env["LOG_LEVEL"] != "info" || service.Env["FEATURE_FLAG_X"] != "true" {
		t.Fatalf("expected env maps to merge, got %#v", service.Env)
	}
	if cfg.BaseDir != dir {
		t.Fatalf("expected base dir %q, got %q", dir, cfg.BaseDir)
	}
}

func TestLoadFilesWithOptionsReplacesServiceIncludedIn(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "project.yaml")
	overlayPath := filepath.Join(dir, "overlay.yaml")
	if err := os.WriteFile(basePath, []byte(`
project:
  name: demo
stacks:
  core:
    services:
      api:
        image: ghcr.io/acme/api:main
        included_in:
          - deployments: [dev]
          - partitions: [blue]
`), 0o644); err != nil {
		t.Fatalf("write base: %v", err)
	}
	if err := os.WriteFile(overlayPath, []byte(`
stacks:
  core:
    services:
      api:
        included_in:
          - deployments: [prod]
`), 0o644); err != nil {
		t.Fatalf("write overlay: %v", err)
	}

	cfg, err := LoadFilesWithOptions([]string{basePath, overlayPath}, LoadOptions{})
	if err != nil {
		t.Fatalf("load configs: %v", err)
	}
	rules := cfg.Stacks["core"].Services["api"].IncludedIn
	if len(rules) != 1 {
		t.Fatalf("expected included_in replace, got %#v", rules)
	}
	if got := strings.Join(rules[0].Deployments, ","); got != "prod" {
		t.Fatalf("expected overlay included_in, got %#v", rules)
	}
}

func TestLoadFilesWithOptionsRejectsLaterImportOverrides(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "project.yaml")
	overlayPath := filepath.Join(dir, "release.yaml")
	if err := os.WriteFile(basePath, []byte("project:\n  name: demo\n"), 0o644); err != nil {
		t.Fatalf("write base: %v", err)
	}
	if err := os.WriteFile(overlayPath, []byte(`
stacks:
  core:
    overrides:
      services:
        api:
          image: ghcr.io/acme/api:1.8.4
`), 0o644); err != nil {
		t.Fatalf("write overlay: %v", err)
	}

	_, err := LoadFilesWithOptions([]string{basePath, overlayPath}, LoadOptions{})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "stacks.core.overrides is not allowed in later config files") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadFilesWithReleaseOptionsMergesAllowedReleaseFields(t *testing.T) {
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
        replicas: 2
        env:
          LOG_LEVEL: info
        labels:
          tier: api
        update_config:
          parallelism: 1
          order: start-first
`), 0o644); err != nil {
		t.Fatalf("write base: %v", err)
	}
	if err := os.WriteFile(releasePath, []byte(`
stacks:
  core:
    services:
      api:
        image: ghcr.io/acme/api@sha256:deadbeef
        replicas: 3
        env:
          FEATURE_FLAG_X: "true"
        update_config:
          parallelism: 2
`), 0o644); err != nil {
		t.Fatalf("write release: %v", err)
	}

	cfg, err := LoadFilesWithReleaseOptions([]string{basePath}, []string{releasePath}, LoadOptions{})
	if err != nil {
		t.Fatalf("load configs: %v", err)
	}
	service := cfg.Stacks["core"].Services["api"]
	if service.Image != "ghcr.io/acme/api@sha256:deadbeef" {
		t.Fatalf("expected release image, got %q", service.Image)
	}
	if service.Replicas != 3 {
		t.Fatalf("expected release replicas, got %d", service.Replicas)
	}
	if service.Env["LOG_LEVEL"] != "info" || service.Env["FEATURE_FLAG_X"] != "true" {
		t.Fatalf("expected merged env map, got %#v", service.Env)
	}
	if service.UpdateConfig == nil || service.UpdateConfig.Parallelism == nil || *service.UpdateConfig.Parallelism != 2 {
		t.Fatalf("expected update_config.parallelism override, got %#v", service.UpdateConfig)
	}
	if service.UpdateConfig.Order == nil || *service.UpdateConfig.Order != "start-first" {
		t.Fatalf("expected update_config.order to be preserved, got %#v", service.UpdateConfig)
	}
}

func TestMergeReleaseOverlayMapPreservesSourceObjectWhenOverridingRef(t *testing.T) {
	base := map[string]any{
		"stacks": map[string]any{
			"core": map[string]any{
				"source": map[string]any{
					"path": "stack.yaml",
				},
			},
		},
	}
	release := map[string]any{
		"stacks": map[string]any{
			"core": map[string]any{
				"source": map[string]any{
					"ref": "2026.03.11-1842",
				},
			},
		},
	}

	merged, err := mergeReleaseOverlayMap(base, release)
	if err != nil {
		t.Fatalf("merge release overlay: %v", err)
	}
	source, ok := lookupExistingMap(merged, []string{"stacks", "core", "source"})
	if !ok {
		t.Fatalf("expected merged source map")
	}
	if source["path"] != "stack.yaml" {
		t.Fatalf("expected source.path to be preserved, got %#v", source)
	}
	if source["ref"] != "2026.03.11-1842" {
		t.Fatalf("expected source.ref override, got %#v", source)
	}
}

func TestLoadFilesWithReleaseOptionsRejectsTopologyFields(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "project.yaml")
	releasePath := filepath.Join(dir, "release.yaml")
	if err := os.WriteFile(basePath, []byte("project:\n  name: demo\n"), 0o644); err != nil {
		t.Fatalf("write base: %v", err)
	}
	if err := os.WriteFile(releasePath, []byte(`
project:
  partitions: [prod]
`), 0o644); err != nil {
		t.Fatalf("write release: %v", err)
	}

	_, err := LoadFilesWithReleaseOptions([]string{basePath}, []string{releasePath}, LoadOptions{})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "project is not allowed in release config files") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadFilesWithReleaseOptionsRejectsUnknownService(t *testing.T) {
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
      worker:
        image: ghcr.io/acme/worker:1.0.0
`), 0o644); err != nil {
		t.Fatalf("write release: %v", err)
	}

	_, err := LoadFilesWithReleaseOptions([]string{basePath}, []string{releasePath}, LoadOptions{})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "stacks.core.services.worker does not exist in the base config") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadFilesWithReleaseTraceRecordsDocumentLayers(t *testing.T) {
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

	_, trace, err := LoadFilesWithReleaseTrace([]string{basePath}, []string{releasePath}, LoadOptions{}, []string{"stacks", "core", "services", "api", "image"})
	if err != nil {
		t.Fatalf("load with trace: %v", err)
	}
	if trace == nil {
		t.Fatalf("expected trace")
	}
	if len(trace.Layers) != 2 {
		t.Fatalf("expected 2 layers, got %#v", trace.Layers)
	}
	if trace.Layers[0].Label != "config "+basePath {
		t.Fatalf("unexpected first label: %#v", trace.Layers[0])
	}
	if trace.Layers[1].Label != "release config "+releasePath {
		t.Fatalf("unexpected second label: %#v", trace.Layers[1])
	}
	if value, ok := lookupPathValue(trace.MergedDoc, []string{"stacks", "core", "services", "api", "image"}); !ok || value != "ghcr.io/acme/api:1.2.3" {
		t.Fatalf("unexpected merged doc value: %#v", trace.MergedDoc)
	}
}

func TestLoadFilesWithReleaseTraceRecordsImportLayers(t *testing.T) {
	dir := t.TempDir()
	servicePath := filepath.Join(dir, "service.yaml")
	projectPath := filepath.Join(dir, "project.yaml")
	if err := os.WriteFile(servicePath, []byte(`
image: ghcr.io/acme/api:main
`), 0o644); err != nil {
		t.Fatalf("write service source: %v", err)
	}
	if err := os.WriteFile(projectPath, []byte(`
project:
  name: demo
stacks:
  core:
    services:
      api:
        source:
          path: service.yaml
`), 0o644); err != nil {
		t.Fatalf("write project: %v", err)
	}

	_, trace, err := LoadFilesWithReleaseTrace([]string{projectPath}, nil, LoadOptions{}, []string{"stacks", "core", "services", "api", "image"})
	if err != nil {
		t.Fatalf("load with trace: %v", err)
	}
	if trace == nil {
		t.Fatalf("expected trace")
	}
	if len(trace.ImportLayers) != 1 {
		t.Fatalf("expected 1 import layer, got %#v", trace.ImportLayers)
	}
	if trace.ImportLayers[0].Label != "import service source "+servicePath {
		t.Fatalf("unexpected import label: %#v", trace.ImportLayers[0])
	}
	if trace.ImportLayers[0].Value != "ghcr.io/acme/api:main" {
		t.Fatalf("unexpected import value: %#v", trace.ImportLayers[0])
	}
}
