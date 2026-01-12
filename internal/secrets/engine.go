package secrets

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/templates"
)

type Resolver interface {
	Value(scope templates.Scope, name string) (string, error)
}

type Writer interface {
	Put(scope templates.Scope, name string, value string) error
}

type MissingReporter interface {
	Missing() []string
}

type provider interface {
	Resolve(ctx context.Context, scope templates.Scope, name string) (string, error)
}

type providerWriter interface {
	Put(ctx context.Context, scope templates.Scope, name string, value string) error
}

type manager struct {
	store        *Store
	provider     provider
	allowMissing bool
	missing      map[string]struct{}
}

var ErrSecretNotFound = errors.New("secret not found")

var ErrSecretWriteUnsupported = errors.New("secret write unsupported")

func NewResolver(cfg *config.Config, store *Store, allowMissing bool) (Resolver, error) {
	var provider provider
	if cfg.Project.SecretsEngine != nil {
		engine, err := newProvider(cfg.Project.SecretsEngine)
		if err != nil {
			return nil, err
		}
		provider = engine
	}
	return &manager{
		store:        store,
		provider:     provider,
		allowMissing: allowMissing,
		missing:      make(map[string]struct{}),
	}, nil
}

func NewWriter(cfg *config.Config) (Writer, error) {
	if cfg.Project.SecretsEngine == nil {
		return nil, ErrSecretWriteUnsupported
	}
	engine, err := newProvider(cfg.Project.SecretsEngine)
	if err != nil {
		return nil, err
	}
	writer, ok := engine.(providerWriter)
	if !ok {
		return nil, ErrSecretWriteUnsupported
	}
	return &writerWrapper{writer: writer}, nil
}

type writerWrapper struct {
	writer providerWriter
}

func (w *writerWrapper) Put(scope templates.Scope, name string, value string) error {
	return w.writer.Put(context.Background(), scope, name, value)
}

func (m *manager) Value(scope templates.Scope, name string) (string, error) {
	if m.store != nil {
		if value, ok := m.store.Values[name]; ok {
			return value, nil
		}
	}
	if m.provider == nil {
		if m.allowMissing {
			m.recordMissing(scope, name)
			return placeholder(scope, name), nil
		}
		return "", fmt.Errorf("%w: %s", ErrSecretNotFound, name)
	}
	value, err := m.provider.Resolve(context.Background(), scope, name)
	if err != nil {
		if errors.Is(err, ErrSecretNotFound) && m.allowMissing {
			m.recordMissing(scope, name)
			return placeholder(scope, name), nil
		}
		return "", err
	}
	return value, nil
}

func newProvider(engine *config.SecretsEngine) (provider, error) {
	switch strings.ToLower(engine.Provider) {
	case "vault", "bao", "openbao":
		return newVaultProvider(engine)
	default:
		return nil, fmt.Errorf("unknown secrets_engine provider %q", engine.Provider)
	}
}

func placeholder(scope templates.Scope, name string) string {
	partition := scope.Partition
	if partition == "" {
		partition = "none"
	}
	stack := scope.Stack
	if stack == "" {
		stack = "none"
	}
	service := scope.Service
	if service == "" {
		service = "none"
	}
	return fmt.Sprintf("SWARMCP_PLACEHOLDER::{project=%s,stack=%s,partition=%s,service=%s,name=%s}", scope.Project, stack, partition, service, name)
}

func (m *manager) recordMissing(scope templates.Scope, name string) {
	m.missing[missingKey(scope, name)] = struct{}{}
}

func (m *manager) Missing() []string {
	out := make([]string, 0, len(m.missing))
	for item := range m.missing {
		out = append(out, item)
	}
	return out
}

func missingKey(scope templates.Scope, name string) string {
	partition := scope.Partition
	if partition == "" {
		partition = "none"
	}
	stack := scope.Stack
	if stack == "" {
		stack = "none"
	}
	service := scope.Service
	if service == "" {
		service = "none"
	}
	return fmt.Sprintf("project=%s stack=%s partition=%s service=%s name=%s", scope.Project, stack, partition, service, name)
}
