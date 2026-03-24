package cmdutil

import (
	"testing"

	"github.com/cmmoran/swarmcp/internal/config"
)

func TestSelectDeploymentNodesByName(t *testing.T) {
	cfg, err := config.Load("../../examples/primary/project.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	target, ok := cfg.Project.Targets["nonprod"]
	if !ok {
		t.Fatalf("expected nonprod target")
	}
	selected := selectDeploymentNodes(cfg.Project.Nodes, target)
	if len(selected) == 0 {
		for name, node := range cfg.Project.Nodes {
			t.Logf("match %q => %v", name, matchesNodeSelector(name, node, target.Include))
		}
		t.Fatalf("expected selected nodes, got 0 (include names=%v labels=%v)", target.Include.Names, target.Include.Labels)
	}
	if _, ok := selected["inclusion-awu"]; !ok {
		t.Fatalf("expected inclusion-awu selected")
	}
	if _, ok := selected["inclusion-nuc"]; !ok {
		t.Fatalf("expected inclusion-nuc selected")
	}
	if _, ok := selected["inclusion-sa"]; !ok {
		t.Fatalf("expected inclusion-sa selected")
	}
}

func TestFilterDeploymentPartitionsUsesDeploymentAllowlist(t *testing.T) {
	cfg := &config.Config{
		Project: config.Project{
			Partitions: []string{"dev", "qa", "prod"},
			Deployment: "prod",
			Targets: config.DeploymentTargets{
				"prod": {
					Partitions: []string{"qa", "prod"},
				},
			},
		},
	}

	got := FilterDeploymentPartitions(cfg, []string{"dev", "qa", "prod"})
	want := []string{"qa", "prod"}
	if len(got) != len(want) {
		t.Fatalf("unexpected partition count: got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected partitions: got=%v want=%v", got, want)
		}
	}
}
