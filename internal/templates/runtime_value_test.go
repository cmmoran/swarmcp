package templates

import (
	"testing"

	"github.com/cmmoran/swarmcp/internal/config"
)

func TestRuntimeValueStandardVolumesCSV(t *testing.T) {
	cfg := &config.Config{
		Project: config.Project{
			Name: "platform",
			Defaults: config.ProjectDefaults{
				Volumes: config.VolumeDefaults{
					BasePath:        "/srv/data",
					ServiceStandard: "persist",
					ServiceTarget:   "/data",
				},
			},
		},
		Stacks: map[string]config.Stack{
			"tools": {
				Mode: "shared",
				Services: map[string]config.Service{
					"drone-runner": {
						Volumes: []config.VolumeRef{
							{
								Standard: "persist",
								Subpath:  "gomod",
								Target:   "/go/pkg/mod",
								Category: "cache",
							},
							{
								Standard: "persist",
								Subpath:  "gobuild",
								Target:   "/root/.cache/go-build",
								Category: "cache",
							},
							{
								Standard: "persist",
								Category: "other_category",
							},
						},
					},
				},
			},
		},
	}
	scope := Scope{
		Project: "platform",
		Stack:   "tools",
		Service: "drone-runner",
	}
	resolver := NewScopeResolver(cfg, scope, false, false, nil, nil, nil)

	out, err := resolver.RuntimeValue("standard_volumes", "standard=persist", "category=cache", "_format=csv")
	if err != nil {
		t.Fatalf("runtime_value: %v", err)
	}
	want := "/srv/data/platform/tools/drone-runner/gomod:/go/pkg/mod,/srv/data/platform/tools/drone-runner/gobuild:/root/.cache/go-build"
	if out != want {
		t.Fatalf("unexpected output: %q", out)
	}
}
