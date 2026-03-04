package secrets

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/templates"
)

type vaultProvider struct {
	addr         string
	token        string
	auth         config.AuthConfig
	mount        string
	pathTemplate string
	client       *http.Client
}

func newVaultProvider(engine *config.SecretsEngine) (*vaultProvider, error) {
	addr := strings.TrimSpace(engine.Addr)
	if addr == "" {
		addr = strings.TrimSpace(os.Getenv("VAULT_ADDR"))
	}
	if addr == "" {
		return nil, fmt.Errorf("secrets_engine.addr is required for vault provider")
	}
	if engine.Vault == nil {
		return nil, fmt.Errorf("secrets_engine.vault is required for vault provider")
	}
	if strings.TrimSpace(engine.Vault.Mount) == "" {
		return nil, fmt.Errorf("secrets_engine.vault.mount is required for vault provider")
	}
	if strings.TrimSpace(engine.Vault.PathTemplate) == "" {
		return nil, fmt.Errorf("secrets_engine.vault.path_template is required for vault provider")
	}
	return &vaultProvider{
		addr:         strings.TrimRight(addr, "/"),
		token:        readEnvOrFile("VAULT_TOKEN", "VAULT_TOKEN_FILE"),
		auth:         engine.Auth,
		mount:        strings.Trim(engine.Vault.Mount, "/"),
		pathTemplate: engine.Vault.PathTemplate,
		client:       http.DefaultClient,
	}, nil
}

func (v *vaultProvider) Resolve(ctx context.Context, scope templates.Scope, name string) (string, error) {
	ref, ok := parseSecretRef(name)
	if ok && ref.scheme != "vault" && ref.scheme != "bao" && ref.scheme != "openbao" {
		return "", fmt.Errorf("unsupported secret scheme %q", ref.scheme)
	}
	path := templates.ExpandPathTokens(v.pathTemplate, scope)
	key := name
	if ok {
		if ref.path != "" {
			path = strings.Trim(ref.path, "/")
		}
		if ref.key != "" {
			key = ref.key
		}
	}
	if key == "" {
		return "", fmt.Errorf("secret reference missing key")
	}
	if path == "" {
		return "", fmt.Errorf("secret path is empty")
	}
	return v.readKV(ctx, path, key)
}

func (v *vaultProvider) Put(ctx context.Context, scope templates.Scope, name string, value string) error {
	ref, ok := parseSecretRef(name)
	if ok && ref.scheme != "vault" && ref.scheme != "bao" && ref.scheme != "openbao" {
		return fmt.Errorf("unsupported secret scheme %q", ref.scheme)
	}
	path := templates.ExpandPathTokens(v.pathTemplate, scope)
	key := name
	if ok {
		if ref.path != "" {
			path = strings.Trim(ref.path, "/")
		}
		if ref.key != "" {
			key = ref.key
		}
	}
	if key == "" {
		return fmt.Errorf("secret reference missing key")
	}
	if path == "" {
		return fmt.Errorf("secret path is empty")
	}
	if err := v.ensureToken(ctx); err != nil {
		return err
	}
	data, err := v.readKVMap(ctx, path)
	if err != nil {
		if err != ErrSecretNotFound {
			return err
		}
		data = make(map[string]any)
	}
	data[key] = value
	return v.writeKV(ctx, path, data)
}

type secretRef struct {
	scheme string
	path   string
	key    string
}

func parseSecretRef(value string) (secretRef, bool) {
	if !strings.Contains(value, "://") {
		return secretRef{}, false
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return secretRef{}, false
	}
	return secretRef{
		scheme: parsed.Scheme,
		path:   strings.TrimPrefix(parsed.Path, "/"),
		key:    parsed.Fragment,
	}, true
}

func (v *vaultProvider) readKV(ctx context.Context, path string, key string) (string, error) {
	data, err := v.readKVMap(ctx, path)
	if err != nil {
		return "", err
	}
	value, ok := data[key]
	if !ok {
		return "", ErrSecretNotFound
	}
	switch v := value.(type) {
	case string:
		return v, nil
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v), nil
		}
		return string(encoded), nil
	}
}

func (v *vaultProvider) readKVMap(ctx context.Context, path string) (map[string]any, error) {
	if err := v.ensureToken(ctx); err != nil {
		return nil, err
	}
	full := fmt.Sprintf("%s/v1/%s/data/%s", v.addr, v.mount, strings.Trim(path, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Vault-Token", v.token)
	resp, err := v.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrSecretNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vault read %s: status %s", full, resp.Status)
	}

	var payload struct {
		Data struct {
			Data map[string]any `json:"data"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if payload.Data.Data == nil {
		return map[string]any{}, nil
	}
	return payload.Data.Data, nil
}

func (v *vaultProvider) writeKV(ctx context.Context, path string, data map[string]any) error {
	full := fmt.Sprintf("%s/v1/%s/data/%s", v.addr, v.mount, strings.Trim(path, "/"))
	payload := map[string]any{"data": data}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, full, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Vault-Token", v.token)
	resp, err := v.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("vault write %s: status %s", full, resp.Status)
	}
	return nil
}

func (v *vaultProvider) ensureToken(ctx context.Context) error {
	if v.token != "" {
		return nil
	}
	method := strings.ToLower(strings.TrimSpace(v.auth.Method))
	if method == "" {
		return fmt.Errorf("secrets engine authentication failed: VAULT_TOKEN is required for vault provider")
	}
	path := normalizeAuthPath(method, v.auth.Path)
	if path == "" {
		return fmt.Errorf("secrets engine authentication failed: secrets_engine.auth.path is required for auth method %q", method)
	}
	switch method {
	case "jwt":
		if err := v.loginJWT(ctx, path, v.auth.Role, v.auth.Audience); err != nil {
			return fmt.Errorf("secrets engine authentication failed: %w", err)
		}
	case "kubernetes":
		if err := v.loginKubernetes(ctx, path, v.auth.Role); err != nil {
			return fmt.Errorf("secrets engine authentication failed: %w", err)
		}
	case "approle":
		if err := v.loginAppRole(ctx, path); err != nil {
			return fmt.Errorf("secrets engine authentication failed: %w", err)
		}
	case "oidc":
		return fmt.Errorf("secrets engine authentication failed: oidc auth is not supported in non-interactive mode")
	case "tls":
		return fmt.Errorf("secrets engine authentication failed: tls auth is not supported yet")
	default:
		return fmt.Errorf("secrets engine authentication failed: unknown auth method %q", method)
	}
	return nil
}

func normalizeAuthPath(method string, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		switch method {
		case "jwt":
			return "auth/jwt/login"
		case "kubernetes":
			return "auth/kubernetes/login"
		case "approle":
			return "auth/approle/login"
		case "oidc":
			return "auth/oidc/login"
		default:
			return ""
		}
	}
	if strings.HasSuffix(path, "/login") {
		return path
	}
	return strings.TrimRight(path, "/") + "/login"
}

func (v *vaultProvider) loginJWT(ctx context.Context, path string, role string, audience string) error {
	jwt := readEnvOrFile("VAULT_JWT", "VAULT_JWT_FILE")
	if jwt == "" {
		return fmt.Errorf("VAULT_JWT or VAULT_JWT_FILE is required for jwt auth")
	}
	if role == "" {
		return fmt.Errorf("secrets_engine.auth.role is required for jwt auth")
	}
	payload := map[string]any{
		"role": role,
		"jwt":  jwt,
	}
	if audience != "" {
		payload["audience"] = audience
	}
	return v.login(ctx, path, payload)
}

func (v *vaultProvider) loginKubernetes(ctx context.Context, path string, role string) error {
	token := readEnvOrFile("VAULT_K8S_TOKEN", "VAULT_K8S_TOKEN_FILE")
	if token == "" {
		token = strings.TrimSpace(readFile("/var/run/secrets/kubernetes.io/serviceaccount/token"))
	}
	if token == "" {
		return fmt.Errorf("VAULT_K8S_TOKEN or service account token is required for kubernetes auth")
	}
	if role == "" {
		return fmt.Errorf("secrets_engine.auth.role is required for kubernetes auth")
	}
	payload := map[string]any{
		"role": role,
		"jwt":  token,
	}
	return v.login(ctx, path, payload)
}

func (v *vaultProvider) loginAppRole(ctx context.Context, path string) error {
	roleID := readEnvOrFile("VAULT_ROLE_ID", "VAULT_ROLE_ID_FILE")
	secretID := readEnvOrFile("VAULT_SECRET_ID", "VAULT_SECRET_ID_FILE")
	if roleID == "" || secretID == "" {
		return fmt.Errorf("VAULT_ROLE_ID/VAULT_ROLE_ID_FILE and VAULT_SECRET_ID/VAULT_SECRET_ID_FILE are required for approle auth")
	}
	payload := map[string]any{
		"role_id":   roleID,
		"secret_id": secretID,
	}
	return v.login(ctx, path, payload)
}

func (v *vaultProvider) login(ctx context.Context, path string, payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	full := fmt.Sprintf("%s/v1/%s", v.addr, strings.TrimPrefix(path, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, full, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := v.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("vault auth %s: status %s", full, resp.Status)
	}
	var response struct {
		Auth struct {
			ClientToken string `json:"client_token"`
		} `json:"auth"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return err
	}
	if response.Auth.ClientToken == "" {
		return fmt.Errorf("vault auth %s: empty client token", full)
	}
	v.token = response.Auth.ClientToken
	return nil
}

func readFile(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func readEnvOrFile(envKey string, fileKey string) string {
	value := strings.TrimSpace(os.Getenv(envKey))
	if value != "" {
		return value
	}
	path := strings.TrimSpace(os.Getenv(fileKey))
	if path == "" {
		return ""
	}
	return strings.TrimSpace(readFile(path))
}
