package apply

import (
	"context"
	"testing"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/swarm"
	dockerapi "github.com/docker/docker/api/types/swarm"
)

type fakeClient struct {
	configs  []swarm.Config
	secrets  []swarm.Secret
	services []swarm.Service
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
	return nil, nil
}

func (f *fakeClient) ListNodes(ctx context.Context) ([]swarm.Node, error) {
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

	plan, err := BuildPlan(context.Background(), client, cfg, desired, nil, "", false)
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

	plan, err := BuildPlan(context.Background(), client, cfg, desired, nil, "", false)
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
