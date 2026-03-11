package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExplainConfigPathIncludesRepeatedConfigAndOverlayLayers(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "project.yaml")
	releasePath := filepath.Join(dir, "release.yaml")
	if err := os.WriteFile(basePath, []byte(`
project:
  name: demo
  deployment: qa
stacks:
  core:
    services:
      api:
        image: ghcr.io/acme/api:main
overlays:
  deployments:
    qa:
      stacks:
        core:
          services:
            api:
              image: ghcr.io/acme/api:2.0.0
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

	result, err := ExplainConfigPath(ExplainOptions{
		ConfigPaths: []string{basePath, releasePath},
		LoadOptions: LoadOptions{},
	}, "stacks.core.services.api.image")
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	if got := result.Winner; got != "project deployment overlay" {
		t.Fatalf("expected project deployment overlay winner, got %q", got)
	}
	if len(result.Layers) < 3 {
		t.Fatalf("expected at least 3 layers, got %#v", result.Layers)
	}
	if result.Layers[0].Label != "config "+basePath {
		t.Fatalf("unexpected first layer: %#v", result.Layers[0])
	}
	if result.Layers[1].Label != "config "+releasePath {
		t.Fatalf("unexpected second layer: %#v", result.Layers[1])
	}
}

func TestExplainConfigPathIncludesImportSourceLayer(t *testing.T) {
	dir := t.TempDir()
	servicePath := filepath.Join(dir, "service.yaml")
	projectPath := filepath.Join(dir, "project.yaml")
	if err := os.WriteFile(servicePath, []byte(`
image: ghcr.io/acme/api:imported
`), 0o644); err != nil {
		t.Fatalf("write service: %v", err)
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

	result, err := ExplainConfigPath(ExplainOptions{
		ConfigPaths: []string{projectPath},
		LoadOptions: LoadOptions{},
	}, "stacks.core.services.api.image")
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	found := false
	for _, layer := range result.Layers {
		if strings.HasPrefix(layer.Label, "import service source ") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected import service source layer, got %#v", result.Layers)
	}
}

func TestExplainConfigPathInvalidTraversal(t *testing.T) {
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

	_, err := ExplainConfigPath(ExplainOptions{
		ConfigPaths: []string{projectPath},
		LoadOptions: LoadOptions{},
	}, "stacks.core.services.api.image.tag")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "invalid field path") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExplainConfigPathUnknownPathIncludesSuggestions(t *testing.T) {
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
        env:
          LOG_LEVEL: info
`), 0o644); err != nil {
		t.Fatalf("write project: %v", err)
	}

	_, err := ExplainConfigPath(ExplainOptions{
		ConfigPaths: []string{projectPath},
		LoadOptions: LoadOptions{},
	}, "stacks.core.services.api.imag")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "available:") || !strings.Contains(err.Error(), "image") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExplainConfigPathDistinguishesProjectAndStackOverlayLabels(t *testing.T) {
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
          services:
            api:
              image: ghcr.io/acme/api:stack
overlays:
  deployments:
    qa:
      stacks:
        core:
          services:
            api:
              image: ghcr.io/acme/api:project
`), 0o644); err != nil {
		t.Fatalf("write project: %v", err)
	}

	result, err := ExplainConfigPath(ExplainOptions{
		ConfigPaths: []string{projectPath},
		LoadOptions: LoadOptions{},
	}, "stacks.core.services.api.image")
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	if len(result.Layers) < 3 {
		t.Fatalf("expected layered result, got %#v", result.Layers)
	}
	labels := []string{result.Layers[1].Label, result.Layers[2].Label}
	if labels[0] != "project deployment overlay" || labels[1] != "stack deployment overlay" {
		t.Fatalf("unexpected labels: %#v", labels)
	}
}

func TestDebugResolvedMapAppliesOverlaySources(t *testing.T) {
	cfg := &Config{
		Project: Project{
			Name:       "demo",
			Deployment: "qa",
			Sources:    Sources{Ref: "base"},
		},
		Stacks: map[string]Stack{
			"core": {
				Sources: Sources{Ref: "stack-base"},
				Partitions: map[string]StackPartition{
					"dev": {Sources: Sources{Ref: "part-base"}},
				},
				Overlays: StackOverlays{
					Deployments: map[string]OverlayStack{
						"qa": {Sources: Sources{Ref: "stack-qa"}},
					},
				},
			},
		},
		Overlays: Overlays{
			Deployments: map[string]Overlay{
				"qa": {
					Project: OverlayProject{Sources: Sources{Ref: "project-qa"}},
					Stacks: map[string]OverlayStack{
						"core": {
							Sources: Sources{Ref: "project-stack-qa"},
							Partitions: map[string]OverlayPartition{
								"dev": {Sources: Sources{Ref: "project-part-qa"}},
							},
						},
					},
				},
			},
		},
	}

	resolved, err := DebugResolvedMap(cfg, "dev", []string{"core"})
	if err != nil {
		t.Fatalf("debug resolved: %v", err)
	}
	projectMap, ok := resolved["project"].(map[string]any)
	if !ok {
		t.Fatalf("expected project map")
	}
	projectSources, ok := projectMap["sources"].(map[string]any)
	if !ok || projectSources["ref"] != "project-qa" {
		t.Fatalf("unexpected project sources: %#v", projectMap["sources"])
	}
	stacksMap := resolved["stacks"].(map[string]any)
	coreMap := stacksMap["core"].(map[string]any)
	stackSources := coreMap["sources"].(map[string]any)
	if stackSources["ref"] != "stack-qa" {
		t.Fatalf("unexpected stack sources: %#v", stackSources)
	}
	partitionsMap := coreMap["partitions"].(map[string]any)
	devMap := partitionsMap["dev"].(map[string]any)
	partSources := devMap["sources"].(map[string]any)
	if partSources["ref"] != "project-part-qa" {
		t.Fatalf("unexpected partition sources: %#v", partSources)
	}
}
