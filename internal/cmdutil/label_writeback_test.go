package cmdutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cmmoran/swarmcp/internal/config"
	"go.yaml.in/yaml/v4"
)

func TestWriteAutoNodeLabelsAddsAndKeeps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "swarmcp.yaml")
	if err := os.WriteFile(path, []byte(`
project:
  name: demo
  defaults:
    volumes:
      node_label_key: node.volume
  nodes:
    node-1:
      volumes: [data, cache]
      labels:
        env: prod
        node.volume.old: "true"
  deployment_targets:
    prod:
      overrides:
        node-1:
          volumes: [data]
          labels:
            node.volume.old: "true"
stacks: {}
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	result, err := WriteAutoNodeLabels(AutoLabelWriteOptions{
		ConfigPath: path,
		Config:     cfg,
	})
	if err != nil {
		t.Fatalf("write auto labels: %v", err)
	}
	if !result.Changed {
		t.Fatalf("expected changes to be written")
	}

	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read updated config: %v", err)
	}
	var parsed config.Config
	if err := yaml.Unmarshal(updated, &parsed); err != nil {
		t.Fatalf("unmarshal updated config: %v", err)
	}

	labels := parsed.Project.Nodes["node-1"].Labels
	if labels["node.volume.data"] != "true" {
		t.Fatalf("expected node.volume.data label to be written")
	}
	if labels["node.volume.cache"] != "true" {
		t.Fatalf("expected node.volume.cache label to be written")
	}
	if labels["node.volume.old"] != "true" {
		t.Fatalf("expected node.volume.old label to be preserved")
	}

	override := parsed.Project.Targets["prod"].Overrides["node-1"].Labels
	if override["node.volume.data"] != "true" {
		t.Fatalf("expected override node.volume.data label to be written")
	}
	if override["node.volume.old"] != "true" {
		t.Fatalf("expected override node.volume.old label to be preserved")
	}
}

func TestWriteAutoNodeLabelsPrune(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "swarmcp.yaml")
	if err := os.WriteFile(path, []byte(`
project:
  name: demo
  defaults:
    volumes:
      node_label_key: node.volume
  nodes:
    node-1:
      volumes: [data, cache]
      labels:
        env: prod
        node.volume.cache: "true"
        node.volume.old: "true"
  deployment_targets:
    prod:
      overrides:
        node-1:
          volumes: [data, cache]
          labels:
            node.volume.cache: "true"
            node.volume.old: "true"
stacks:
  core:
    services:
      api:
        image: nginx
        volumes:
          - name: data
            target: /data
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	result, err := WriteAutoNodeLabels(AutoLabelWriteOptions{
		ConfigPath: path,
		Config:     cfg,
		Prune:      true,
	})
	if err != nil {
		t.Fatalf("write auto labels: %v", err)
	}
	if result.Pruned == 0 {
		t.Fatalf("expected pruned labels")
	}

	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read updated config: %v", err)
	}
	var parsed config.Config
	if err := yaml.Unmarshal(updated, &parsed); err != nil {
		t.Fatalf("unmarshal updated config: %v", err)
	}

	labels := parsed.Project.Nodes["node-1"].Labels
	if labels["node.volume.data"] != "true" {
		t.Fatalf("expected node.volume.data label to remain")
	}
	if _, ok := labels["node.volume.cache"]; ok {
		t.Fatalf("expected node.volume.cache label to be pruned")
	}
	if _, ok := labels["node.volume.old"]; ok {
		t.Fatalf("expected node.volume.old label to be pruned")
	}

	override := parsed.Project.Targets["prod"].Overrides["node-1"].Labels
	if override["node.volume.data"] != "true" {
		t.Fatalf("expected override node.volume.data label to remain")
	}
	if _, ok := override["node.volume.cache"]; ok {
		t.Fatalf("expected override node.volume.cache label to be pruned")
	}
	if _, ok := override["node.volume.old"]; ok {
		t.Fatalf("expected override node.volume.old label to be pruned")
	}
}
