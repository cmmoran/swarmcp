package apply

import (
	"testing"

	"github.com/cmmoran/swarmcp/internal/render"
	"github.com/cmmoran/swarmcp/internal/templates"
	"github.com/docker/docker/api/types/mount"
	dockerapi "github.com/docker/docker/api/types/swarm"
)

func TestIntentDiffsIgnoreOrderForUnorderedFields(t *testing.T) {
	current := serviceIntent{
		Image:    "nginx:latest",
		Workdir:  "/",
		Mode:     "replicated",
		Replicas: 1,
		Labels: map[string]string{
			"service": "web",
		},
		Env:         []string{"B=2", "A=1"},
		Constraints: []string{"node.labels.zone==east", "node.role==worker"},
		Networks:    []string{"net-b", "net-a"},
		Ports: []portIntent{
			{
				Target:    443,
				Published: 8443,
				Protocol:  dockerapi.PortConfigProtocolTCP,
				Mode:      dockerapi.PortConfigPublishModeIngress,
			},
			{
				Target:    80,
				Published: 8080,
				Protocol:  dockerapi.PortConfigProtocolTCP,
				Mode:      dockerapi.PortConfigPublishModeIngress,
			},
		},
		Configs: []ServiceMount{
			{Name: "cfg-b", Target: "/etc/b", UID: "0", GID: "0", Mode: 0o444},
			{Name: "cfg-a", Target: "/etc/a", UID: "0", GID: "0", Mode: 0o444},
		},
		Secrets: []ServiceMount{
			{Name: "sec-b", Target: "/run/sec/b", UID: "0", GID: "0", Mode: 0o444},
			{Name: "sec-a", Target: "/run/sec/a", UID: "0", GID: "0", Mode: 0o444},
		},
		Volumes: []mount.Mount{
			{Type: mount.TypeVolume, Source: "data", Target: "/data", ReadOnly: true},
			{Type: mount.TypeBind, Source: "/srv/logs", Target: "/logs", ReadOnly: false},
		},
	}

	desired := serviceIntent{
		Image:    "nginx:latest",
		Workdir:  "/",
		Mode:     "replicated",
		Replicas: 1,
		Labels: map[string]string{
			"service": "web",
		},
		Env:         []string{"A=1", "B=2"},
		Constraints: []string{"node.role==worker", "node.labels.zone==east"},
		Networks:    []string{"net-a", "net-b"},
		Ports: []portIntent{
			{
				Target:    80,
				Published: 8080,
				Protocol:  dockerapi.PortConfigProtocolTCP,
				Mode:      dockerapi.PortConfigPublishModeIngress,
			},
			{
				Target:    443,
				Published: 8443,
				Protocol:  dockerapi.PortConfigProtocolTCP,
				Mode:      dockerapi.PortConfigPublishModeIngress,
			},
		},
		Configs: []ServiceMount{
			{Name: "cfg-a", Target: "/etc/a", UID: "0", GID: "0", Mode: 0o444},
			{Name: "cfg-b", Target: "/etc/b", UID: "0", GID: "0", Mode: 0o444},
		},
		Secrets: []ServiceMount{
			{Name: "sec-a", Target: "/run/sec/a", UID: "0", GID: "0", Mode: 0o444},
			{Name: "sec-b", Target: "/run/sec/b", UID: "0", GID: "0", Mode: 0o444},
		},
		Volumes: []mount.Mount{
			{Type: mount.TypeBind, Source: "/srv/logs", Target: "/logs", ReadOnly: false},
			{Type: mount.TypeVolume, Source: "data", Target: "/data", ReadOnly: true},
		},
	}

	if diffs := intentDiffs(current, desired); len(diffs) != 0 {
		t.Fatalf("expected no diffs, got %v", diffs)
	}
	if !intentEqual(current, desired) {
		t.Fatalf("expected intents to be equal")
	}
}

func TestRenderLabelTemplatesExpandsTokensInKeys(t *testing.T) {
	engine := templates.New(render.NoopResolver{})
	data := render.TemplateData{
		Project:   "primary",
		Stack:     "core",
		Partition: "dev",
		Service:   "api",
	}
	scope := templates.Scope{
		Project:   "primary",
		Stack:     "core",
		Partition: "dev",
		Service:   "api",
	}
	labels := map[string]string{
		"traefik.http.routers.{partition}.rule": "host",
	}
	rendered, err := renderLabelTemplates(engine, data, scope, labels)
	if err != nil {
		t.Fatalf("render label templates: %v", err)
	}
	if _, ok := rendered["traefik.http.routers.dev.rule"]; !ok {
		t.Fatalf("expected expanded label key, got %v", rendered)
	}
}
