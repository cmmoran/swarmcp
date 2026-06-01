package apply

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cmmoran/swarmcp/internal/swarm"
)

func TestOmitReplayableSecretPayloadsStripsDirectSecretValue(t *testing.T) {
	plan := Plan{
		CreateSecrets: []swarm.SecretSpec{{
			Name:    "api_token_abcd",
			Data:    []byte("secret"),
			HasData: true,
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
	if plan.CreateSecrets[0].HasData {
		t.Fatalf("expected replayable secret payload marker to be cleared")
	}
}

func TestOmitReplayableSecretPayloadsStripsDirectFileSecretValue(t *testing.T) {
	plan := Plan{
		CreateSecrets: []swarm.SecretSpec{{
			Name:    "api_token_abcd",
			Data:    []byte("secret"),
			HasData: true,
		}},
	}
	sources := []PlanSecretSource{{
		SecretName: "api_token_abcd",
		Dependencies: []PlanSecretDependency{{
			Name:     "api_token",
			Hash:     secretValueHash("secret"),
			Provider: "file",
			Key:      "api_token",
		}},
	}}

	OmitReplayableSecretPayloads(&plan, sources)
	if plan.CreateSecrets[0].Data != nil {
		t.Fatalf("expected replayable file secret payload to be omitted")
	}
	if plan.CreateSecrets[0].HasData {
		t.Fatalf("expected replayable file secret payload marker to be cleared")
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
	if !planFile.Plan.CreateSecrets[0].HasData {
		t.Fatalf("expected resolved payload marker")
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

func TestResolvePlanSecretPayloadsReadsMatchingSecretsFile(t *testing.T) {
	dir := t.TempDir()
	secretsPath := filepath.Join(dir, "secrets.yaml")
	if err := os.WriteFile(secretsPath, []byte("values:\n  api_token: secret\n"), 0o600); err != nil {
		t.Fatalf("write secrets: %v", err)
	}
	inputs, err := FileInputs("secrets", []string{secretsPath})
	if err != nil {
		t.Fatalf("FileInputs: %v", err)
	}
	planFile := PlanFile{
		APIVersion: PlanFileAPIVersion,
		Secrets:    PlanSecrets{Mode: PlanSecretModeReference},
		Inputs:     inputs,
		SecretSources: []PlanSecretSource{{
			SecretName: "api_token_abcd",
			Dependencies: []PlanSecretDependency{{
				Name:     "api_token",
				Hash:     secretValueHash("secret"),
				Provider: "file",
				Key:      "api_token",
			}},
		}},
		Plan: Plan{
			CreateSecrets: []swarm.SecretSpec{{Name: "api_token_abcd"}},
		},
	}

	if err := ValidatePlanFile(planFile); err != nil {
		t.Fatalf("ValidatePlanFile: %v", err)
	}
	if err := ResolvePlanSecretPayloads(context.Background(), &planFile); err != nil {
		t.Fatalf("ResolvePlanSecretPayloads: %v", err)
	}
	if got := string(planFile.Plan.CreateSecrets[0].Data); got != "secret" {
		t.Fatalf("unexpected resolved payload: %q", got)
	}
	if !planFile.Plan.CreateSecrets[0].HasData {
		t.Fatalf("expected resolved payload marker")
	}
}

func TestResolvePlanSecretPayloadsRejectsChangedSecretsFile(t *testing.T) {
	dir := t.TempDir()
	secretsPath := filepath.Join(dir, "secrets.yaml")
	if err := os.WriteFile(secretsPath, []byte("values:\n  api_token: secret\n"), 0o600); err != nil {
		t.Fatalf("write secrets: %v", err)
	}
	inputs, err := FileInputs("secrets", []string{secretsPath})
	if err != nil {
		t.Fatalf("FileInputs: %v", err)
	}
	if err := os.WriteFile(secretsPath, []byte("values:\n  api_token: changed\n"), 0o600); err != nil {
		t.Fatalf("rewrite secrets: %v", err)
	}
	planFile := PlanFile{
		APIVersion: PlanFileAPIVersion,
		Secrets:    PlanSecrets{Mode: PlanSecretModeReference},
		Inputs:     inputs,
		SecretSources: []PlanSecretSource{{
			SecretName: "api_token_abcd",
			Dependencies: []PlanSecretDependency{{
				Name:     "api_token",
				Hash:     secretValueHash("secret"),
				Provider: "file",
				Key:      "api_token",
			}},
		}},
		Plan: Plan{
			CreateSecrets: []swarm.SecretSpec{{Name: "api_token_abcd"}},
		},
	}

	if err := ResolvePlanSecretPayloads(context.Background(), &planFile); err == nil {
		t.Fatalf("expected changed secrets file to be rejected")
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

func TestValidatePlanFileRejectsFileReferenceWithoutSecretsInput(t *testing.T) {
	planFile := PlanFile{
		APIVersion: PlanFileAPIVersion,
		Secrets:    PlanSecrets{Mode: PlanSecretModeReference},
		SecretSources: []PlanSecretSource{{
			SecretName: "api_token_abcd",
			Dependencies: []PlanSecretDependency{{
				Name:     "api_token",
				Hash:     secretValueHash("secret"),
				Provider: "file",
				Key:      "api_token",
			}},
		}},
		Plan: Plan{
			CreateSecrets: []swarm.SecretSpec{{Name: "api_token_abcd"}},
		},
	}

	if err := ValidatePlanFile(planFile); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestValidatePlanFileRejectsReferenceModeWithPayloads(t *testing.T) {
	planFile := PlanFile{
		APIVersion: PlanFileAPIVersion,
		Secrets:    PlanSecrets{Mode: PlanSecretModeReference},
		Plan: Plan{
			CreateSecrets: []swarm.SecretSpec{{
				Name:    "api_token_abcd",
				Data:    []byte("secret"),
				HasData: true,
			}},
		},
	}

	if err := ValidatePlanFile(planFile); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestPayloadModeAllowsEmptySecretPayload(t *testing.T) {
	planFile := PlanFile{
		APIVersion: PlanFileAPIVersion,
		Secrets:    PlanSecrets{Mode: PlanSecretModePayload},
		Plan: Plan{
			CreateSecrets: []swarm.SecretSpec{{
				Name:    "empty_secret",
				HasData: true,
			}},
		},
	}

	if err := ValidatePlanFile(planFile); err != nil {
		t.Fatalf("ValidatePlanFile: %v", err)
	}
	if err := ResolvePlanSecretPayloads(context.Background(), &planFile); err != nil {
		t.Fatalf("ResolvePlanSecretPayloads: %v", err)
	}
}
