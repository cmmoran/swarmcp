package vault

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/cmmoran/swarmcp/internal/spec"
	"github.com/hashicorp/vault/api"
)

var (
	ErrNoVaultConfig = errors.New("vault: no config provided")
)

type Client interface {
	// ResolveSecret loads a secret from Vault given a template-rendered logical path.
	// It returns the raw secret bytes (usually a single field).
	ResolveSecret(ctx context.Context, path string) ([]byte, error)
	// Close stops any background renewers (if running).
	Close() error
}

type sdkClient struct {
	v       *api.Client
	closers []func() error
	mu      sync.Mutex
}

func NewNoop() Client { return &noopClient{} }

type noopClient struct{}

func (n *noopClient) ResolveSecret(ctx context.Context, path string) ([]byte, error) {
	_ = ctx
	_ = path
	return []byte{}, nil
}
func (n *noopClient) Close() error { return nil }

// NewFromProject creates a Vault client if project.Spec.Vault is set; otherwise returns a Noop client.
// It performs: unwrap(wrappedSecretId) -> approle login(roleId, secretId) -> starts a token renewer.
func NewFromProject(ctx context.Context, p *spec.Project) (Client, error) {
	if p == nil || p.Spec.Vault.Addr == "" {
		return NewNoop(), nil
	}
	roleIDPath := p.Spec.Vault.RoleIDPath
	wrappedSecretIDPath := p.Spec.Vault.WrappedSecretIDPath
	if roleIDPath == "" || wrappedSecretIDPath == "" {
		return nil, fmt.Errorf("vault: roleIdPath and wrappedSecretIdPath are required")
	}
	roleID, err := os.ReadFile(roleIDPath)
	if err != nil {
		return nil, fmt.Errorf("vault: read roleIdPath: %w", err)
	}
	wrappedSecretID, err := os.ReadFile(wrappedSecretIDPath)
	if err != nil {
		return nil, fmt.Errorf("vault: read wrappedSecretIdPath: %w", err)
	}

	cfg := api.DefaultConfig()
	cfg.Address = p.Spec.Vault.Addr
	vcli, err := api.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("vault: new client: %w", err)
	}
	if ns := strings.TrimSpace(p.Spec.Vault.Namespace); ns != "" {
		vcli.SetNamespace(ns)
	}
	sdk := &sdkClient{v: vcli}

	// 1) unwrap the secret_id
	secretID, err := unwrap(ctx, vcli, strings.TrimSpace(string(wrappedSecretID)))
	if err != nil {
		return nil, err
	}

	// 2) AppRole login
	auth, err := approleLogin(ctx, vcli, strings.TrimSpace(string(roleID)), secretID)
	if err != nil {
		return nil, err
	}

	// 3) Start renewer on the login secret (if renewable)
	if auth != nil && auth.Renewable {
		stopRenewer := startRenewer(ctx, vcli, auth)
		sdk.closers = append(sdk.closers, stopRenewer)
	}

	return sdk, nil
}

func (c *sdkClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var first error
	for _, fn := range c.closers {
		if err := fn(); err != nil && first == nil {
			first = err
		}
	}
	c.closers = nil
	return first
}
