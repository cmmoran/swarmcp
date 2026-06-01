package apply

import (
	"path/filepath"
	"testing"

	"github.com/cmmoran/swarmcp/internal/swarm"
)

func TestPlanFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.yaml")
	want := NewPlanFile("test-version", "demo", "prod", "blue", "core", "prod-context", true, Plan{
		CreateConfigs: []swarm.ConfigSpec{{
			Name: "app_cfg_abcd1234",
			Labels: map[string]string{
				"swarmcp.io/name": "app_cfg",
			},
			Data: []byte("config"),
		}},
		StackDeploys: []StackDeploy{{
			Name:           "demo_blue_core",
			Compose:        []byte("services: {}\n"),
			ServiceCreates: 1,
		}},
	})

	if err := WritePlanFile(path, want); err != nil {
		t.Fatalf("WritePlanFile: %v", err)
	}
	got, err := ReadPlanFile(path)
	if err != nil {
		t.Fatalf("ReadPlanFile: %v", err)
	}
	if got.APIVersion != PlanFileAPIVersion {
		t.Fatalf("unexpected api version: %q", got.APIVersion)
	}
	if got.Project != "demo" || got.Deployment != "prod" || got.Partition != "blue" || got.Stack != "core" || got.Context != "prod-context" {
		t.Fatalf("unexpected plan metadata: %#v", got)
	}
	if len(got.Plan.CreateConfigs) != 1 || string(got.Plan.CreateConfigs[0].Data) != "config" {
		t.Fatalf("unexpected config payload: %#v", got.Plan.CreateConfigs)
	}
	if len(got.Plan.StackDeploys) != 1 || string(got.Plan.StackDeploys[0].Compose) != "services: {}\n" {
		t.Fatalf("unexpected stack deploys: %#v", got.Plan.StackDeploys)
	}
}
