package apply

import (
	"testing"

	"github.com/cmmoran/swarmcp/internal/render"
	"github.com/cmmoran/swarmcp/internal/secrets"
	"github.com/cmmoran/swarmcp/internal/swarm"
	"github.com/cmmoran/swarmcp/internal/templates"
)

func TestSecretSourcesForPlanIncludesCreatedSecretDependencies(t *testing.T) {
	version := 7
	secretName, hash := render.PhysicalName("api_token", "rendered-secret")
	desired := DesiredState{
		Defs: []render.RenderedDef{{
			Kind:    "secret",
			Name:    "api_token",
			Content: "rendered-secret",
			ScopeID: templates.Scope{Project: "demo", Deployment: "prod", Stack: "core"},
			SecretDependencies: []render.SecretDependency{{
				Name:  "api_token",
				Scope: templates.Scope{Project: "demo", Deployment: "prod", Stack: "core"},
				Hash:  hash,
				Metadata: secrets.SecretMetadata{
					Provider: "vault",
					Mount:    "kv",
					Path:     "demo/prod/core",
					Key:      "api_token",
					Version:  &version,
				},
			}},
		}},
	}
	plan := Plan{
		CreateSecrets: []swarm.SecretSpec{{Name: secretName}},
	}

	sources := SecretSourcesForPlan(desired, plan)
	if len(sources) != 1 {
		t.Fatalf("expected one secret source, got %#v", sources)
	}
	if sources[0].SecretName != secretName || sources[0].LogicalName != "api_token" {
		t.Fatalf("unexpected secret source: %#v", sources[0])
	}
	dep := sources[0].Dependencies[0]
	if dep.Provider != "vault" || dep.Mount != "kv" || dep.Path != "demo/prod/core" || dep.Key != "api_token" {
		t.Fatalf("unexpected dependency metadata: %#v", dep)
	}
	if dep.Version == nil || *dep.Version != 7 {
		t.Fatalf("unexpected dependency version: %#v", dep.Version)
	}
}
