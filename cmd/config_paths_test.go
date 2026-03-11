package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEffectiveConfigPathsReadsProjectFileOnlyWhenNoExplicitFlags(t *testing.T) {
	prev := opts
	t.Cleanup(func() { opts = prev })

	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	projectPaths := []string{"project.yaml", "release.yaml"}
	if err := writeProjectConfigPaths(projectPaths); err != nil {
		t.Fatalf("write .swarmcp.project: %v", err)
	}

	paths, err := effectiveConfigPaths()
	if err != nil {
		t.Fatalf("effectiveConfigPaths: %v", err)
	}
	if !reflect.DeepEqual(paths, projectPaths) {
		t.Fatalf("expected %v, got %v", projectPaths, paths)
	}

	opts.ConfigPaths = []string{"explicit.yaml"}
	paths, err = effectiveConfigPaths()
	if err != nil {
		t.Fatalf("effectiveConfigPaths explicit: %v", err)
	}
	if !reflect.DeepEqual(paths, []string{"explicit.yaml"}) {
		t.Fatalf("expected explicit config only, got %v", paths)
	}

	opts.ReleaseConfigs = []string{"releases/prod.yaml"}
	paths, err = effectiveConfigPaths()
	if err != nil {
		t.Fatalf("effectiveConfigPaths explicit with release: %v", err)
	}
	if !reflect.DeepEqual(paths, []string{"explicit.yaml", "releases/prod.yaml"}) {
		t.Fatalf("expected explicit config plus release config, got %v", paths)
	}
}

func TestWriteProjectConfigPathsStoresAllConfigs(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	paths := []string{"project.yaml", filepath.Join("releases", "qa.yaml")}
	if err := writeProjectConfigPaths(paths); err != nil {
		t.Fatalf("write .swarmcp.project: %v", err)
	}
	readBack, err := readProjectConfigPaths()
	if err != nil {
		t.Fatalf("read .swarmcp.project: %v", err)
	}
	if !reflect.DeepEqual(readBack, paths) {
		t.Fatalf("expected %v, got %v", paths, readBack)
	}
}
