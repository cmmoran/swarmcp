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
	version := 17
	want.SecretSources = []PlanSecretSource{{
		SecretName:  "app_secret_abcd1234",
		LogicalName: "app_secret",
		Scope:       PlanScope{Project: "demo", Deployment: "prod", Stack: "core", Partition: "blue"},
		Dependencies: []PlanSecretDependency{{
			Name:     "db_password",
			Scope:    PlanScope{Project: "demo", Deployment: "prod", Stack: "core", Partition: "blue"},
			Hash:     "hash",
			Provider: "vault",
			Addr:     "http://vault.test",
			Mount:    "kv",
			Path:     "demo/prod/core",
			Key:      "db_password",
			Version:  &version,
		}},
	}}
	want.Inputs = []PlanInput{{Kind: "project", Path: "project.yaml", SHA256: "abc123"}}
	want.SourceInputs = []PlanSourceInput{{
		Kind:    "git",
		Origin:  "stack.app.base",
		URL:     "ssh://git@example.com/repo.git",
		Ref:     "v1.2.3",
		Commit:  "0123456789abcdef",
		Path:    "deploy/app",
		Subtree: "abcdef0123456789",
	}}

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
	if got.Secrets.Mode != PlanSecretModePayload {
		t.Fatalf("unexpected secret mode: %q", got.Secrets.Mode)
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
	if len(got.SecretSources) != 1 || got.SecretSources[0].Dependencies[0].Version == nil || *got.SecretSources[0].Dependencies[0].Version != 17 {
		t.Fatalf("unexpected secret sources: %#v", got.SecretSources)
	}
	if len(got.Inputs) != 1 || got.Inputs[0].Kind != "project" || got.Inputs[0].SHA256 != "abc123" {
		t.Fatalf("unexpected inputs: %#v", got.Inputs)
	}
	if len(got.SourceInputs) != 1 || got.SourceInputs[0].Commit != "0123456789abcdef" || got.SourceInputs[0].Subtree != "abcdef0123456789" {
		t.Fatalf("unexpected source inputs: %#v", got.SourceInputs)
	}
}
