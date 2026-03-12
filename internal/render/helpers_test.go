package render

import (
	"strings"
	"testing"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/templates"
)

func TestScopeHelpers(t *testing.T) {
	serviceScope := templates.Scope{Project: "demo", Stack: "core", Partition: "blue", Service: "api"}
	if got := scopeLabel(serviceScope); got != "stack core partition blue service api" {
		t.Fatalf("unexpected service scope label: %q", got)
	}
	if got := runtimeNodeID("config", serviceScope, "app"); got != "config:stack core partition blue service api:app" {
		t.Fatalf("unexpected runtime node id: %q", got)
	}
	if !containsFilter([]string{"core", "tools"}, "core") {
		t.Fatalf("expected containsFilter to match present value")
	}
	if containsFilter([]string{"core", "tools"}, "api") {
		t.Fatalf("did not expect containsFilter to match absent value")
	}
}

func TestLabelHelpers(t *testing.T) {
	name, hash := PhysicalName("logical-name", "content")
	if !strings.HasPrefix(name, "logical-name_") {
		t.Fatalf("unexpected physical name prefix: %q", name)
	}
	if len(hash) != 64 {
		t.Fatalf("unexpected hash length: %d", len(hash))
	}

	labels := Labels(templates.Scope{Project: "demo"}, "cfg", hash)
	if labels[LabelPartition] != "none" || labels[LabelStack] != "none" || labels[LabelService] != "none" {
		t.Fatalf("expected empty scope fields to normalize to none: %#v", labels)
	}
	formatted := FormatLabels(labels)
	if !strings.Contains(formatted, LabelProject+"=demo") || !strings.Contains(formatted, LabelName+"=cfg") {
		t.Fatalf("unexpected formatted labels: %q", formatted)
	}
	if FormatLabels(nil) != "" {
		t.Fatalf("expected empty label formatting for nil map")
	}
}

func TestWithNetworkScope(t *testing.T) {
	internal := true
	cfg := &config.Config{
		Project: config.Project{
			Name:       "demo",
			Partitions: []string{"blue"},
			Defaults:   config.ProjectDefaults{Networks: config.NetworkDefaults{Shared: []string{"shared-net"}}},
		},
		Stacks: map[string]config.Stack{
			"core": {
				Mode: "partitioned",
				Services: map[string]config.Service{
					"api": {NetworkEphemeral: &config.ServiceNetworkEphemeral{Internal: &internal}},
				},
			},
		},
	}

	scope := withNetworkScope(cfg, templates.Scope{Project: "demo", Stack: "core", Partition: "blue", Service: "api"})
	if scope.NetworkEphemeral == "" {
		t.Fatalf("expected ephemeral network name to be populated")
	}
	if !strings.Contains(scope.NetworksShared, "shared-net") {
		t.Fatalf("expected shared networks to be included in scope: %#v", scope)
	}

	projectOnly := withNetworkScope(cfg, templates.Scope{Project: "demo"})
	if projectOnly.NetworkEphemeral != "" {
		t.Fatalf("did not expect project scope to get ephemeral network")
	}
}
