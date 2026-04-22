package secrets

import (
	"context"
	"testing"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/templates"
)

type fakeProvider struct {
	engine *config.SecretsEngine
}

func (f *fakeProvider) Resolve(ctx context.Context, scope templates.Scope, name string) (string, error) {
	return f.engine.Addr + "|" + name, nil
}

func (f *fakeProvider) Put(ctx context.Context, scope templates.Scope, name string, value string) error {
	return nil
}

func TestResolverUsesScopedSecretsEngine(t *testing.T) {
	prev := providerFactory
	providerFactory = func(engine *config.SecretsEngine) (provider, error) {
		return &fakeProvider{engine: engine}, nil
	}
	t.Cleanup(func() { providerFactory = prev })

	cfg := &config.Config{
		Project: config.Project{
			Name:       "demo",
			Deployment: "prod",
			SecretsEngine: &config.SecretsEngine{
				Provider: "vault",
				Addr:     "base",
				Auth:     config.AuthConfig{Method: "approle"},
				Vault:    &config.VaultKV{Mount: "kv", PathTemplate: "{project}/{partition}/{stack}/{service}"},
			},
		},
		Overlays: config.Overlays{
			Deployments: map[string]config.Overlay{
				"prod": {
					Project: config.OverlayProject{
						SecretsEngine: &config.SecretsEngine{
							Provider: "vault",
							Addr:     "deploy",
							Auth:     config.AuthConfig{Method: "approle"},
							Vault:    &config.VaultKV{Mount: "kv", PathTemplate: "{project}/{partition}/{stack}/{service}"},
						},
					},
				},
			},
			Partitions: config.PartitionOverlays{
				Rules: []config.PartitionOverlay{
					{
						Name:  "prod",
						Match: config.OverlayMatch{Partition: config.OverlayMatchPartition{Pattern: "prod"}},
						Overlay: config.Overlay{
							Project: config.OverlayProject{
								SecretsEngine: &config.SecretsEngine{
									Provider: "vault",
									Addr:     "partition",
									Auth:     config.AuthConfig{Method: "approle"},
									Vault:    &config.VaultKV{Mount: "kv", PathTemplate: "{project}/{partition}/{stack}/{service}"},
								},
							},
						},
					},
				},
			},
		},
	}

	resolver, err := NewResolver(cfg, nil, false)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	value, err := resolver.Value(templates.Scope{Project: "demo", Deployment: "prod", Partition: "prod"}, "api_key")
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	if value != "partition|api_key" {
		t.Fatalf("unexpected resolved value: %q", value)
	}
}

func TestWriterRejectsWhenScopedSecretsEngineAbsent(t *testing.T) {
	prev := providerFactory
	providerFactory = func(engine *config.SecretsEngine) (provider, error) {
		return &fakeProvider{engine: engine}, nil
	}
	t.Cleanup(func() { providerFactory = prev })

	cfg := &config.Config{Project: config.Project{Name: "demo"}}
	writer, err := NewWriter(cfg)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	err = writer.Put(templates.Scope{Project: "demo"}, "name", "value")
	if err != ErrSecretWriteUnsupported {
		t.Fatalf("unexpected error: %v", err)
	}
}
