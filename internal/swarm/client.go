package swarm

import "context"

type Client interface {
    EnsureNetworks(ctx context.Context, names []string) error
}

type NoopClient struct{}

func NewNoopClient() *NoopClient { return &NoopClient{} }

func (c *NoopClient) EnsureNetworks(ctx context.Context, names []string) error { _ = ctx; _ = names; return nil }
