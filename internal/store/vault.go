package store

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	vault "github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/api/auth/approle"

	"github.com/cmmoran/swarmcp/internal/spec"
)

type vaultClient struct {
	api     *vault.Client
	closers []func()
	mu      sync.Mutex
}

func newVault(cfg spec.SecretsProviderSpec) (Client, error) {
	vCfg := vault.DefaultConfig() // honors BAO_* and VAULT_* env vars
	if cfg.Addr != "" {
		_ = vCfg.ReadEnvironment()
		vCfg.Address = cfg.Addr
	} else {
		_ = vCfg.ReadEnvironment() // VAULT_ADDR or BAO_ADDR, etc.
	}
	api, err := vault.NewClient(vCfg)
	if err != nil {
		return nil, err
	}
	if cfg.Namespace != "" {
		api.SetNamespace(cfg.Namespace)
	}
	if cfg.RoleIDPath == "" || cfg.WrappedSecretIDPath == "" {
		return nil, fmt.Errorf("vault: roleIdPath and wrappedSecretIdPath are required")
	}
	roleID, err := os.ReadFile(cfg.RoleIDPath)
	if err != nil {
		return nil, fmt.Errorf("vault: read roleIdPath: %w", err)
	}
	vCli := &vaultClient{api: api}

	roleIDStr := strings.TrimSpace(string(roleID))
	var appRoleAuth *approle.AppRoleAuth
	if appRoleAuth, err = approle.NewAppRoleAuth(roleIDStr, &approle.SecretID{
		FromFile: cfg.WrappedSecretIDPath,
	}, approle.WithWrappingToken()); err != nil {
		return nil, err
	}

	if sec, loginErr := vCli.api.Auth().Login(context.Background(), appRoleAuth); loginErr != nil {
		return nil, loginErr
	} else {
		vCli.WithToken(sec.Auth.ClientToken)
		if sec.Renewable {
			if stopRenewer, srerr := vCli.StartAutoRenew(context.Background(), sec); srerr == nil && stopRenewer != nil {
				vCli.closers = append(vCli.closers, stopRenewer)
			}
		}
	}

	return vCli, nil
}

func (c *vaultClient) ResolveSecret(ctx context.Context, path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("vault: empty path")
	}
	basePath, field := splitField(path)
	// Try KV v2 first
	if b, err := c.readKVv2(ctx, basePath, field); err == nil {
		return b, nil
	}
	// Fallback: raw read
	sec, err := c.api.Logical().ReadWithContext(ctx, basePath)
	if err != nil {
		return nil, err
	}
	if sec == nil || sec.Data == nil {
		return nil, fmt.Errorf("vault: no data at %s", basePath)
	}
	return pickField(sec.Data, field)
}

func (c *vaultClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, fn := range c.closers {
		fn()
	}
	c.closers = nil
}

func (c *vaultClient) WithNamespace(ns string) { c.api.SetNamespace(ns) }

func (c *vaultClient) WithToken(t string) { c.api.SetToken(t) }
func (c *vaultClient) Token() string      { return c.api.Token() }
func (c *vaultClient) Read(p string) (map[string]any, error) {
	s, e := c.api.Logical().Read(p)
	return coerce(s, e)
}
func (c *vaultClient) Write(p string, d map[string]any) (map[string]any, error) {
	s, e := c.api.Logical().Write(p, d)
	return coerce(s, e)
}
func (c *vaultClient) KVGet(mount, path string) (map[string]any, error) {
	s, err := c.api.Logical().Read(fmt.Sprintf("%s/data/%s", trimS(mount), trimL(path)))
	if err != nil {
		return nil, err
	}
	if s == nil || s.Data == nil {
		return nil, nil
	}
	if m, ok := s.Data["data"].(map[string]any); ok {
		return m, nil
	}
	return nil, fmt.Errorf("unexpected kv v2 payload at %q", path)
}
func (c *vaultClient) KVPut(mount, path string, data map[string]any) error {
	_, err := c.api.Logical().Write(fmt.Sprintf("%s/data/%s", trimS(mount), trimL(path)), map[string]any{"data": data})
	return err
}

func (c *vaultClient) StartAutoRenew(ctx context.Context, lease any) (func(), error) {
	s, _ := lease.(*vault.Secret)
	if s == nil {
		return func() {}, nil
	}
	r, err := c.api.NewLifetimeWatcher(&vault.LifetimeWatcherInput{Secret: s})
	if err != nil {
		return nil, err
	}
	go r.Renew()
	go func() {
		defer r.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case ren := <-r.RenewCh():
				if ren != nil && ren.Secret != nil && ren.Secret.Auth != nil && len(ren.Secret.Auth.ClientToken) > 0 {
					c.api.SetToken(ren.Secret.Auth.ClientToken)
				}
			case <-r.DoneCh():
				return
			}
		}
	}()
	return r.Stop, nil
}
func (c *vaultClient) readKVv2(ctx context.Context, basePath, field string) ([]byte, error) {
	kvPath := toKVv2Path(basePath)
	sec, err := c.api.Logical().ReadWithContext(ctx, kvPath)
	if err != nil {
		return nil, err
	}
	if sec == nil || sec.Data == nil {
		return nil, fmt.Errorf("no data")
	}
	// KV v2 shape: data.data
	rawData, ok := sec.Data["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("not kvv2")
	}
	return pickField(rawData, field)
}

func (c *vaultClient) Unwrap(token string) (map[string]any, error) {
	old := c.api.Token()
	c.api.SetToken(token)
	defer c.api.SetToken(old)
	s, err := c.api.Logical().Unwrap(token)
	return coerce(s, err, "secret_id")
}
