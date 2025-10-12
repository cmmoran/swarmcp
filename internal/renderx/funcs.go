package renderx

import (
	"context"
	"encoding/base64"
	"strings"
	"text/template"

	"github.com/cmmoran/swarmcp/internal/vault"
)

// GetSecretPath returns the in-container path for a secret name.
// It should implement nearest-ancestor resolution (service → stack → project),
// and may return a deterministic default ("/run/secrets/<name>") when unknown.
type GetSecretPath func(name string) (string, error)

// ConfigFuncMap configs may only reference secret paths (never secret bytes).
func ConfigFuncMap(getPath GetSecretPath) template.FuncMap {
	return template.FuncMap{
		"secretPath": func(name string) (string, error) {
			return getPath(name)
		},
	}
}

// SecretFuncMap secrets may read Vault and also reference secret paths.
func SecretFuncMap(ctx context.Context, v vault.Client, getPath GetSecretPath) template.FuncMap {
	return template.FuncMap{
		"secret": func(path string) (string, error) {
			if v == nil {
				return "", nil
			}
			b, err := v.ResolveSecret(ctx, strings.TrimSpace(path))
			if err != nil {
				return "", err
			}
			return string(b), nil
		},
		"secretB64": func(path string) (string, error) {
			if v == nil {
				return "", nil
			}
			b, err := v.ResolveSecret(ctx, strings.TrimSpace(path))
			if err != nil {
				return "", err
			}
			return base64.StdEncoding.EncodeToString(b), nil
		},
		"secretPath": func(name string) (string, error) {
			return getPath(name)
		},
	}
}
