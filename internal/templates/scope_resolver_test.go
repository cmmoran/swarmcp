package templates

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cmmoran/swarmcp/internal/config"
)

func TestScopeResolverConfigValue(t *testing.T) {
	dir := t.TempDir()
	templatePath := filepath.Join(dir, "app.tmpl")
	if err := os.WriteFile(templatePath, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}

	cfg := &config.Config{
		Project: config.Project{
			Name: "primary",
			Configs: map[string]config.ConfigDef{
				"app": {Source: templatePath},
			},
		},
	}

	resolver := NewScopeResolver(cfg, Scope{Project: "primary"}, false, false, nil, nil, nil)
	val, err := resolver.ConfigValue("app")
	if err != nil {
		t.Fatalf("ConfigValue: %v", err)
	}
	valStr, ok := val.(string)
	if !ok {
		t.Fatalf("unexpected value type: %T", val)
	}
	if valStr != "hello" {
		t.Fatalf("unexpected value: %q", valStr)
	}
}

func TestScopeResolverConfigValueTokenPath(t *testing.T) {
	dir := t.TempDir()
	partitionDir := filepath.Join(dir, "dev")
	if err := os.MkdirAll(partitionDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	templatePath := filepath.Join(dir, "{partition}", "app.tmpl")
	if err := os.WriteFile(filepath.Join(partitionDir, "app.tmpl"), []byte("scoped"), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}

	cfg := &config.Config{
		Project: config.Project{
			Name: "primary",
			Configs: map[string]config.ConfigDef{
				"app": {Source: templatePath},
			},
		},
	}

	resolver := NewScopeResolver(cfg, Scope{Project: "primary", Partition: "dev"}, false, false, nil, nil, nil)
	val, err := resolver.ConfigValue("app")
	if err != nil {
		t.Fatalf("ConfigValue: %v", err)
	}
	valStr, ok := val.(string)
	if !ok {
		t.Fatalf("unexpected value type: %T", val)
	}
	if valStr != "scoped" {
		t.Fatalf("unexpected value: %q", valStr)
	}
}

func TestScopeResolverConfigValueFallbackToValues(t *testing.T) {
	cfg := &config.Config{
		Project: config.Project{
			Name: "primary",
		},
	}
	values := map[string]any{
		"global": map[string]any{
			"domain": "example.com",
		},
	}

	resolver := NewScopeResolver(cfg, Scope{Project: "primary"}, false, true, nil, nil, values)
	val, err := resolver.ConfigValue("domain")
	if err != nil {
		t.Fatalf("ConfigValue: %v", err)
	}
	valStr, ok := val.(string)
	if !ok {
		t.Fatalf("unexpected value type: %T", val)
	}
	if valStr != "example.com" {
		t.Fatalf("unexpected value: %q", valStr)
	}
}

func TestScopeResolverConfigValueFallbackToValuesList(t *testing.T) {
	cfg := &config.Config{
		Project: config.Project{
			Name: "primary",
		},
	}
	values := map[string]any{
		"global": map[string]any{
			"ip_whitelist": []any{"10.0.0.1/32", "10.0.0.2/32"},
		},
	}

	resolver := NewScopeResolver(cfg, Scope{Project: "primary"}, false, true, nil, nil, values)
	val, err := resolver.ConfigValue("ip_whitelist")
	if err != nil {
		t.Fatalf("ConfigValue: %v", err)
	}
	list, ok := val.([]any)
	if !ok {
		t.Fatalf("unexpected value type: %T", val)
	}
	if len(list) != 2 {
		t.Fatalf("unexpected list length: %d", len(list))
	}
}

func TestScopeResolverSecretValue(t *testing.T) {
	cfg := &config.Config{
		Project: config.Project{
			Name: "primary",
			Secrets: map[string]config.SecretDef{
				"token": {Source: "ignored"},
			},
		},
	}

	resolver := NewScopeResolver(cfg, Scope{Project: "primary"}, true, false, nil, func(scope Scope, name string) (string, error) {
		if name == "token" {
			return "secret", nil
		}
		return "", nil
	}, nil)
	val, err := resolver.SecretValue("token")
	if err != nil {
		t.Fatalf("SecretValue: %v", err)
	}
	if val != "secret" {
		t.Fatalf("unexpected value: %q", val)
	}
}

func TestScopeResolverSecretValueUsesResolvedScope(t *testing.T) {
	cfg := &config.Config{
		Project: config.Project{
			Name: "primary",
			Secrets: map[string]config.SecretDef{
				"token": {Source: "ignored"},
			},
		},
		Stacks: map[string]config.Stack{
			"app": {
				Services: map[string]config.Service{
					"api": {},
				},
			},
		},
	}

	var gotScope Scope
	var gotName string
	resolver := NewScopeResolver(cfg, Scope{Project: "primary", Stack: "app", Service: "api"}, true, false, nil, func(scope Scope, name string) (string, error) {
		gotScope = scope
		gotName = name
		return "secret", nil
	}, nil)
	if _, err := resolver.SecretValue("token"); err != nil {
		t.Fatalf("SecretValue: %v", err)
	}
	if gotName != "token" {
		t.Fatalf("unexpected name: %q", gotName)
	}
	if gotScope.Stack != "" || gotScope.Service != "" || gotScope.Project != "primary" {
		t.Fatalf("unexpected scope: %#v", gotScope)
	}
}

func TestScopeResolverSecretValueMissingUsesCallerScope(t *testing.T) {
	cfg := &config.Config{
		Project: config.Project{
			Name: "primary",
		},
	}

	var gotScope Scope
	var gotName string
	resolver := NewScopeResolver(cfg, Scope{Project: "primary", Stack: "app", Service: "api"}, true, false, nil, func(scope Scope, name string) (string, error) {
		gotScope = scope
		gotName = name
		return "secret", nil
	}, nil)
	if _, err := resolver.SecretValue("token"); err != nil {
		t.Fatalf("SecretValue: %v", err)
	}
	if gotName != "token" {
		t.Fatalf("unexpected name: %q", gotName)
	}
	if gotScope.Stack != "app" || gotScope.Service != "api" || gotScope.Project != "primary" {
		t.Fatalf("unexpected scope: %#v", gotScope)
	}
}

func TestScopeResolverSecretValueDenied(t *testing.T) {
	cfg := &config.Config{
		Project: config.Project{
			Name: "primary",
			Secrets: map[string]config.SecretDef{
				"token": {Source: "ignored"},
			},
		},
	}

	resolver := NewScopeResolver(cfg, Scope{Project: "primary"}, false, false, nil, nil, nil)
	if _, err := resolver.SecretValue("token"); err == nil {
		t.Fatalf("expected error for secret_value in config scope")
	}
}

func TestScopeResolverServiceOverridesStack(t *testing.T) {
	dir := t.TempDir()
	stackPath := filepath.Join(dir, "stack.tmpl")
	servicePath := filepath.Join(dir, "service.tmpl")
	if err := os.WriteFile(stackPath, []byte("stack"), 0o600); err != nil {
		t.Fatalf("write stack template: %v", err)
	}
	if err := os.WriteFile(servicePath, []byte("service"), 0o600); err != nil {
		t.Fatalf("write service template: %v", err)
	}

	cfg := &config.Config{
		Project: config.Project{Name: "primary"},
		Stacks: map[string]config.Stack{
			"app": {
				Configs: config.ConfigDefsOrRefs{Defs: map[string]config.ConfigDef{
					"shared": {Source: stackPath},
				}},
				Services: map[string]config.Service{
					"api": {
						Configs: []config.ConfigRef{
							{Name: "shared", Source: servicePath},
						},
					},
				},
			},
		},
	}

	resolver := NewScopeResolver(cfg, Scope{Project: "primary", Stack: "app", Service: "api"}, false, false, nil, nil, nil)
	val, err := resolver.ConfigValue("shared")
	if err != nil {
		t.Fatalf("ConfigValue: %v", err)
	}
	valStr, ok := val.(string)
	if !ok {
		t.Fatalf("unexpected value type: %T", val)
	}
	if valStr != "service" {
		t.Fatalf("unexpected value: %q", valStr)
	}
}

func TestScopeResolverServiceSecretDefaultSource(t *testing.T) {
	cfg := &config.Config{
		Project: config.Project{Name: "primary"},
		Stacks: map[string]config.Stack{
			"app": {
				Services: map[string]config.Service{
					"api": {
						Secrets: []config.SecretRef{
							{Name: "token"},
						},
					},
				},
			},
		},
	}

	resolver := NewScopeResolver(cfg, Scope{Project: "primary", Stack: "app", Service: "api"}, true, false, nil, nil, nil)
	def, scope, ok := resolver.ResolveSecretWithScope("token")
	if !ok {
		t.Fatalf("expected secret resolution")
	}
	if scope.Service != "api" {
		t.Fatalf("unexpected scope: %#v", scope)
	}
	if def.Source != config.DefaultSecretSource("token", "") {
		t.Fatalf("unexpected source: %q", def.Source)
	}
}

func TestScopeResolverServiceSecretDefaultSourceSkippedWhenHigherScopeExists(t *testing.T) {
	cfg := &config.Config{
		Project: config.Project{Name: "primary"},
		Stacks: map[string]config.Stack{
			"app": {
				Secrets: config.SecretDefsOrRefs{Defs: map[string]config.SecretDef{
					"token": {Source: "stack.tmpl"},
				}},
				Services: map[string]config.Service{
					"api": {
						Secrets: []config.SecretRef{
							{Name: "token"},
						},
					},
				},
			},
		},
	}

	resolver := NewScopeResolver(cfg, Scope{Project: "primary", Stack: "app", Service: "api"}, true, false, nil, nil, nil)
	def, scope, ok := resolver.ResolveSecretWithScope("token")
	if !ok {
		t.Fatalf("expected secret resolution")
	}
	if scope.Stack != "app" || scope.Service != "" {
		t.Fatalf("unexpected scope: %#v", scope)
	}
	if def.Source != "stack.tmpl" {
		t.Fatalf("unexpected source: %q", def.Source)
	}
}
