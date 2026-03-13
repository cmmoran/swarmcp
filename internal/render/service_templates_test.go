package render

import (
	"strings"
	"testing"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/templates"
)

func TestRenderTemplateStringMapExpandsEnvKeys(t *testing.T) {
	engine := templates.New(NoopResolver{})
	scope := templates.Scope{
		Project:   "primary",
		Stack:     "core",
		Partition: "dev",
		Service:   "api",
	}
	data := TemplateData{
		Project:   "primary",
		Stack:     "core",
		Partition: "dev",
		Service:   "api",
	}
	env := map[string]string{
		"CONFIG_{partition}": "value",
	}
	rendered, err := renderTemplateStringMap(engine, scope, data, "env", env)
	if err != nil {
		t.Fatalf("render env: %v", err)
	}
	if _, ok := rendered["CONFIG_dev"]; !ok {
		t.Fatalf("expected expanded env key, got %v", rendered)
	}
}

func TestRenderTemplateStringsExpandsConstraints(t *testing.T) {
	engine := templates.New(NoopResolver{})
	scope := templates.Scope{
		Project:   "primary",
		Stack:     "core",
		Partition: "dev",
		Service:   "api",
	}
	data := TemplateData{
		Project:   "primary",
		Stack:     "core",
		Partition: "dev",
		Service:   "api",
	}
	in := []string{"node.labels.env=={partition}"}
	rendered, err := renderTemplateStrings(engine, scope, data, "placement.constraints", in)
	if err != nil {
		t.Fatalf("render constraints: %v", err)
	}
	if len(rendered) != 1 || rendered[0] != "node.labels.env==dev" {
		t.Fatalf("expected expanded constraint, got %v", rendered)
	}
}

func TestRenderTemplateStringExpandsScopeTokens(t *testing.T) {
	engine := templates.New(NoopResolver{})
	scope := templates.Scope{
		Project:   "primary",
		Stack:     "core",
		Partition: "dev",
		Service:   "api",
	}
	data := TemplateData{
		Project:   "primary",
		Stack:     "core",
		Partition: "dev",
		Service:   "api",
	}
	rendered, err := RenderTemplateString(engine, scope, data, "image", "repo/{service}:{partition}")
	if err != nil {
		t.Fatalf("RenderTemplateString: %v", err)
	}
	if rendered != "repo/api:dev" {
		t.Fatalf("unexpected rendered string: %q", rendered)
	}
}

func TestRenderTemplatePoliciesAndRefs(t *testing.T) {
	engine := templates.New(NoopResolver{})
	scope := templates.Scope{
		Project:   "primary",
		Stack:     "core",
		Partition: "dev",
		Service:   "api",
	}
	data := TemplateData{
		Project:   "primary",
		Stack:     "core",
		Partition: "dev",
		Service:   "api",
	}

	condition := "{service}-failure"
	delay := "{partition}-delay"
	window := "{project}-window"
	restart, err := renderTemplateRestartPolicy(engine, scope, data, &config.RestartPolicy{
		Condition: &condition,
		Delay:     &delay,
		Window:    &window,
	})
	if err != nil {
		t.Fatalf("render restart policy: %v", err)
	}
	if restart == nil || *restart.Condition != "api-failure" || *restart.Delay != "dev-delay" || *restart.Window != "primary-window" {
		t.Fatalf("unexpected restart policy: %#v", restart)
	}

	order := "{stack}-first"
	monitor := "{service}-monitor"
	update, err := renderTemplateUpdatePolicy(engine, scope, data, &config.UpdatePolicy{
		Order:   &order,
		Monitor: &monitor,
	}, "update_config")
	if err != nil {
		t.Fatalf("render update policy: %v", err)
	}
	if update == nil || *update.Order != "core-first" || *update.Monitor != "api-monitor" {
		t.Fatalf("unexpected update policy: %#v", update)
	}

	configRefs, err := renderTemplateConfigRefs(engine, scope, data, []config.ConfigRef{{
		Name:   "app",
		Target: "/etc/{service}.yaml",
		UID:    "{partition}",
		GID:    "{stack}",
		Mode:   "0440",
	}})
	if err != nil {
		t.Fatalf("render config refs: %v", err)
	}
	if configRefs[0].Target != "/etc/api.yaml" || configRefs[0].UID != "dev" || configRefs[0].GID != "core" {
		t.Fatalf("unexpected config refs: %#v", configRefs)
	}

	secretRefs, err := renderTemplateSecretRefs(engine, scope, data, []config.SecretRef{{
		Name:   "token",
		Target: "/run/secrets/{service}",
		UID:    "{partition}",
		GID:    "{stack}",
		Mode:   "0400",
	}})
	if err != nil {
		t.Fatalf("render secret refs: %v", err)
	}
	if secretRefs[0].Target != "/run/secrets/api" || secretRefs[0].UID != "dev" || secretRefs[0].GID != "core" {
		t.Fatalf("unexpected secret refs: %#v", secretRefs)
	}

	volumeRefs, err := renderTemplateVolumeRefs(engine, scope, data, []config.VolumeRef{{
		Standard: "{service}-persist",
		Source:   "{project}_{stack}",
		Target:   "/data/{partition}",
		Subpath:  "{service}",
		Category: "{stack}",
	}})
	if err != nil {
		t.Fatalf("render volume refs: %v", err)
	}
	if volumeRefs[0].Standard != "api-persist" || volumeRefs[0].Source != "primary_core" || volumeRefs[0].Target != "/data/dev" {
		t.Fatalf("unexpected volume refs: %#v", volumeRefs)
	}

	ports, err := renderTemplatePorts(engine, scope, data, []config.Port{{
		Target:   8080,
		Protocol: "{service}",
		Mode:     "{partition}",
	}})
	if err != nil {
		t.Fatalf("render ports: %v", err)
	}
	if ports[0].Protocol != "api" || ports[0].Mode != "dev" {
		t.Fatalf("unexpected rendered ports: %#v", ports)
	}
}

func TestRenderServiceTemplates(t *testing.T) {
	engine := templates.New(NoopResolver{})
	scope := templates.Scope{
		Project:          "primary",
		Stack:            "core",
		Partition:        "dev",
		Service:          "api",
		NetworkEphemeral: "primary_dev_core_svc_api",
	}
	data := TemplateData{
		Project:   "primary",
		Stack:     "core",
		Partition: "dev",
		Service:   "api",
	}
	delay := "{partition}-delay"
	service := config.Service{
		Image:     "repo/{service}:{partition}",
		Workdir:   "/srv/{service}",
		Mode:      "{partition}",
		Command:   []string{"run", "{service}"},
		Args:      []string{"--env={partition}"},
		DependsOn: []string{"db", ""},
		Placement: config.Placement{Constraints: []string{"node.labels.env=={partition}"}},
		Networks:  []string{"{project}_{stack}"},
		Env:       map[string]string{"APP_{partition}": "{service}", "EPHEMERAL": "{network_ephemeral}"},
		Ports:     []config.Port{{Target: 8080, Protocol: "{service}", Mode: "{partition}"}},
		Configs:   []config.ConfigRef{{Name: "cfg", Target: "/etc/{service}.yaml"}},
		Secrets:   []config.SecretRef{{Name: "sec", Target: "/run/secrets/{service}"}},
		Volumes:   []config.VolumeRef{{Source: "{project}_{stack}", Target: "/data/{partition}"}},
		RestartPolicy: &config.RestartPolicy{
			Delay: &delay,
		},
	}

	rendered, err := RenderServiceTemplates(engine, scope, data, service)
	if err != nil {
		t.Fatalf("RenderServiceTemplates: %v", err)
	}
	if rendered.Image != "repo/api:dev" || rendered.Workdir != "/srv/api" || rendered.Mode != "dev" {
		t.Fatalf("unexpected rendered service basics: %#v", rendered)
	}
	if !strings.Contains(rendered.Command[1], "api") || !strings.Contains(rendered.Args[0], "dev") {
		t.Fatalf("unexpected rendered command/args: %#v %#v", rendered.Command, rendered.Args)
	}
	if rendered.Env["APP_dev"] != "api" {
		t.Fatalf("unexpected rendered env: %#v", rendered.Env)
	}
	if rendered.Env["EPHEMERAL"] != "primary_dev_core_svc_api" {
		t.Fatalf("unexpected rendered ephemeral env: %#v", rendered.Env)
	}
	if rendered.Configs[0].Target != "/etc/api.yaml" || rendered.Secrets[0].Target != "/run/secrets/api" || rendered.Volumes[0].Target != "/data/dev" {
		t.Fatalf("unexpected rendered refs: configs=%#v secrets=%#v volumes=%#v", rendered.Configs, rendered.Secrets, rendered.Volumes)
	}
	if rendered.RestartPolicy == nil || *rendered.RestartPolicy.Delay != "dev-delay" {
		t.Fatalf("unexpected rendered restart policy: %#v", rendered.RestartPolicy)
	}
}
