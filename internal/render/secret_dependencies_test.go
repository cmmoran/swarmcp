package render

import (
	"testing"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/secrets"
)

func TestRenderProjectRecordsSecretValueDependencies(t *testing.T) {
	cfg := &config.Config{
		Project: config.Project{
			Name:       "demo",
			Deployment: "prod",
			Secrets: map[string]config.SecretDef{
				"api_token": {
					Source: `inline:{{ secret_value "api_token" }}`,
				},
			},
		},
	}
	store := &secrets.Store{
		Values: map[string]string{
			"api_token": "super-secret",
		},
	}

	summary, err := RenderProject(cfg, store, nil, nil, nil, false, true)
	if err != nil {
		t.Fatalf("RenderProject: %v", err)
	}
	if len(summary.Defs) != 1 {
		t.Fatalf("expected one rendered def, got %#v", summary.Defs)
	}
	def := summary.Defs[0]
	if def.Content != "super-secret" {
		t.Fatalf("unexpected rendered content: %q", def.Content)
	}
	if len(def.SecretDependencies) != 1 {
		t.Fatalf("expected one secret dependency, got %#v", def.SecretDependencies)
	}
	dep := def.SecretDependencies[0]
	if dep.Name != "api_token" {
		t.Fatalf("unexpected dependency name: %q", dep.Name)
	}
	if dep.Metadata.Provider != "file" || dep.Metadata.Key != "api_token" {
		t.Fatalf("unexpected dependency metadata: %#v", dep.Metadata)
	}
	if dep.Hash != contentHash("super-secret") {
		t.Fatalf("unexpected dependency hash: %q", dep.Hash)
	}
	if dep.Scope.Project != "demo" || dep.Scope.Deployment != "prod" {
		t.Fatalf("unexpected dependency scope: %#v", dep.Scope)
	}
}
