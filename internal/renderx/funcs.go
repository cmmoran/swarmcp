package renderx

import (
	"context"
	"encoding/base64"
	"strings"
	"text/template"

	"github.com/cmmoran/swarmcp/internal/spec"
	"github.com/cmmoran/swarmcp/internal/vault"
)

// secretPath(<name>) provider backed by EffectiveService
func secretPathOf(eff *spec.EffectiveService) func(string) (string, error) {
	return func(name string) (string, error) {
		for _, s := range eff.Secrets {
			if s.Name == name {
				return s.File.Target, nil
			}
		}
		// fallback to Docker default for secrets
		return "/run/secrets/" + name, nil
	}
}

// ConfigFuncMap template.FuncMap for CONFIG templates: ONLY secretPath(name)
func ConfigFuncMap(eff *spec.EffectiveService) template.FuncMap {
	return template.FuncMap{
		"secretPath": secretPathOf(eff),
	}
}

// SecretFuncMap template.FuncMap for SECRET templates: secret(path), secretB64(path), secretPath(name)
func SecretFuncMap(ctx context.Context, v vault.Client, eff *spec.EffectiveService) template.FuncMap {
	return template.FuncMap{
		"secret": func(path string) (string, error) {
			b, err := v.ResolveSecret(ctx, strings.TrimSpace(path))
			if err != nil {
				return "", err
			}
			return string(b), nil
		},
		"secretB64": func(path string) (string, error) {
			b, err := v.ResolveSecret(ctx, strings.TrimSpace(path))
			if err != nil {
				return "", err
			}
			return base64.StdEncoding.EncodeToString(b), nil
		},
		"secretPath": secretPathOf(eff),
	}
}
