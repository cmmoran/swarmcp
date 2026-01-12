package apply

import (
	"reflect"
	"testing"

	"github.com/cmmoran/swarmcp/internal/config"
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
