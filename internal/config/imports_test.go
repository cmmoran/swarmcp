package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveImportsStackOverrides(t *testing.T) {
	dir := t.TempDir()
	stackPath := filepath.Join(dir, "stack.yaml")
	overridePath := filepath.Join(dir, "stack.overrides.yaml")
	projectPath := filepath.Join(dir, "project.yaml")

	if err := os.WriteFile(stackPath, []byte(`
services:
  api:
    image: api:1
    replicas: 1
`), 0o644); err != nil {
		t.Fatalf("write stack: %v", err)
	}

	if err := os.WriteFile(overridePath, []byte(`
services:
  api:
    replicas: 2
`), 0o644); err != nil {
		t.Fatalf("write overrides: %v", err)
	}

	if err := os.WriteFile(projectPath, []byte(`
project:
  name: demo
stacks:
  app:
    source:
      path: stack.yaml
      overrides_path: stack.overrides.yaml
    overrides:
      services:
        api:
          replicas: 3
`), 0o644); err != nil {
		t.Fatalf("write project: %v", err)
	}

	cfg, err := LoadWithOptions(projectPath, LoadOptions{})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	service := cfg.Stacks["app"].Services["api"]
	if service.Image != "api:1" {
		t.Fatalf("image mismatch: %q", service.Image)
	}
	if service.Replicas != 3 {
		t.Fatalf("replicas mismatch: %d", service.Replicas)
	}
}

func TestResolveImportsNormalizesTemplates(t *testing.T) {
	dir := t.TempDir()
	stackPath := filepath.Join(dir, "stack.yaml")
	projectPath := filepath.Join(dir, "project.yaml")

	stackContent := "services:\n" +
		"  api:\n" +
		"    image: api:1\n" +
		"    labels:\n" +
		"      traefik.http.routers.api.rule: {{ printf \"Host(`api.%s`)\" (config_value_index \"domain\" 0) }}\n"
	if err := os.WriteFile(stackPath, []byte(stackContent), 0o644); err != nil {
		t.Fatalf("write stack: %v", err)
	}

	if err := os.WriteFile(projectPath, []byte(`
project:
  name: demo
stacks:
  app:
    source:
      path: stack.yaml
`), 0o644); err != nil {
		t.Fatalf("write project: %v", err)
	}

	if _, err := LoadWithOptions(projectPath, LoadOptions{}); err != nil {
		t.Fatalf("load config: %v", err)
	}
}

func TestSourcesPathEscape(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	outsideDir := filepath.Join(root, "outside")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	projectPath := filepath.Join(projectDir, "project.yaml")
	if err := os.WriteFile(projectPath, []byte(`
project:
  name: demo
  sources:
    path: ../outside
  configs:
    demo:
      source: example.txt
`), 0o644); err != nil {
		t.Fatalf("write project: %v", err)
	}

	_, err := LoadWithOptions(projectPath, LoadOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "escapes root") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveImportsServiceOverrides(t *testing.T) {
	dir := t.TempDir()
	servicePath := filepath.Join(dir, "service.yaml")
	projectPath := filepath.Join(dir, "project.yaml")

	if err := os.WriteFile(servicePath, []byte(`
image: worker:1
env:
  LOG_LEVEL: warn
`), 0o644); err != nil {
		t.Fatalf("write service: %v", err)
	}

	if err := os.WriteFile(projectPath, []byte(`
project:
  name: demo
stacks:
  app:
    services:
      worker:
        source:
          path: service.yaml
        overrides:
          env:
            LOG_LEVEL: info
            REGION: us-east-1
`), 0o644); err != nil {
		t.Fatalf("write project: %v", err)
	}

	cfg, err := LoadWithOptions(projectPath, LoadOptions{})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	service := cfg.Stacks["app"].Services["worker"]
	if service.Image != "worker:1" {
		t.Fatalf("image mismatch: %q", service.Image)
	}
	if service.Env["LOG_LEVEL"] != "info" || service.Env["REGION"] != "us-east-1" {
		t.Fatalf("env mismatch: %#v", service.Env)
	}
}

func TestResolveImportsListMerge(t *testing.T) {
	dir := t.TempDir()
	stackPath := filepath.Join(dir, "stack.yaml")
	projectPath := filepath.Join(dir, "project.yaml")

	if err := os.WriteFile(stackPath, []byte(`
services:
  api:
    image: api:1
    ports:
      - target: 80
        published: 8080
        protocol: tcp
        mode: ingress
`), 0o644); err != nil {
		t.Fatalf("write stack: %v", err)
	}

	if err := os.WriteFile(projectPath, []byte(`
project:
  name: demo
stacks:
  app:
    source:
      path: stack.yaml
    overrides:
      services:
        api:
          ports:
            - target: 81
              published: 8081
              protocol: tcp
              mode: ingress
`), 0o644); err != nil {
		t.Fatalf("write project: %v", err)
	}

	cfg, err := LoadWithOptions(projectPath, LoadOptions{})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	ports := cfg.Stacks["app"].Services["api"].Ports
	if len(ports) != 1 || ports[0].Target != 81 {
		t.Fatalf("expected list replace, got %#v", ports)
	}
}

func TestResolveImportsListAppend(t *testing.T) {
	dir := t.TempDir()
	stackPath := filepath.Join(dir, "stack.yaml")
	projectPath := filepath.Join(dir, "project.yaml")

	if err := os.WriteFile(stackPath, []byte(`
services:
  api:
    image: api:1
    ports:
      - target: 80
        published: 8080
        protocol: tcp
        mode: ingress
`), 0o644); err != nil {
		t.Fatalf("write stack: %v", err)
	}

	if err := os.WriteFile(projectPath, []byte(`
project:
  name: demo
stacks:
  app:
    source:
      path: stack.yaml
    overrides:
      services:
        api:
          ports+:
            - target: 81
              published: 8081
              protocol: tcp
              mode: ingress
`), 0o644); err != nil {
		t.Fatalf("write project: %v", err)
	}

	cfg, err := LoadWithOptions(projectPath, LoadOptions{})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	ports := cfg.Stacks["app"].Services["api"].Ports
	if len(ports) != 2 || ports[0].Target != 80 || ports[1].Target != 81 {
		t.Fatalf("expected list append, got %#v", ports)
	}
}

func TestResolveImportsRejectsLocalServiceFields(t *testing.T) {
	dir := t.TempDir()
	servicePath := filepath.Join(dir, "service.yaml")
	projectPath := filepath.Join(dir, "project.yaml")

	if err := os.WriteFile(servicePath, []byte(`
image: worker:1
`), 0o644); err != nil {
		t.Fatalf("write service: %v", err)
	}

	if err := os.WriteFile(projectPath, []byte(`
project:
  name: demo
stacks:
  app:
    services:
      worker:
        source:
          path: service.yaml
        image: worker:2
`), 0o644); err != nil {
		t.Fatalf("write project: %v", err)
	}

	_, err := LoadWithOptions(projectPath, LoadOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "local fields are not allowed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveImportsAllowsAbsoluteSourcePath(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	stackPath := filepath.Join(root, "stack.yaml")
	projectPath := filepath.Join(projectDir, "project.yaml")

	if err := os.WriteFile(stackPath, []byte(`
services:
  api:
    image: api:1
`), 0o644); err != nil {
		t.Fatalf("write stack: %v", err)
	}

	if err := os.WriteFile(projectPath, []byte(`
project:
  name: demo
stacks:
  app:
    source:
      path: `+stackPath+`
`), 0o644); err != nil {
		t.Fatalf("write project: %v", err)
	}

	cfg, err := LoadWithOptions(projectPath, LoadOptions{})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Stacks["app"].Services["api"].Image != "api:1" {
		t.Fatalf("unexpected image: %q", cfg.Stacks["app"].Services["api"].Image)
	}
}

func TestSourcesAbsolutePathRoot(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	sourceDir := filepath.Join(root, "sources")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("mkdir sources: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "example.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write example: %v", err)
	}

	projectPath := filepath.Join(projectDir, "project.yaml")
	if err := os.WriteFile(projectPath, []byte(`
project:
  name: demo
  sources:
    path: `+sourceDir+`
  configs:
    demo:
      source: example.txt
`), 0o644); err != nil {
		t.Fatalf("write project: %v", err)
	}

	cfg, err := LoadWithOptions(projectPath, LoadOptions{})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	source := cfg.Project.Configs["demo"].Source
	if !strings.HasPrefix(source, sourceDir) {
		t.Fatalf("expected source under %q, got %q", sourceDir, source)
	}
}

func TestSourcesAbsolutePathSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	sourceDir := filepath.Join(root, "sources")
	outsideDir := filepath.Join(root, "outside")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("mkdir sources: %v", err)
	}
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outsideDir, "secret.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(sourceDir, "escape")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	projectPath := filepath.Join(projectDir, "project.yaml")
	if err := os.WriteFile(projectPath, []byte(`
project:
  name: demo
  sources:
    path: `+sourceDir+`
  configs:
    demo:
      source: escape/secret.txt
`), 0o644); err != nil {
		t.Fatalf("write project: %v", err)
	}

	_, err := LoadWithOptions(projectPath, LoadOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "escapes root") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestImportAbsolutePathSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	workingDir := filepath.Join(root, "working")
	outsideDir := filepath.Join(root, "outside")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		t.Fatalf("mkdir working: %v", err)
	}
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}

	stackPath := filepath.Join(workingDir, "stack.yaml")
	if err := os.WriteFile(stackPath, []byte(`
configs:
  demo:
    source: escape/secret.txt
services:
  api:
    image: api:1
`), 0o644); err != nil {
		t.Fatalf("write stack: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outsideDir, "secret.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(workingDir, "escape")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	projectPath := filepath.Join(projectDir, "project.yaml")
	if err := os.WriteFile(projectPath, []byte(`
project:
  name: demo
stacks:
  app:
    source:
      path: `+stackPath+`
`), 0o644); err != nil {
		t.Fatalf("write project: %v", err)
	}

	_, err := LoadWithOptions(projectPath, LoadOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "escapes root") {
		t.Fatalf("unexpected error: %v", err)
	}
}
