package apply

import (
	"testing"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/render"
	"github.com/cmmoran/swarmcp/internal/templates"
	"github.com/docker/docker/api/types/mount"
)

func TestDesiredVolumeMountsServiceScoped(t *testing.T) {
	cfg := &config.Config{
		Project: config.Project{
			Name: "primary",
			Defaults: config.ProjectDefaults{
				Volumes: config.VolumeDefaults{
					BasePath: "/srv/data",
				},
			},
		},
	}
	stack := config.Stack{
		Mode:     "shared",
		Volumes:  map[string]config.VolumeDef{},
		Services: map[string]config.Service{},
	}
	service := config.Service{
		Volumes: []config.VolumeRef{
			{Name: "traefik_data", Target: "/data"},
		},
	}

	scope := templates.Scope{Project: cfg.Project.Name, Stack: "core", Service: "ingress"}
	_, engine, _, data := render.NewServiceTemplateEngine(cfg, scope, nil, false, nil)
	mounts, err := desiredVolumeMounts(cfg, engine, data, "core", stack, "", "ingress", service)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(mounts))
	}
	if mounts[0].Type != mount.TypeBind {
		t.Fatalf("unexpected mount type: %#v", mounts[0])
	}
	if mounts[0].Source != "/srv/data/primary/core/ingress/traefik_data" {
		t.Fatalf("unexpected source: %#v", mounts[0].Source)
	}
	if mounts[0].Target != "/data" {
		t.Fatalf("unexpected target: %#v", mounts[0].Target)
	}
}

func TestDesiredVolumeMountsStackScoped(t *testing.T) {
	cfg := &config.Config{
		Project: config.Project{
			Name: "primary",
			Defaults: config.ProjectDefaults{
				Volumes: config.VolumeDefaults{
					BasePath: "/srv/data",
				},
			},
		},
	}
	stack := config.Stack{
		Mode: "shared",
		Volumes: map[string]config.VolumeDef{
			"db": {Target: "/var/lib/db"},
		},
	}
	service := config.Service{
		Volumes: []config.VolumeRef{
			{Name: "db", Target: "/db"},
		},
	}

	scope := templates.Scope{Project: cfg.Project.Name, Stack: "core", Service: "api"}
	_, engine, _, data := render.NewServiceTemplateEngine(cfg, scope, nil, false, nil)
	mounts, err := desiredVolumeMounts(cfg, engine, data, "core", stack, "", "api", service)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(mounts))
	}
	if mounts[0].Source != "/srv/data/primary/core/db" {
		t.Fatalf("unexpected source: %#v", mounts[0].Source)
	}
	if mounts[0].Target != "/db" {
		t.Fatalf("unexpected target: %#v", mounts[0].Target)
	}
}

func TestDesiredVolumeMountsServiceStandard(t *testing.T) {
	cfg := &config.Config{
		Project: config.Project{
			Name: "primary",
			Defaults: config.ProjectDefaults{
				Volumes: config.VolumeDefaults{
					BasePath: "/srv/data",
				},
			},
		},
	}
	stack := config.Stack{
		Mode:     "shared",
		Volumes:  map[string]config.VolumeDef{},
		Services: map[string]config.Service{},
	}
	service := config.Service{
		Volumes: []config.VolumeRef{
			{Standard: "service"},
		},
	}

	scope := templates.Scope{Project: cfg.Project.Name, Stack: "core", Service: "postgres"}
	_, engine, _, data := render.NewServiceTemplateEngine(cfg, scope, nil, false, nil)
	mounts, err := desiredVolumeMounts(cfg, engine, data, "core", stack, "", "postgres", service)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(mounts))
	}
	if mounts[0].Source != "/srv/data/primary/core/postgres" {
		t.Fatalf("unexpected source: %#v", mounts[0].Source)
	}
	if mounts[0].Target != "/data" {
		t.Fatalf("unexpected target: %#v", mounts[0].Target)
	}
}

func TestDesiredVolumeMountsServiceStandardOverrides(t *testing.T) {
	cfg := &config.Config{
		Project: config.Project{
			Name: "primary",
			Defaults: config.ProjectDefaults{
				Volumes: config.VolumeDefaults{
					BasePath: "/srv/data",
				},
			},
		},
	}
	stack := config.Stack{
		Mode:     "shared",
		Volumes:  map[string]config.VolumeDef{},
		Services: map[string]config.Service{},
	}
	service := config.Service{
		Volumes: []config.VolumeRef{
			{Standard: "service", Target: "/var/lib/postgresql/data", Subpath: "data"},
		},
	}

	scope := templates.Scope{Project: cfg.Project.Name, Stack: "core", Service: "postgres"}
	_, engine, _, data := render.NewServiceTemplateEngine(cfg, scope, nil, false, nil)
	mounts, err := desiredVolumeMounts(cfg, engine, data, "core", stack, "", "postgres", service)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(mounts))
	}
	if mounts[0].Source != "/srv/data/primary/core/postgres/data" {
		t.Fatalf("unexpected source: %#v", mounts[0].Source)
	}
	if mounts[0].Target != "/var/lib/postgresql/data" {
		t.Fatalf("unexpected target: %#v", mounts[0].Target)
	}
}
