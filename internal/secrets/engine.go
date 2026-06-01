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

type MetadataResolver interface {
	ValueWithMetadata(scope templates.Scope, name string) (ResolvedSecret, error)
}

type ResolvedSecret struct {
	Value    string         `json:"value,omitempty" yaml:"value,omitempty"`
	Metadata SecretMetadata `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

type SecretMetadata struct {
	Provider string            `json:"provider,omitempty" yaml:"provider,omitempty"`
	Addr     string            `json:"addr,omitempty" yaml:"addr,omitempty"`
	Auth     config.AuthConfig `json:"auth,omitempty" yaml:"auth,omitempty"`
	Mount    string            `json:"mount,omitempty" yaml:"mount,omitempty"`
	Path     string            `json:"path,omitempty" yaml:"path,omitempty"`
	Key      string            `json:"key,omitempty" yaml:"key,omitempty"`
	Version  *int              `json:"version,omitempty" yaml:"version,omitempty"`
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

type metadataProvider interface {
	ResolveWithMetadata(ctx context.Context, scope templates.Scope, name string) (ResolvedSecret, error)
}

type providerWriter interface {
	Put(ctx context.Context, scope templates.Scope, name string, value string) error
}

type manager struct {
	cfg          *config.Config
	store        *Store
	allowMissing bool
	missing      map[string]struct{}
}

var ErrSecretNotFound = errors.New("secret not found")

var ErrSecretWriteUnsupported = errors.New("secret write unsupported")

var providerFactory = newProvider

func NewResolver(cfg *config.Config, store *Store, allowMissing bool) (Resolver, error) {
	return &manager{
		cfg:          cfg,
		store:        store,
		allowMissing: allowMissing,
		missing:      make(map[string]struct{}),
	}, nil
}

func NewWriter(cfg *config.Config) (Writer, error) {
	return &writerWrapper{cfg: cfg}, nil
}

type writerWrapper struct {
	cfg *config.Config
}

func (w *writerWrapper) Put(scope templates.Scope, name string, value string) error {
	if w.cfg == nil {
		return ErrSecretWriteUnsupported
	}
	engineCfg := w.cfg.ProjectSecretsEngine(scope.Partition)
	if engineCfg == nil {
		return ErrSecretWriteUnsupported
	}
	engine, err := providerFactory(engineCfg)
	if err != nil {
		return err
	}
	writer, ok := engine.(providerWriter)
	if !ok {
		return ErrSecretWriteUnsupported
	}
	return writer.Put(context.Background(), scope, name, value)
}

func (m *manager) Value(scope templates.Scope, name string) (string, error) {
	resolved, err := m.ValueWithMetadata(scope, name)
	if err != nil {
		return "", err
	}
	return resolved.Value, nil
}

func (m *manager) ValueWithMetadata(scope templates.Scope, name string) (ResolvedSecret, error) {
	if m.store != nil {
		if value, ok := m.store.Values[name]; ok {
			return ResolvedSecret{
				Value: value,
				Metadata: SecretMetadata{
					Provider: "file",
					Key:      name,
				},
			}, nil
		}
	}
	var resolved provider
	if m.cfg != nil {
		engineCfg := m.cfg.ProjectSecretsEngine(scope.Partition)
		if engineCfg != nil {
			engine, err := providerFactory(engineCfg)
			if err != nil {
				return ResolvedSecret{}, err
			}
			resolved = engine
		}
	}
	if resolved == nil {
		if m.allowMissing {
			m.recordMissing(scope, name)
			return ResolvedSecret{Value: placeholder(scope, name)}, nil
		}
		return ResolvedSecret{}, fmt.Errorf("%w: %s", ErrSecretNotFound, name)
	}
	if metadataResolver, ok := resolved.(metadataProvider); ok {
		secret, err := metadataResolver.ResolveWithMetadata(context.Background(), scope, name)
		if err != nil {
			if errors.Is(err, ErrSecretNotFound) && m.allowMissing {
				m.recordMissing(scope, name)
				return ResolvedSecret{Value: placeholder(scope, name)}, nil
			}
			return ResolvedSecret{}, err
		}
		return secret, nil
	}
	value, err := resolved.Resolve(context.Background(), scope, name)
	if err != nil {
		if errors.Is(err, ErrSecretNotFound) && m.allowMissing {
			m.recordMissing(scope, name)
			return ResolvedSecret{Value: placeholder(scope, name)}, nil
		}
		return ResolvedSecret{}, err
	}
	return ResolvedSecret{Value: value}, nil
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
