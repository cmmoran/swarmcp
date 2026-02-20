package apply

import (
	"testing"
	"time"

	"github.com/cmmoran/swarmcp/internal/render"
	"github.com/cmmoran/swarmcp/internal/swarm"
)

func TestPrunePlanPreservesNewestConfigs(t *testing.T) {
	now := time.Now()
	labels := map[string]string{
		render.LabelManaged:   "true",
		render.LabelProject:   "primary",
		render.LabelStack:     "core",
		render.LabelPartition: "none",
		render.LabelName:      "shared-config",
	}
	plan := Plan{
		DeleteConfigs: []swarm.Config{
			{Name: "old", CreatedAt: now.Add(-2 * time.Hour), Labels: labels},
			{Name: "new", CreatedAt: now.Add(-1 * time.Hour), Labels: labels},
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
	labels := map[string]string{
		render.LabelManaged:   "true",
		render.LabelProject:   "primary",
		render.LabelStack:     "core",
		render.LabelPartition: "none",
		render.LabelName:      "shared-secret",
	}
	plan := Plan{
		DeleteSecrets: []swarm.Secret{
			{Name: "old", CreatedAt: now.Add(-2 * time.Hour), Labels: labels},
			{Name: "new", CreatedAt: now.Add(-1 * time.Hour), Labels: labels},
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

func TestPrunePlanPreservesPerPartitionForLogicalConfig(t *testing.T) {
	now := time.Now()
	labels := func(partition string) map[string]string {
		return map[string]string{
			render.LabelManaged:   "true",
			render.LabelProject:   "primary",
			render.LabelStack:     "core",
			render.LabelPartition: partition,
			render.LabelName:      "traefik.yml",
		}
	}
	plan := Plan{
		DeleteConfigs: []swarm.Config{
			{Name: "cfg-a-old", CreatedAt: now.Add(-4 * time.Hour), Labels: labels("a")},
			{Name: "cfg-a-new", CreatedAt: now.Add(-3 * time.Hour), Labels: labels("a")},
			{Name: "cfg-b-old", CreatedAt: now.Add(-2 * time.Hour), Labels: labels("b")},
			{Name: "cfg-b-new", CreatedAt: now.Add(-1 * time.Hour), Labels: labels("b")},
		},
	}

	pruned, result := PrunePlan(plan, 1)
	if len(pruned.DeleteConfigs) != 2 {
		t.Fatalf("expected 2 config deletes, got %d", len(pruned.DeleteConfigs))
	}
	deleted := map[string]struct{}{}
	for _, cfg := range pruned.DeleteConfigs {
		deleted[cfg.Name] = struct{}{}
	}
	if _, ok := deleted["cfg-a-old"]; !ok {
		t.Fatalf("expected cfg-a-old to be deleted")
	}
	if _, ok := deleted["cfg-b-old"]; !ok {
		t.Fatalf("expected cfg-b-old to be deleted")
	}
	if result.ConfigsPreserved != 2 {
		t.Fatalf("expected 2 configs preserved, got %d", result.ConfigsPreserved)
	}
}

func TestPrunePlanIgnoresServiceLabelForSecretPreserveScope(t *testing.T) {
	now := time.Now()
	labels := func(service string) map[string]string {
		return map[string]string{
			render.LabelManaged:   "true",
			render.LabelProject:   "primary",
			render.LabelStack:     "core",
			render.LabelPartition: "none",
			render.LabelName:      "shared-secret",
			render.LabelService:   service,
		}
	}
	plan := Plan{
		DeleteSecrets: []swarm.Secret{
			{Name: "sec-older", CreatedAt: now.Add(-2 * time.Hour), Labels: labels("svc-a")},
			{Name: "sec-newer", CreatedAt: now.Add(-1 * time.Hour), Labels: labels("svc-b")},
		},
	}

	pruned, result := PrunePlan(plan, 1)
	if len(pruned.DeleteSecrets) != 1 {
		t.Fatalf("expected 1 secret delete, got %d", len(pruned.DeleteSecrets))
	}
	if pruned.DeleteSecrets[0].Name != "sec-older" {
		t.Fatalf("expected oldest secret to be deleted, got %q", pruned.DeleteSecrets[0].Name)
	}
	if result.SecretsPreserved != 1 {
		t.Fatalf("expected 1 secret preserved, got %d", result.SecretsPreserved)
	}
}
