package apply

import (
	"context"
	"strings"
	"testing"

	"github.com/cmmoran/swarmcp/internal/swarm"
	dockerapi "github.com/docker/docker/api/types/swarm"
)

func TestValidatePlanAssumptionsRejectsCreatedConfigAlreadyPresent(t *testing.T) {
	client := &fakeClient{
		configs: []swarm.Config{{Name: "cfg-new", ID: "cfg-1"}},
	}
	assumptions := PlanAssumptions{
		AbsentConfigs: []string{"cfg-new"},
	}

	err := ValidatePlanAssumptions(context.Background(), client, assumptions)
	if err == nil || !strings.Contains(err.Error(), "config \"cfg-new\" now exists") {
		t.Fatalf("expected existing config assumption failure, got %v", err)
	}
}

func TestValidatePlanAssumptionsRejectsDeletedSecretReplacement(t *testing.T) {
	client := &fakeClient{
		secrets: []swarm.Secret{{Name: "sec-old", ID: "replacement"}},
	}
	assumptions := PlanAssumptions{
		PresentSecrets: []ResourceAssumption{{Name: "sec-old", ID: "original"}},
	}

	err := ValidatePlanAssumptions(context.Background(), client, assumptions)
	if err == nil || !strings.Contains(err.Error(), "secret \"sec-old\" id changed") {
		t.Fatalf("expected replaced secret assumption failure, got %v", err)
	}
}

func TestValidatePlanAssumptionsRejectsServiceVersionDrift(t *testing.T) {
	client := &fakeClient{
		services: []swarm.Service{{Name: "primary_core_api", ID: "svc-1", Version: 12}},
	}
	assumptions := PlanAssumptions{
		PresentServices: []ServiceAssumption{{Name: "primary_core_api", ID: "svc-1", Version: 11}},
	}

	err := ValidatePlanAssumptions(context.Background(), client, assumptions)
	if err == nil || !strings.Contains(err.Error(), "service \"primary_core_api\" version changed") {
		t.Fatalf("expected service version assumption failure, got %v", err)
	}
}

func TestValidatePlanAssumptionsRejectsDeletedConfigNowInUse(t *testing.T) {
	client := &fakeClient{
		configs: []swarm.Config{{Name: "cfg-old", ID: "cfg-1"}},
		services: []swarm.Service{{
			Name: "primary_core_api",
			Spec: dockerapi.ServiceSpec{
				TaskTemplate: dockerapi.TaskSpec{
					ContainerSpec: &dockerapi.ContainerSpec{
						Configs: []*dockerapi.ConfigReference{{
							ConfigID:   "cfg-1",
							ConfigName: "cfg-old",
						}},
					},
				},
			},
		}},
	}
	assumptions := PlanAssumptions{
		PresentConfigs: []ResourceAssumption{{Name: "cfg-old", ID: "cfg-1"}},
	}

	err := ValidatePlanAssumptions(context.Background(), client, assumptions)
	if err == nil || !strings.Contains(err.Error(), "config \"cfg-old\" is now in use") {
		t.Fatalf("expected in-use config assumption failure, got %v", err)
	}
}

func TestValidatePlanAssumptionsAcceptsMatchingState(t *testing.T) {
	client := &fakeClient{
		configs:  []swarm.Config{{Name: "cfg-old", ID: "cfg-1"}},
		secrets:  []swarm.Secret{{Name: "sec-old", ID: "sec-1"}},
		networks: []swarm.Network{{Name: "existing-net", ID: "net-1"}},
		services: []swarm.Service{{Name: "primary_core_api", ID: "svc-1", Version: 11}},
	}
	assumptions := PlanAssumptions{
		AbsentConfigs:   []string{"cfg-new"},
		AbsentSecrets:   []string{"sec-new"},
		AbsentNetworks:  []string{"new-net"},
		AbsentServices:  []string{"primary_core_worker"},
		PresentConfigs:  []ResourceAssumption{{Name: "cfg-old", ID: "cfg-1"}},
		PresentSecrets:  []ResourceAssumption{{Name: "sec-old", ID: "sec-1"}},
		PresentServices: []ServiceAssumption{{Name: "primary_core_api", ID: "svc-1", Version: 11}},
	}

	if err := ValidatePlanAssumptions(context.Background(), client, assumptions); err != nil {
		t.Fatalf("ValidatePlanAssumptions: %v", err)
	}
}

func TestFinalizePlanAssumptionsMatchesFinalOperations(t *testing.T) {
	plan := Plan{
		CreateConfigs: []swarm.ConfigSpec{{Name: "cfg-new"}},
		DeleteConfigs: []swarm.Config{{Name: "cfg-stale", ID: "cfg-1"}},
		StackDeploys:  []StackDeploy{{Name: "primary_core"}},
		PruneStacks:   []string{"primary_old"},
		Assumptions: PlanAssumptions{
			AbsentConfigs: []string{"cfg-new", "cfg-not-in-final-plan"},
			PresentConfigs: []ResourceAssumption{
				{Name: "cfg-stale", ID: "cfg-1"},
				{Name: "cfg-preserved", ID: "cfg-2"},
			},
			PresentServices: []ServiceAssumption{
				{Name: "primary_core_api", ID: "original-svc", Stack: "primary_core", Version: 11},
				{Name: "primary_old_worker", ID: "prune-only-svc", Stack: "primary_old", Version: 12},
			},
		},
	}

	final := FinalizePlanAssumptions(plan)
	if got := final.Assumptions.AbsentConfigs; len(got) != 1 || got[0] != "cfg-new" {
		t.Fatalf("unexpected absent configs: %#v", got)
	}
	if got := final.Assumptions.PresentConfigs; len(got) != 1 || got[0].Name != "cfg-stale" || got[0].ID != "cfg-1" {
		t.Fatalf("unexpected present configs: %#v", got)
	}
	if got := final.Assumptions.PresentServices; len(got) != 1 || got[0].Name != "primary_core_api" || got[0].ID != "original-svc" || got[0].Stack != "primary_core" || got[0].Version != 11 {
		t.Fatalf("unexpected present services: %#v", got)
	}
}
