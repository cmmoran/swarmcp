package apply

import (
	"reflect"
	"testing"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/swarm"
)

func TestDesiredServiceNetworksSharedStack(t *testing.T) {
	cfg := &config.Config{
		Project: config.Project{
			Name:       "primary",
			Partitions: []string{"dev", "qa"},
		},
	}
	service := config.Service{Egress: true}

	got := desiredServiceNetworks(cfg, "core", "shared", "", "ingress", service)
	want := []string{
		"primary_core",
		"primary_egress",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected networks: %#v", got)
	}
}

func TestDesiredServiceNetworksPartitionedStack(t *testing.T) {
	cfg := &config.Config{
		Project: config.Project{
			Name:       "primary",
			Partitions: []string{"dev", "qa"},
		},
	}
	service := config.Service{}

	got := desiredServiceNetworks(cfg, "app", "partitioned", "dev", "api", service)
	want := []string{
		"primary_dev_app",
		"primary_dev_internal",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected networks: %#v", got)
	}
}

func TestDesiredServiceNetworksSharedDefaults(t *testing.T) {
	cfg := &config.Config{
		Project: config.Project{
			Name: "primary",
			Defaults: config.ProjectDefaults{
				Networks: config.NetworkDefaults{
					Shared: []string{"{project}_core", "{project}_{partition}_extras"},
				},
			},
		},
	}
	service := config.Service{}

	got := desiredServiceNetworks(cfg, "app", "partitioned", "dev", "api", service)
	want := []string{
		"primary_dev_app",
		"primary_core",
		"primary_dev_extras",
		"primary_dev_internal",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected networks: %#v", got)
	}
}

func TestDesiredServiceNetworksEphemeral(t *testing.T) {
	cfg := &config.Config{
		Project: config.Project{
			Name:       "primary",
			Partitions: []string{"dev"},
		},
	}
	service := config.Service{
		NetworkEphemeral: &config.ServiceNetworkEphemeral{},
	}

	got := desiredServiceNetworks(cfg, "app", "partitioned", "dev", "api", service)
	want := []string{
		"primary_dev_app",
		"primary_dev_internal",
		"primary_dev_app_svc_api",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected networks: %#v", got)
	}
}

func TestDesiredNetworksExcludesStacksNotIncludedInDeployment(t *testing.T) {
	cfg := &config.Config{
		Project: config.Project{
			Name:       "demo",
			Deployment: "prod",
			Partitions: []string{"dev", "prod"},
			Defaults: config.ProjectDefaults{
				Networks: config.NetworkDefaults{
					Shared: []string{"{project}_core", "{project}_{partition}_shared"},
				},
			},
		},
		Stacks: map[string]config.Stack{
			"tools": {
				Mode: "shared",
				IncludedIn: []config.InclusionRule{
					{Deployments: []string{"nonprod"}},
				},
				Services: map[string]config.Service{
					"drone": {Egress: true},
				},
			},
			"participant": {
				Mode: "partitioned",
				IncludedIn: []config.InclusionRule{
					{Deployments: []string{"prod"}, Partitions: []string{"prod"}},
				},
				Services: map[string]config.Service{
					"api": {},
				},
			},
		},
	}

	got := DesiredNetworks(cfg, nil, nil)
	names := make([]string, 0, len(got))
	for _, item := range got {
		names = append(names, item.Name)
	}

	want := []string{
		"demo_core",
		"demo_prod_internal",
		"demo_prod_participant",
		"demo_prod_shared",
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("unexpected networks: got=%v want=%v", names, want)
	}
}

func TestDesiredNetworksExcludesPartitionsNotIncludedInDeployment(t *testing.T) {
	cfg := &config.Config{
		Project: config.Project{
			Name:       "demo",
			Deployment: "prod",
			Partitions: []string{"dev", "prod"},
			Defaults: config.ProjectDefaults{
				Networks: config.NetworkDefaults{
					Shared: []string{"{project}_core", "{project}_{partition}_shared"},
				},
			},
		},
		Stacks: map[string]config.Stack{
			"participant": {
				Mode: "partitioned",
				Services: map[string]config.Service{
					"api": {
						IncludedIn: []config.InclusionRule{
							{Deployments: []string{"prod"}, Partitions: []string{"prod"}},
						},
					},
				},
			},
		},
	}

	got := DesiredNetworks(cfg, nil, nil)
	names := make([]string, 0, len(got))
	for _, item := range got {
		names = append(names, item.Name)
	}

	want := []string{
		"demo_core",
		"demo_prod_internal",
		"demo_prod_participant",
		"demo_prod_shared",
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("unexpected networks: got=%v want=%v", names, want)
	}
}

func networkNames(specs []swarm.NetworkSpec) []string {
	out := make([]string, 0, len(specs))
	for _, spec := range specs {
		out = append(out, spec.Name)
	}
	return out
}
