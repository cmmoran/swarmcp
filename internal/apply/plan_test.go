package apply

import (
	"context"
	"testing"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/render"
	"github.com/cmmoran/swarmcp/internal/swarm"
	dockerapi "github.com/docker/docker/api/types/swarm"
)

type fakeClient struct {
	configs  []swarm.Config
	secrets  []swarm.Secret
	services []swarm.Service
	networks []swarm.Network
}

func (f *fakeClient) ListConfigs(ctx context.Context) ([]swarm.Config, error) {
	return f.configs, nil
}

func (f *fakeClient) ListSecrets(ctx context.Context) ([]swarm.Secret, error) {
	return f.secrets, nil
}

func (f *fakeClient) CreateConfig(ctx context.Context, spec swarm.ConfigSpec) (string, error) {
	return "config-id", nil
}

func (f *fakeClient) CreateSecret(ctx context.Context, spec swarm.SecretSpec) (string, error) {
	return "secret-id", nil
}

func (f *fakeClient) ListServices(ctx context.Context) ([]swarm.Service, error) {
	return f.services, nil
}

func (f *fakeClient) ListNetworks(ctx context.Context) ([]swarm.Network, error) {
	return f.networks, nil
}

func (f *fakeClient) ListNodes(ctx context.Context) ([]swarm.Node, error) {
	return nil, nil
}

func (f *fakeClient) ConfigContent(ctx context.Context, id string) ([]byte, error) {
	return nil, nil
}

func (f *fakeClient) CreateNetwork(ctx context.Context, spec swarm.NetworkSpec) (string, error) {
	return "network-id", nil
}

func (f *fakeClient) CreateService(ctx context.Context, spec dockerapi.ServiceSpec) (string, error) {
	return "service-id", nil
}

func (f *fakeClient) RemoveConfig(ctx context.Context, id string) error {
	return nil
}

func (f *fakeClient) RemoveSecret(ctx context.Context, id string) error {
	return nil
}

func (f *fakeClient) UpdateService(ctx context.Context, service swarm.Service, spec dockerapi.ServiceSpec) error {
	return nil
}

func (f *fakeClient) UpdateNode(ctx context.Context, node swarm.Node, spec dockerapi.NodeSpec) error {
	return nil
}

func TestBuildPlanFiltersExistingNamesAndDeletesStale(t *testing.T) {
	client := &fakeClient{
		configs: []swarm.Config{
			{Name: "cfg-present", ID: "cfg-1", Labels: map[string]string{"swarmcp.io/managed": "true", "swarmcp.io/project": "primary"}},
			{Name: "cfg-stale", ID: "cfg-2", Labels: map[string]string{"swarmcp.io/managed": "true", "swarmcp.io/project": "primary"}},
			{Name: "cfg-foreign", ID: "cfg-3", Labels: map[string]string{"swarmcp.io/managed": "true", "swarmcp.io/project": "other"}},
		},
		secrets: []swarm.Secret{
			{Name: "sec-present", ID: "sec-1", Labels: map[string]string{"swarmcp.io/managed": "true", "swarmcp.io/project": "primary"}},
			{Name: "sec-stale", ID: "sec-2", Labels: map[string]string{"swarmcp.io/managed": "true", "swarmcp.io/project": "primary"}},
		},
	}
	cfg := &config.Config{
		Project: config.Project{Name: "primary"},
	}

	desired := DesiredState{
		Configs: []swarm.ConfigSpec{
			{Name: "cfg-present"},
			{Name: "cfg-missing"},
		},
		Secrets: []swarm.SecretSpec{
			{Name: "sec-present"},
			{Name: "sec-missing"},
		},
	}

	plan, err := BuildPlan(context.Background(), client, cfg, desired, nil, nil, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.CreateConfigs) != 1 || plan.CreateConfigs[0].Name != "cfg-missing" {
		t.Fatalf("unexpected config plan: %#v", plan.CreateConfigs)
	}
	if len(plan.CreateSecrets) != 1 || plan.CreateSecrets[0].Name != "sec-missing" {
		t.Fatalf("unexpected secret plan: %#v", plan.CreateSecrets)
	}
	if len(plan.DeleteConfigs) != 1 || plan.DeleteConfigs[0].Name != "cfg-stale" {
		t.Fatalf("unexpected config deletes: %#v", plan.DeleteConfigs)
	}
	if len(plan.DeleteSecrets) != 1 || plan.DeleteSecrets[0].Name != "sec-stale" {
		t.Fatalf("unexpected secret deletes: %#v", plan.DeleteSecrets)
	}
	if got := plan.Assumptions.AbsentConfigs; len(got) != 1 || got[0] != "cfg-missing" {
		t.Fatalf("unexpected absent config assumptions: %#v", got)
	}
	if got := plan.Assumptions.AbsentSecrets; len(got) != 1 || got[0] != "sec-missing" {
		t.Fatalf("unexpected absent secret assumptions: %#v", got)
	}
	if got := plan.Assumptions.PresentConfigs; len(got) != 1 || got[0].Name != "cfg-stale" || got[0].ID != "cfg-2" {
		t.Fatalf("unexpected present config assumptions: %#v", got)
	}
	if got := plan.Assumptions.PresentSecrets; len(got) != 1 || got[0].Name != "sec-stale" || got[0].ID != "sec-2" {
		t.Fatalf("unexpected present secret assumptions: %#v", got)
	}
}

func TestBuildPlanDedupesDesiredNames(t *testing.T) {
	client := &fakeClient{}
	cfg := &config.Config{
		Project: config.Project{Name: "primary"},
	}

	desired := DesiredState{
		Configs: []swarm.ConfigSpec{
			{Name: "cfg-dup"},
			{Name: "cfg-dup"},
		},
		Secrets: []swarm.SecretSpec{
			{Name: "sec-dup"},
			{Name: "sec-dup"},
		},
	}

	plan, err := BuildPlan(context.Background(), client, cfg, desired, nil, nil, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.CreateConfigs) != 1 || plan.CreateConfigs[0].Name != "cfg-dup" {
		t.Fatalf("unexpected config plan: %#v", plan.CreateConfigs)
	}
	if len(plan.CreateSecrets) != 1 || plan.CreateSecrets[0].Name != "sec-dup" {
		t.Fatalf("unexpected secret plan: %#v", plan.CreateSecrets)
	}
}

func TestBuildPlanRecordsStackServiceAssumptions(t *testing.T) {
	client := &fakeClient{
		services: []swarm.Service{{
			Name:    "primary_core_api",
			ID:      "svc-1",
			Version: 42,
			Labels: map[string]string{
				render.LabelManaged:   "true",
				render.LabelProject:   "primary",
				render.LabelStack:     "core",
				render.LabelPartition: "none",
				render.LabelService:   "api",
			},
			Spec: dockerapi.ServiceSpec{
				Annotations: dockerapi.Annotations{
					Name: "primary_core_api",
				},
				TaskTemplate: dockerapi.TaskSpec{
					ContainerSpec: &dockerapi.ContainerSpec{Image: "nginx:old"},
				},
			},
		}},
	}
	cfg := &config.Config{
		Project: config.Project{Name: "primary"},
		Stacks: map[string]config.Stack{
			"core": {
				Mode: "shared",
				Services: map[string]config.Service{
					"api":    {Image: "nginx:new"},
					"worker": {Image: "busybox:latest"},
				},
			},
		},
	}

	plan, err := BuildPlan(context.Background(), client, cfg, DesiredState{}, nil, nil, nil, false)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if got := plan.Assumptions.AbsentServices; len(got) != 1 || got[0] != "primary_core_worker" {
		t.Fatalf("unexpected absent service assumptions: %#v", got)
	}
	if got := plan.Assumptions.PresentServices; len(got) != 1 || got[0].Name != "primary_core_api" || got[0].ID != "svc-1" || got[0].Version != 42 {
		t.Fatalf("unexpected present service assumptions: %#v", got)
	}
}

func TestBuildPlanAssumptionsIncludePruneOnlyStacks(t *testing.T) {
	assumptions := buildPlanAssumptions(Plan{
		PruneStacks: []string{"primary_core"},
	}, nil, []swarm.Service{{
		Name:    "primary_core_api",
		ID:      "svc-1",
		Version: 42,
		Labels: map[string]string{
			render.LabelManaged:   "true",
			render.LabelProject:   "primary",
			render.LabelStack:     "core",
			render.LabelPartition: "none",
			render.LabelService:   "api",
		},
	}})

	if got := assumptions.PresentServices; len(got) != 1 || got[0].Name != "primary_core_api" || got[0].ID != "svc-1" || got[0].Version != 42 {
		t.Fatalf("unexpected present service assumptions: %#v", got)
	}
}
