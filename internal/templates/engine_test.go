package templates

import (
	"fmt"
	"strings"
	"testing"
)

type engineStubResolver struct {
	values map[string]any
}

func (s engineStubResolver) ConfigValue(name string) (any, error)  { return s.values[name], nil }
func (s engineStubResolver) ConfigRef(name string) (string, error) { return "", nil }
func (s engineStubResolver) ConfigRefs(pattern string) ([]string, error) {
	return nil, nil
}
func (s engineStubResolver) SecretValue(name string) (string, error) { return "", nil }
func (s engineStubResolver) SecretRef(name string) (string, error)   { return "", nil }
func (s engineStubResolver) SecretRefs(pattern string) ([]string, error) {
	return nil, nil
}
func (s engineStubResolver) RuntimeValue(args ...string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	return args[0], nil
}

func TestConfigValueIndexParsesStringList(t *testing.T) {
	engine := New(engineStubResolver{
		values: map[string]any{
			"domain": `["example.com","example.net"]`,
		},
	})
	rendered, err := engine.Render("test", `{{ config_value_index "domain" 0 }}`, nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.TrimSpace(rendered) != "example.com" {
		t.Fatalf("unexpected render: %q", rendered)
	}
}

func TestConfigValueIndexStringIndex(t *testing.T) {
	engine := New(engineStubResolver{
		values: map[string]any{
			"domain": `["example.com","example.net"]`,
		},
	})
	rendered, err := engine.Render("test", `{{ config_value_index "domain" "0" }}`, nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.TrimSpace(rendered) != "example.com" {
		t.Fatalf("unexpected render: %q", rendered)
	}
}

func TestConfigValueGetParsesStringMap(t *testing.T) {
	engine := New(engineStubResolver{
		values: map[string]any{
			"labels": `{"env":"prod"}`,
		},
	})
	rendered, err := engine.Render("test", `{{ config_value_get "labels" "env" }}`, nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.TrimSpace(rendered) != "prod" {
		t.Fatalf("unexpected render: %q", rendered)
	}
}

func TestConfigValueIndexMissingWraps(t *testing.T) {
	engine := New(errResolver{})
	_, err := engine.Render("test", `{{ config_value_index "missing" 0 }}`, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "config_value_index \"missing\":") {
		t.Fatalf("unexpected error: %v", err)
	}
}

type errResolver struct{}

func (errResolver) ConfigValue(name string) (any, error) {
	return "", fmt.Errorf("config %q not found", name)
}
func (errResolver) ConfigRef(name string) (string, error) { return "", nil }
func (errResolver) ConfigRefs(pattern string) ([]string, error) {
	return nil, nil
}
func (errResolver) SecretValue(name string) (string, error) { return "", nil }
func (errResolver) SecretRef(name string) (string, error)   { return "", nil }
func (errResolver) SecretRefs(pattern string) ([]string, error) {
	return nil, nil
}
func (errResolver) RuntimeValue(args ...string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	return args[0], nil
}
