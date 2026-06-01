package secrets

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/templates"
)

type fakeVaultTransport struct {
	t        *testing.T
	token    string
	mu       sync.Mutex
	data     map[string]map[string]any
	versions map[string]int
	queries  []string
	writes   []map[string]any
}

func newFakeVaultTransport(t *testing.T) *fakeVaultTransport {
	return &fakeVaultTransport{
		t:        t,
		token:    "test-token",
		data:     make(map[string]map[string]any),
		versions: make(map[string]int),
	}
}

func (f *fakeVaultTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	switch {
	case req.URL.Path == "/v1/auth/approle/login":
		if req.Method != http.MethodPost {
			return response(http.StatusMethodNotAllowed, "method not allowed"), nil
		}
		resp := map[string]any{"auth": map[string]any{"client_token": f.token}}
		return jsonResponse(http.StatusOK, resp), nil
	case strings.HasPrefix(req.URL.Path, "/v1/kv/data/"):
		path := strings.TrimPrefix(req.URL.Path, "/v1/kv/data/")
		if req.Header.Get("X-Vault-Token") != f.token {
			return response(http.StatusForbidden, "unauthorized"), nil
		}
		switch req.Method {
		case http.MethodGet:
			f.mu.Lock()
			data, ok := f.data[path]
			version := f.versions[path]
			f.queries = append(f.queries, req.URL.RawQuery)
			f.mu.Unlock()
			if !ok {
				return response(http.StatusNotFound, "not found"), nil
			}
			resp := map[string]any{
				"data": map[string]any{
					"data": data,
					"metadata": map[string]any{
						"version": version,
					},
				},
			}
			return jsonResponse(http.StatusOK, resp), nil
		case http.MethodPost:
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return response(http.StatusBadRequest, "bad payload"), nil
			}
			var payload struct {
				Data map[string]any `json:"data"`
			}
			if err := json.Unmarshal(body, &payload); err != nil {
				return response(http.StatusBadRequest, "bad payload"), nil
			}
			f.mu.Lock()
			f.data[path] = payload.Data
			f.writes = append(f.writes, payload.Data)
			f.mu.Unlock()
			return response(http.StatusOK, "ok"), nil
		default:
			return response(http.StatusMethodNotAllowed, "method not allowed"), nil
		}
	default:
		return response(http.StatusNotFound, "not found"), nil
	}
}

func response(code int, body string) *http.Response {
	return &http.Response{
		StatusCode: code,
		Status:     http.StatusText(code),
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func jsonResponse(code int, payload any) *http.Response {
	data, _ := json.Marshal(payload)
	return &http.Response{
		StatusCode: code,
		Status:     http.StatusText(code),
		Body:       io.NopCloser(bytes.NewReader(data)),
		Header:     make(http.Header),
	}
}

func TestVaultProviderResolveWithAppRole(t *testing.T) {
	transport := newFakeVaultTransport(t)
	transport.data["primary/dev/core/api"] = map[string]any{"password": "secret"}

	t.Setenv("VAULT_ROLE_ID", "role-id")
	t.Setenv("VAULT_SECRET_ID", "secret-id")

	engine := &config.SecretsEngine{
		Provider: "vault",
		Addr:     "http://vault.test",
		Auth: config.AuthConfig{
			Method: "approle",
		},
		Vault: &config.VaultKV{
			Mount:        "kv",
			PathTemplate: "{project}/{partition}/{stack}/{service}",
		},
	}
	provider, err := newVaultProvider(engine)
	if err != nil {
		t.Fatalf("newVaultProvider: %v", err)
	}
	provider.client = &http.Client{Transport: transport}
	scope := templates.Scope{
		Project:   "primary",
		Partition: "dev",
		Stack:     "core",
		Service:   "api",
	}
	value, err := provider.Resolve(context.Background(), scope, "password")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if value != "secret" {
		t.Fatalf("unexpected value: %q", value)
	}
}

func TestVaultProviderResolveWithMetadataCapturesKVVersion(t *testing.T) {
	transport := newFakeVaultTransport(t)
	transport.data["primary/dev/core/api"] = map[string]any{"password": "secret"}
	transport.versions["primary/dev/core/api"] = 17

	t.Setenv("VAULT_ROLE_ID", "role-id")
	t.Setenv("VAULT_SECRET_ID", "secret-id")

	engine := &config.SecretsEngine{
		Provider: "openbao",
		Addr:     "http://vault.test",
		Auth: config.AuthConfig{
			Method: "approle",
		},
		Vault: &config.VaultKV{
			Mount:        "kv",
			PathTemplate: "{project}/{partition}/{stack}/{service}",
		},
	}
	provider, err := newVaultProvider(engine)
	if err != nil {
		t.Fatalf("newVaultProvider: %v", err)
	}
	provider.client = &http.Client{Transport: transport}
	scope := templates.Scope{
		Project:   "primary",
		Partition: "dev",
		Stack:     "core",
		Service:   "api",
	}
	secret, err := provider.ResolveWithMetadata(context.Background(), scope, "password")
	if err != nil {
		t.Fatalf("ResolveWithMetadata: %v", err)
	}
	if secret.Value != "secret" {
		t.Fatalf("unexpected value: %q", secret.Value)
	}
	if secret.Metadata.Provider != "openbao" || secret.Metadata.Mount != "kv" || secret.Metadata.Path != "primary/dev/core/api" || secret.Metadata.Key != "password" {
		t.Fatalf("unexpected metadata: %#v", secret.Metadata)
	}
	if secret.Metadata.Version == nil || *secret.Metadata.Version != 17 {
		t.Fatalf("expected version 17, got %#v", secret.Metadata.Version)
	}
}

func TestVaultProviderResolveWithMetadataRequestsPinnedKVVersion(t *testing.T) {
	transport := newFakeVaultTransport(t)
	transport.data["primary/dev/core/api"] = map[string]any{"password": "secret"}
	transport.versions["primary/dev/core/api"] = 17

	t.Setenv("VAULT_ROLE_ID", "role-id")
	t.Setenv("VAULT_SECRET_ID", "secret-id")

	engine := &config.SecretsEngine{
		Provider: "vault",
		Addr:     "http://vault.test",
		Auth: config.AuthConfig{
			Method: "approle",
		},
		Vault: &config.VaultKV{
			Mount:        "kv",
			PathTemplate: "{project}/{partition}/{stack}/{service}",
		},
	}
	provider, err := newVaultProvider(engine)
	if err != nil {
		t.Fatalf("newVaultProvider: %v", err)
	}
	provider.client = &http.Client{Transport: transport}
	secret, err := provider.ResolveWithMetadata(context.Background(), templates.Scope{}, "vault://primary/dev/core/api?version=17#password")
	if err != nil {
		t.Fatalf("ResolveWithMetadata: %v", err)
	}
	if secret.Value != "secret" {
		t.Fatalf("unexpected value: %q", secret.Value)
	}
	if secret.Metadata.Version == nil || *secret.Metadata.Version != 17 {
		t.Fatalf("expected version 17, got %#v", secret.Metadata.Version)
	}
	transport.mu.Lock()
	queries := append([]string(nil), transport.queries...)
	transport.mu.Unlock()
	if len(queries) == 0 || queries[len(queries)-1] != "version=17" {
		t.Fatalf("expected pinned version query, got %#v", queries)
	}
}

func TestVaultProviderResolveRejectsInvalidPinnedKVVersion(t *testing.T) {
	t.Setenv("VAULT_ROLE_ID", "role-id")
	t.Setenv("VAULT_SECRET_ID", "secret-id")

	engine := &config.SecretsEngine{
		Provider: "vault",
		Addr:     "http://vault.test",
		Auth: config.AuthConfig{
			Method: "approle",
		},
		Vault: &config.VaultKV{
			Mount:        "kv",
			PathTemplate: "{project}/{partition}/{stack}/{service}",
		},
	}
	provider, err := newVaultProvider(engine)
	if err != nil {
		t.Fatalf("newVaultProvider: %v", err)
	}
	provider.client = &http.Client{Transport: newFakeVaultTransport(t)}
	_, err = provider.ResolveWithMetadata(context.Background(), templates.Scope{}, "vault://primary/dev/core/api?version=latest#password")
	if err == nil || !strings.Contains(err.Error(), "version must be a positive integer") {
		t.Fatalf("expected invalid version error, got %v", err)
	}
}

func TestVaultProviderPutWritesKey(t *testing.T) {
	transport := newFakeVaultTransport(t)
	transport.data["primary/dev/core/api"] = map[string]any{"existing": "value"}

	t.Setenv("VAULT_ROLE_ID", "role-id")
	t.Setenv("VAULT_SECRET_ID", "secret-id")

	engine := &config.SecretsEngine{
		Provider: "bao",
		Addr:     "http://vault.test",
		Auth: config.AuthConfig{
			Method: "approle",
		},
		Vault: &config.VaultKV{
			Mount:        "kv",
			PathTemplate: "{project}/{partition}/{stack}/{service}",
		},
	}
	provider, err := newVaultProvider(engine)
	if err != nil {
		t.Fatalf("newVaultProvider: %v", err)
	}
	provider.client = &http.Client{Transport: transport}
	scope := templates.Scope{
		Project:   "primary",
		Partition: "dev",
		Stack:     "core",
		Service:   "api",
	}
	if err := provider.Put(context.Background(), scope, "password", "new-value"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	transport.mu.Lock()
	data := transport.data["primary/dev/core/api"]
	transport.mu.Unlock()
	if data["existing"] != "value" {
		t.Fatalf("existing key modified: %#v", data)
	}
	if data["password"] != "new-value" {
		t.Fatalf("password not updated: %#v", data)
	}
}

func TestVaultProviderPutCreatesIfMissing(t *testing.T) {
	transport := newFakeVaultTransport(t)

	t.Setenv("VAULT_ROLE_ID_FILE", writeTempFile(t, "role-id"))
	t.Setenv("VAULT_SECRET_ID_FILE", writeTempFile(t, "secret-id"))

	engine := &config.SecretsEngine{
		Provider: "openbao",
		Addr:     "http://vault.test",
		Auth: config.AuthConfig{
			Method: "approle",
		},
		Vault: &config.VaultKV{
			Mount:        "kv",
			PathTemplate: "{project}/{partition}/{stack}/{service}",
		},
	}
	provider, err := newVaultProvider(engine)
	if err != nil {
		t.Fatalf("newVaultProvider: %v", err)
	}
	provider.client = &http.Client{Transport: transport}
	scope := templates.Scope{
		Project:   "primary",
		Partition: "dev",
		Stack:     "core",
		Service:   "api",
	}
	if err := provider.Put(context.Background(), scope, "password", "new-value"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	transport.mu.Lock()
	data := transport.data["primary/dev/core/api"]
	transport.mu.Unlock()
	if data["password"] != "new-value" {
		t.Fatalf("password not created: %#v", data)
	}
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	file := t.TempDir() + "/secret"
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return file
}
