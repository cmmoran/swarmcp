package apply

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cmmoran/swarmcp/internal/swarm"
)

func TestOmitReplayableSecretPayloadsStripsDirectSecretValue(t *testing.T) {
	plan := Plan{
		CreateSecrets: []swarm.SecretSpec{{
			Name: "api_token_abcd",
			Data: []byte("secret"),
		}},
	}
	sources := []PlanSecretSource{{
		SecretName: "api_token_abcd",
		Dependencies: []PlanSecretDependency{{
			Name:     "api_token",
			Hash:     secretValueHash("secret"),
			Provider: "vault",
			Addr:     "http://vault.test",
			Mount:    "kv",
			Path:     "demo/prod/core",
			Key:      "api_token",
		}},
	}}

	OmitReplayableSecretPayloads(&plan, sources)
	if plan.CreateSecrets[0].Data != nil {
		t.Fatalf("expected replayable secret payload to be omitted")
	}
}

func TestResolvePlanSecretPayloadsReadsPinnedVaultVersion(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/kv/data/demo/prod/core" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		if token := r.Header.Get("X-Vault-Token"); token != "test-token" {
			t.Fatalf("unexpected token: %q", token)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"data": map[string]any{
					"api_token": "secret",
				},
				"metadata": map[string]any{
					"version": 7,
				},
			},
		})
	}))
	defer server.Close()
	t.Setenv("VAULT_TOKEN", "test-token")
	version := 7
	planFile := PlanFile{
		APIVersion: PlanFileAPIVersion,
		SecretSources: []PlanSecretSource{{
			SecretName: "api_token_abcd",
			Dependencies: []PlanSecretDependency{{
				Name:     "api_token",
				Hash:     secretValueHash("secret"),
				Provider: "vault",
				Addr:     server.URL,
				Mount:    "kv",
				Path:     "demo/prod/core",
				Key:      "api_token",
				Version:  &version,
			}},
		}},
		Plan: Plan{
			CreateSecrets: []swarm.SecretSpec{{
				Name: "api_token_abcd",
			}},
		},
	}

	if err := ResolvePlanSecretPayloads(context.Background(), &planFile); err != nil {
		t.Fatalf("ResolvePlanSecretPayloads: %v", err)
	}
	if got := string(planFile.Plan.CreateSecrets[0].Data); got != "secret" {
		t.Fatalf("unexpected resolved payload: %q", got)
	}
	if gotQuery != "version=7" {
		t.Fatalf("expected pinned version query, got %q", gotQuery)
	}
}

func TestResolvePlanSecretPayloadsRejectsHashMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"data": map[string]any{
					"api_token": "different",
				},
				"metadata": map[string]any{
					"version": 7,
				},
			},
		})
	}))
	defer server.Close()
	t.Setenv("VAULT_TOKEN", "test-token")
	planFile := PlanFile{
		APIVersion: PlanFileAPIVersion,
		SecretSources: []PlanSecretSource{{
			SecretName: "api_token_abcd",
			Dependencies: []PlanSecretDependency{{
				Name:     "api_token",
				Hash:     secretValueHash("secret"),
				Provider: "vault",
				Addr:     server.URL,
				Mount:    "kv",
				Path:     "demo/prod/core",
				Key:      "api_token",
			}},
		}},
		Plan: Plan{
			CreateSecrets: []swarm.SecretSpec{{Name: "api_token_abcd"}},
		},
	}

	if err := ResolvePlanSecretPayloads(context.Background(), &planFile); err == nil {
		t.Fatalf("expected hash mismatch")
	}
}

func TestSetPlanSecretModeDetectsReferencePlans(t *testing.T) {
	planFile := PlanFile{
		APIVersion: PlanFileAPIVersion,
		SecretSources: []PlanSecretSource{{
			SecretName: "api_token_abcd",
			Dependencies: []PlanSecretDependency{{
				Name:     "api_token",
				Hash:     secretValueHash("secret"),
				Provider: "vault",
				Addr:     "http://vault.test",
				Mount:    "kv",
				Path:     "demo/prod/core",
				Key:      "api_token",
			}},
		}},
		Plan: Plan{
			CreateSecrets: []swarm.SecretSpec{{Name: "api_token_abcd"}},
		},
	}

	SetPlanSecretMode(&planFile)
	if planFile.Secrets.Mode != PlanSecretModeReference {
		t.Fatalf("unexpected secret mode: %q", planFile.Secrets.Mode)
	}
	if err := ValidatePlanFile(planFile); err != nil {
		t.Fatalf("ValidatePlanFile: %v", err)
	}
}

func TestValidatePlanFileRejectsReferenceModeWithPayloads(t *testing.T) {
	planFile := PlanFile{
		APIVersion: PlanFileAPIVersion,
		Secrets:    PlanSecrets{Mode: PlanSecretModeReference},
		Plan: Plan{
			CreateSecrets: []swarm.SecretSpec{{
				Name: "api_token_abcd",
				Data: []byte("secret"),
			}},
		},
	}

	if err := ValidatePlanFile(planFile); err == nil {
		t.Fatalf("expected validation error")
	}
}
