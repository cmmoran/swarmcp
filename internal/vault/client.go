package vault

import "context"

type Client interface {
	ResolveSecret(ctx context.Context, path string, data map[string]any) ([]byte, error)
}

type NoopClient struct{}

func NewNoopClient() *NoopClient { return &NoopClient{} }

func (c *NoopClient) ResolveSecret(ctx context.Context, path string, data map[string]any) ([]byte, error) {
	_ = ctx
	_ = data
	return []byte("noop:" + path), nil
}
