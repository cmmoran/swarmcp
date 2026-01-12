package apply

import (
	"testing"
	"time"

	"github.com/cmmoran/swarmcp/internal/swarm"
)

func TestPrunePlanPreservesNewestConfigs(t *testing.T) {
	now := time.Now()
	plan := Plan{
		DeleteConfigs: []swarm.Config{
			{Name: "old", CreatedAt: now.Add(-2 * time.Hour)},
			{Name: "new", CreatedAt: now.Add(-1 * time.Hour)},
		},
	}

	pruned, result := PrunePlan(plan, 1)
	if len(pruned.DeleteConfigs) != 1 {
		t.Fatalf("expected 1 config delete, got %d", len(pruned.DeleteConfigs))
	}
	if pruned.DeleteConfigs[0].Name != "old" {
		t.Fatalf("expected oldest config to be deleted, got %q", pruned.DeleteConfigs[0].Name)
	}
	if result.ConfigsPreserved != 1 {
		t.Fatalf("expected 1 config preserved, got %d", result.ConfigsPreserved)
	}
}

func TestPrunePlanPreservesNewestSecrets(t *testing.T) {
	now := time.Now()
	plan := Plan{
		DeleteSecrets: []swarm.Secret{
			{Name: "old", CreatedAt: now.Add(-2 * time.Hour)},
			{Name: "new", CreatedAt: now.Add(-1 * time.Hour)},
		},
	}

	pruned, result := PrunePlan(plan, 1)
	if len(pruned.DeleteSecrets) != 1 {
		t.Fatalf("expected 1 secret delete, got %d", len(pruned.DeleteSecrets))
	}
	if pruned.DeleteSecrets[0].Name != "old" {
		t.Fatalf("expected oldest secret to be deleted, got %q", pruned.DeleteSecrets[0].Name)
	}
	if result.SecretsPreserved != 1 {
		t.Fatalf("expected 1 secret preserved, got %d", result.SecretsPreserved)
	}
}
