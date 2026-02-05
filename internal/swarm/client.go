package swarm

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/cmmoran/swarmcp/internal/fsutil"
	"github.com/docker/cli/cli/connhelper"
	"github.com/docker/docker/api/types"
	dockerapi "github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
)

var ErrNotImplemented = errors.New("swarm client not implemented")

type Client interface {
	ListConfigs(ctx context.Context) ([]Config, error)
	ListSecrets(ctx context.Context) ([]Secret, error)
	ListServices(ctx context.Context) ([]Service, error)
	ListNetworks(ctx context.Context) ([]Network, error)
	ListNodes(ctx context.Context) ([]Node, error)
	ConfigContent(ctx context.Context, id string) ([]byte, error)
	CreateNetwork(ctx context.Context, spec NetworkSpec) (string, error)
	CreateService(ctx context.Context, spec dockerapi.ServiceSpec) (string, error)
	CreateConfig(ctx context.Context, spec ConfigSpec) (string, error)
	CreateSecret(ctx context.Context, spec SecretSpec) (string, error)
	RemoveConfig(ctx context.Context, id string) error
	RemoveSecret(ctx context.Context, id string) error
	UpdateService(ctx context.Context, service Service, spec dockerapi.ServiceSpec) error
	UpdateNode(ctx context.Context, node Node, spec dockerapi.NodeSpec) error
}

func NewClient(contextName string) (Client, error) {
	if contextName == "" || contextName == "default" {
		cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			return nil, err
		}
		return &apiClient{cli: cli}, nil
	}

	endpoint, tlsConfig, err := loadContextEndpoint(contextName)
	if err != nil {
		return nil, err
	}

	helper, err := connhelper.GetConnectionHelper(endpoint.Host)
	if err != nil {
		return nil, err
	}

	opts := []client.Opt{client.WithAPIVersionNegotiation()}
	if helper != nil {
		opts = append(opts,
			client.WithHost(helper.Host),
			client.WithHTTPClient(&http.Client{
				Transport: &http.Transport{
					DialContext: helper.Dialer,
				},
			}),
			client.WithDialContext(helper.Dialer),
		)
	} else {
		opts = append(opts, client.WithHost(endpoint.Host))
		if tlsConfig != nil {
			opts = append(opts, client.WithHTTPClient(&http.Client{
				Transport: &http.Transport{
					Proxy:           http.ProxyFromEnvironment,
					TLSClientConfig: tlsConfig,
				},
			}))
		}
	}

	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, err
	}

	return &apiClient{cli: cli}, nil
}

type apiClient struct {
	cli *client.Client
}

func (c *apiClient) ListConfigs(ctx context.Context) ([]Config, error) {
	configs, err := c.cli.ConfigList(ctx, types.ConfigListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]Config, 0, len(configs))
	for _, cfg := range configs {
		out = append(out, Config{
			ID:        cfg.ID,
			Name:      cfg.Spec.Annotations.Name,
			Labels:    cfg.Spec.Annotations.Labels,
			CreatedAt: cfg.CreatedAt,
		})
	}
	return out, nil
}

func (c *apiClient) ListSecrets(ctx context.Context) ([]Secret, error) {
	secrets, err := c.cli.SecretList(ctx, types.SecretListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]Secret, 0, len(secrets))
	for _, sec := range secrets {
		out = append(out, Secret{
			ID:        sec.ID,
			Name:      sec.Spec.Annotations.Name,
			Labels:    sec.Spec.Annotations.Labels,
			CreatedAt: sec.CreatedAt,
		})
	}
	return out, nil
}

func (c *apiClient) ListServices(ctx context.Context) ([]Service, error) {
	services, err := c.cli.ServiceList(ctx, types.ServiceListOptions{Status: true})
	if err != nil {
		return nil, err
	}
	out := make([]Service, 0, len(services))
	for _, svc := range services {
		out = append(out, Service{
			ID:      svc.ID,
			Name:    svc.Spec.Annotations.Name,
			Labels:  svc.Spec.Annotations.Labels,
			Spec:    svc.Spec,
			Version: svc.Version.Index,
			Status:  svc.ServiceStatus,
		})
	}
	return out, nil
}

func (c *apiClient) ListNetworks(ctx context.Context) ([]Network, error) {
	networks, err := c.cli.NetworkList(ctx, types.NetworkListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]Network, 0, len(networks))
	for _, net := range networks {
		var subnets []string
		for _, cfg := range net.IPAM.Config {
			if cfg.Subnet != "" {
				subnets = append(subnets, cfg.Subnet)
			}
		}
		out = append(out, Network{
			ID:      net.ID,
			Name:    net.Name,
			Driver:  net.Driver,
			Scope:   net.Scope,
			Subnets: subnets,
		})
	}
	return out, nil
}

func (c *apiClient) ListNodes(ctx context.Context) ([]Node, error) {
	nodes, err := c.cli.NodeList(ctx, types.NodeListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]Node, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, Node{
			ID:       node.ID,
			Name:     node.Spec.Annotations.Name,
			Hostname: node.Description.Hostname,
			Labels:   node.Spec.Labels,
			Spec:     node.Spec,
			Version:  node.Version.Index,
		})
	}
	return out, nil
}

func (c *apiClient) ConfigContent(ctx context.Context, id string) ([]byte, error) {
	_, raw, err := c.cli.ConfigInspectWithRaw(ctx, id)
	return raw, err
}

func (c *apiClient) CreateNetwork(ctx context.Context, spec NetworkSpec) (string, error) {
	resp, err := c.cli.NetworkCreate(ctx, spec.Name, types.NetworkCreate{
		Driver:     spec.Driver,
		Attachable: spec.Attachable,
		Internal:   spec.Internal,
		Labels:     spec.Labels,
	})
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (c *apiClient) CreateService(ctx context.Context, spec dockerapi.ServiceSpec) (string, error) {
	resp, err := c.cli.ServiceCreate(ctx, spec, types.ServiceCreateOptions{})
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (c *apiClient) CreateConfig(ctx context.Context, spec ConfigSpec) (string, error) {
	resp, err := c.cli.ConfigCreate(ctx, dockerapi.ConfigSpec{
		Annotations: dockerapi.Annotations{
			Name:   spec.Name,
			Labels: spec.Labels,
		},
		Data: spec.Data,
	})
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (c *apiClient) CreateSecret(ctx context.Context, spec SecretSpec) (string, error) {
	resp, err := c.cli.SecretCreate(ctx, dockerapi.SecretSpec{
		Annotations: dockerapi.Annotations{
			Name:   spec.Name,
			Labels: spec.Labels,
		},
		Data: spec.Data,
	})
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (c *apiClient) RemoveConfig(ctx context.Context, id string) error {
	return c.cli.ConfigRemove(ctx, id)
}

func (c *apiClient) RemoveSecret(ctx context.Context, id string) error {
	return c.cli.SecretRemove(ctx, id)
}

func (c *apiClient) UpdateService(ctx context.Context, service Service, spec dockerapi.ServiceSpec) error {
	_, err := c.cli.ServiceUpdate(ctx, service.ID, dockerapi.Version{Index: service.Version}, spec, types.ServiceUpdateOptions{})
	return err
}

func (c *apiClient) UpdateNode(ctx context.Context, node Node, spec dockerapi.NodeSpec) error {
	return c.cli.NodeUpdate(ctx, node.ID, dockerapi.Version{Index: node.Version}, spec)
}

type contextMetadata struct {
	Name      string                     `json:"name,omitempty"`
	Endpoints map[string]json.RawMessage `json:"endpoints,omitempty"`
}

type contextEndpoint struct {
	Host          string `json:"Host,omitempty"`
	SkipTLSVerify bool   `json:"SkipTLSVerify,omitempty"`
}

func loadContextEndpoint(contextName string) (contextEndpoint, *tls.Config, error) {
	configDir := dockerConfigDir()
	contextID := contextDir(contextName)
	metaPath := filepath.Join(configDir, "contexts", "meta", contextID, "meta.json")
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		return contextEndpoint{}, nil, fmt.Errorf("load context %q: %w", contextName, err)
	}

	var meta contextMetadata
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return contextEndpoint{}, nil, fmt.Errorf("parse context %q: %w", contextName, err)
	}
	rawEndpoint, ok := meta.Endpoints["docker"]
	if !ok {
		return contextEndpoint{}, nil, fmt.Errorf("context %q: docker endpoint not found", contextName)
	}
	var endpoint contextEndpoint
	if err := json.Unmarshal(rawEndpoint, &endpoint); err != nil {
		return contextEndpoint{}, nil, fmt.Errorf("context %q: parse docker endpoint: %w", contextName, err)
	}
	if endpoint.Host == "" {
		return contextEndpoint{}, nil, fmt.Errorf("context %q: docker endpoint host is empty", contextName)
	}

	tlsConfig, err := loadTLSConfig(configDir, contextID, endpoint.SkipTLSVerify)
	if err != nil {
		return contextEndpoint{}, nil, fmt.Errorf("context %q: %w", contextName, err)
	}

	return endpoint, tlsConfig, nil
}

func dockerConfigDir() string {
	if value := os.Getenv("DOCKER_CONFIG"); value != "" {
		return value
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".docker")
}

func contextDir(name string) string {
	sum := sha256.Sum256([]byte(name))
	return hex.EncodeToString(sum[:])
}

func loadTLSConfig(configDir, contextID string, skipVerify bool) (*tls.Config, error) {
	tlsDir := filepath.Join(configDir, "contexts", "tls", contextID, "docker")
	caPath := filepath.Join(tlsDir, "ca.pem")
	certPath := filepath.Join(tlsDir, "cert.pem")
	keyPath := filepath.Join(tlsDir, "key.pem")

	hasCA := fsutil.FileExists(caPath)
	hasCert := fsutil.FileExists(certPath)
	hasKey := fsutil.FileExists(keyPath)
	if !skipVerify && !hasCA && !hasCert && !hasKey {
		return nil, nil
	}

	cfg := &tls.Config{InsecureSkipVerify: skipVerify}
	if hasCA {
		caBytes, err := os.ReadFile(caPath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", caPath, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caBytes) {
			return nil, fmt.Errorf("invalid CA data in %s", caPath)
		}
		cfg.RootCAs = pool
	}
	if hasCert || hasKey {
		if !hasCert || !hasKey {
			return nil, fmt.Errorf("context TLS requires both %s and %s", certPath, keyPath)
		}
		cert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return nil, fmt.Errorf("load TLS cert/key: %w", err)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	return cfg, nil
}
