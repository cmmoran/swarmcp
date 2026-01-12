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
