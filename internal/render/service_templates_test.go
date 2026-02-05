package render

import (
	"testing"

	"github.com/cmmoran/swarmcp/internal/templates"
)

func TestRenderTemplateStringMapExpandsEnvKeys(t *testing.T) {
	engine := templates.New(NoopResolver{})
	data := TemplateData{
		Project:   "primary",
		Stack:     "core",
		Partition: "dev",
		Service:   "api",
	}
	env := map[string]string{
		"CONFIG_{partition}": "value",
	}
	rendered, err := renderTemplateStringMap(engine, data, "env", env)
	if err != nil {
		t.Fatalf("render env: %v", err)
	}
	if _, ok := rendered["CONFIG_dev"]; !ok {
		t.Fatalf("expected expanded env key, got %v", rendered)
	}
}

func TestRenderTemplateStringsExpandsConstraints(t *testing.T) {
	engine := templates.New(NoopResolver{})
	data := TemplateData{
		Project:   "primary",
		Stack:     "core",
		Partition: "dev",
		Service:   "api",
	}
	in := []string{"node.labels.env=={partition}"}
	rendered, err := renderTemplateStrings(engine, data, "placement.constraints", in)
	if err != nil {
		t.Fatalf("render constraints: %v", err)
	}
	if len(rendered) != 1 || rendered[0] != "node.labels.env==dev" {
		t.Fatalf("expected expanded constraint, got %v", rendered)
	}
}
