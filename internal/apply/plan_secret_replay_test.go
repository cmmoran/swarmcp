package apply

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/cmmoran/swarmcp/internal/config"
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
			Assumptions: PlanAssumptions{
				AbsentSecrets: []string{"api_token_abcd"},
			},
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
			Assumptions: PlanAssumptions{
				AbsentSecrets: []string{"api_token_abcd"},
			},
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
			Assumptions: PlanAssumptions{
				AbsentSecrets: []string{"api_token_abcd"},
			},
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
			Assumptions: PlanAssumptions{
				AbsentSecrets: []string{"api_token_abcd"},
			},
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

func TestValidatePlanFileRejectsPayloadModeWithoutPayload(t *testing.T) {
	planFile := PlanFile{
		APIVersion: PlanFileAPIVersion,
		Secrets:    PlanSecrets{Mode: PlanSecretModePayload},
		Plan: Plan{
			CreateSecrets: []swarm.SecretSpec{{
				Name: "api_token_abcd",
			}},
		},
	}

	if err := ValidatePlanFile(planFile); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestValidatePlanFileRejectsMutatingPlanWithoutAssumptions(t *testing.T) {
	planFile := PlanFile{
		APIVersion: PlanFileAPIVersion,
		Secrets:    PlanSecrets{Mode: PlanSecretModePayload},
		Plan: Plan{
			CreateConfigs: []swarm.ConfigSpec{{
				Name: "config_v1",
				Data: []byte("config"),
			}},
		},
	}

	if err := ValidatePlanFile(planFile); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestValidatePlanFileRejectsIncompleteGitSourceInput(t *testing.T) {
	planFile := PlanFile{
		APIVersion: PlanFileAPIVersion,
		Secrets:    PlanSecrets{Mode: PlanSecretModePayload},
		SourceInputs: []PlanSourceInput{{
			Kind: "git",
			URL:  "ssh://git@example.com/repo.git",
		}},
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
			Assumptions: PlanAssumptions{
				AbsentSecrets: []string{"empty_secret"},
			},
		},
	}

	if err := ValidatePlanFile(planFile); err != nil {
		t.Fatalf("ValidatePlanFile: %v", err)
	}
	if err := ResolvePlanSecretPayloads(context.Background(), &planFile); err != nil {
		t.Fatalf("ResolvePlanSecretPayloads: %v", err)
	}
}

func TestResolvePlanSecretPayloadsReplaysComposedRecipe(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "secret.tmpl"), []byte("user={{ secret_value \"username\" }}\npass={{ secret_value \"password\" }}\n"), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}
	runPlanSecretGit(t, repo, "init", "-b", "main")
	runPlanSecretGit(t, repo, "config", "user.email", "test@example.com")
	runPlanSecretGit(t, repo, "config", "user.name", "Test User")
	runPlanSecretGit(t, repo, "add", ".")
	runPlanSecretGit(t, repo, "commit", "-m", "add secret template")
	url := "file://" + filepath.ToSlash(repo)
	cacheDir := filepath.Join(dir, ".swarmcp", "sources")
	cacheRepo := filepath.Join(cacheDir, "repos", planSecretTestSourceHashKey(url))
	if err := os.MkdirAll(filepath.Dir(cacheRepo), 0o755); err != nil {
		t.Fatalf("mkdir cache repo parent: %v", err)
	}
	runPlanSecretGit(t, dir, "clone", "--bare", repo, cacheRepo)
	source, err := config.ResolveSourceRef(config.SourceRef{
		URL:  url,
		Ref:  "main",
		Path: "secret.tmpl",
	}, "", config.LoadOptions{CacheDir: cacheDir})
	if err != nil {
		t.Fatalf("ResolveSourceRef: %v", err)
	}
	meta, found, err := config.ReadSourceMetadata(url, "main", "secret.tmpl", config.LoadOptions{CacheDir: cacheDir})
	if err != nil {
		t.Fatalf("ReadSourceMetadata: %v", err)
	}
	if !found {
		t.Fatalf("expected source metadata")
	}
	projectPath := filepath.Join(dir, "project.yaml")
	if err := os.WriteFile(projectPath, []byte("project:\n  name: demo\n"), 0o600); err != nil {
		t.Fatalf("write project: %v", err)
	}
	secretsPath := filepath.Join(dir, "secrets.yaml")
	if err := os.WriteFile(secretsPath, []byte("values:\n  username: alice\n  password: secret\n"), 0o600); err != nil {
		t.Fatalf("write secrets: %v", err)
	}
	inputs, err := FileInputs("project", []string{projectPath})
	if err != nil {
		t.Fatalf("project FileInputs: %v", err)
	}
	secretInputs, err := FileInputs("secrets", []string{secretsPath})
	if err != nil {
		t.Fatalf("secrets FileInputs: %v", err)
	}
	inputs = append(inputs, secretInputs...)
	rendered := "user=alice\npass=secret\n"
	planFile := PlanFile{
		APIVersion: PlanFileAPIVersion,
		Secrets:    PlanSecrets{Mode: PlanSecretModePayload},
		Inputs:     inputs,
		SourceInputs: []PlanSourceInput{{
			Kind:    "git",
			URL:     meta.URL,
			Ref:     meta.Ref,
			Commit:  meta.Commit,
			Path:    meta.Path,
			Subtree: meta.Subtree,
		}},
		SecretSources: []PlanSecretSource{{
			SecretName:  "app_secret_abcd",
			LogicalName: "app_secret",
			Recipe: &PlanSecretRecipe{
				Source:       source,
				RenderedHash: secretValueHash(rendered),
			},
			Dependencies: []PlanSecretDependency{
				{Name: "username", Hash: secretValueHash("alice"), Provider: "file", Key: "username"},
				{Name: "password", Hash: secretValueHash("secret"), Provider: "file", Key: "password"},
			},
		}},
		Plan: Plan{
			CreateSecrets: []swarm.SecretSpec{{
				Name:    "app_secret_abcd",
				Data:    []byte(rendered),
				HasData: true,
			}},
			Assumptions: PlanAssumptions{
				AbsentSecrets: []string{"app_secret_abcd"},
			},
		},
	}

	OmitReplayableSecretPayloadsFromPlan(context.Background(), &planFile)
	if PlanHasSecretPayloads(planFile.Plan) {
		t.Fatalf("expected recipe payload to be omitted")
	}
	SetPlanSecretMode(&planFile)
	if err := ValidatePlanFile(planFile); err != nil {
		t.Fatalf("ValidatePlanFile: %v", err)
	}
	if err := ResolvePlanSecretPayloads(context.Background(), &planFile); err != nil {
		t.Fatalf("ResolvePlanSecretPayloads: %v", err)
	}
	if got := string(planFile.Plan.CreateSecrets[0].Data); got != rendered {
		t.Fatalf("unexpected recipe payload: %q", got)
	}
}

func TestValidatePlanFileRejectsRecipeWithoutGitSource(t *testing.T) {
	planFile := PlanFile{
		APIVersion: PlanFileAPIVersion,
		Secrets:    PlanSecrets{Mode: PlanSecretModeRecipe},
		SecretSources: []PlanSecretSource{{
			SecretName: "app_secret_abcd",
			Recipe: &PlanSecretRecipe{
				Source:       "secret.tmpl",
				RenderedHash: secretValueHash("user=alice\npass=secret\n"),
			},
			Dependencies: []PlanSecretDependency{
				{Name: "username", Hash: secretValueHash("alice"), Provider: "file", Key: "username"},
				{Name: "password", Hash: secretValueHash("secret"), Provider: "file", Key: "password"},
			},
		}},
		Plan: Plan{
			CreateSecrets: []swarm.SecretSpec{{Name: "app_secret_abcd"}},
			Assumptions: PlanAssumptions{
				AbsentSecrets: []string{"app_secret_abcd"},
			},
		},
	}

	if err := ValidatePlanFile(planFile); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestPlanSourceInputForGitSourceRequiresMatchingRef(t *testing.T) {
	inputs := []PlanSourceInput{
		{
			Kind:    "git",
			URL:     "ssh://git@example.com/repo.git",
			Ref:     "main",
			Commit:  "1111111111111111111111111111111111111111",
			Path:    "secret.tmpl",
			Subtree: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		{
			Kind:    "git",
			URL:     "ssh://git@example.com/repo.git",
			Ref:     "release",
			Commit:  "2222222222222222222222222222222222222222",
			Path:    "secret.tmpl",
			Subtree: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
	}

	input, ok := planSourceInputForGitSource(inputs, config.GitSource{
		URL:  "ssh://git@example.com/repo.git",
		Ref:  "release",
		Path: "secret.tmpl",
	})
	if !ok {
		t.Fatalf("expected matching source input")
	}
	if input.Commit != "2222222222222222222222222222222222222222" {
		t.Fatalf("selected wrong source input: %#v", input)
	}
}

func TestPlanRecipeResolverPrefersScopedSecretValue(t *testing.T) {
	scope := PlanScope{
		Project:    "demo",
		Deployment: "prod",
		Stack:      "core",
		Partition:  "qa",
		Service:    "api",
	}
	resolver := planRecipeResolver{
		scope: scope,
		values: map[string]string{
			planRecipeSecretKey(PlanScope{}, "password"): "global",
			planRecipeSecretKey(scope, "password"):       "scoped",
		},
	}

	got, err := resolver.SecretValue("password")
	if err != nil {
		t.Fatalf("SecretValue: %v", err)
	}
	if got != "scoped" {
		t.Fatalf("expected scoped value, got %q", got)
	}
}

func runPlanSecretGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, string(out))
	}
	return string(out)
}

func planSecretTestSourceHashKey(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:16])
}
