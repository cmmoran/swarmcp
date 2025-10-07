package swarm

import "context"

type Client interface {
	// Networks
	EnsureNetworks(ctx context.Context, nets []NetworkSpec) (map[string]string, error) // name -> ID

	// Configs/Secrets (returns created/ensured name->ID)
	EnsureConfigs(ctx context.Context, cfgs []ConfigPayload) (map[string]string, error)
	EnsureSecrets(ctx context.Context, secs []SecretPayload) (map[string]string, error)

	// Services (create/update; returns ID and whether it was updated)
	EnsureService(ctx context.Context, spec ServiceApply) (string, bool, error)

	// Ownership helpers
	ListOwned(ctx context.Context, ownerLabels map[string]string) ([]OwnedObject, error)
	Prune(ctx context.Context, objs []OwnedObject) error
}

type NoopClient struct{}

func NewNoopClient() *NoopClient { return &NoopClient{} }

func (c *NoopClient) EnsureNetworks(ctx context.Context, nets []NetworkSpec) (map[string]string, error) {
	_ = ctx
	_ = nets
	return map[string]string{}, nil
}
func (c *NoopClient) EnsureConfigs(ctx context.Context, cfgs []ConfigPayload) (map[string]string, error) {
	_ = ctx
	_ = cfgs
	return map[string]string{}, nil
}
func (c *NoopClient) EnsureSecrets(ctx context.Context, secs []SecretPayload) (map[string]string, error) {
	_ = ctx
	_ = secs
	return map[string]string{}, nil
}
func (c *NoopClient) EnsureService(ctx context.Context, spec ServiceApply) (string, bool, error) {
	_ = ctx
	_ = spec
	return "", false, nil
}
func (c *NoopClient) ListOwned(ctx context.Context, ownerLabels map[string]string) ([]OwnedObject, error) {
	_ = ctx
	_ = ownerLabels
	return nil, nil
}
func (c *NoopClient) Prune(ctx context.Context, objs []OwnedObject) error {
	_ = ctx
	_ = objs
	return nil
}

// Lightweight specs used by the client to materialize Swarm objects.
type NetworkSpec struct {
	Name     string
	Driver   string
	Internal bool
	Labels   map[string]string
}
type ConfigPayload struct {
	Name   string
	Bytes  []byte
	Labels map[string]string
}
type SecretPayload struct {
	Name   string
	Bytes  []byte
	Labels map[string]string
}
type ServiceApply struct {
	Name   string
	Labels map[string]string
	// Step 2 will add full swarm.ServiceSpec wiring here.
}

type OwnedObject struct {
	ID     string
	Name   string
	Kind   string // network|config|secret|service
	Labels map[string]string
}
