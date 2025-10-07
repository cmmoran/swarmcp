package swarm

import (
	"context"
	"time"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/swarm"
	dclient "github.com/docker/docker/client"
)

// DockerClient implements Client using the official Docker SDK.
type DockerClient struct {
	c *dclient.Client
}

func NewDockerClient() (*DockerClient, error) {
	cli, err := dclient.NewClientWithOpts(dclient.FromEnv, dclient.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &DockerClient{c: cli}, nil
}

// EnsureNetworks idempotently creates overlay networks with labels.
func (d *DockerClient) EnsureNetworks(ctx context.Context, nets []NetworkSpec) (map[string]string, error) {
	out := make(map[string]string, len(nets))
	if len(nets) == 0 {
		return out, nil
	}
	for _, ns := range nets {
		flt := filters.NewArgs()
		flt.Add("name", ns.Name)
		nws, err := d.c.NetworkList(ctx, network.ListOptions{Filters: flt})
		if err != nil {
			return nil, err
		}
		if len(nws) > 0 {
			out[ns.Name] = nws[0].ID
			continue
		}
		resp, err := d.c.NetworkCreate(ctx, ns.Name, network.CreateOptions{
			Driver:     ifEmpty(ns.Driver, "overlay"),
			Internal:   ns.Internal,
			Attachable: true,
			Labels:     ns.Labels,
			Scope:      "swarm",
		})
		if err != nil {
			return nil, err
		}
		out[ns.Name] = resp.ID
	}
	return out, nil
}

func (d *DockerClient) EnsureConfigs(ctx context.Context, cfgs []ConfigPayload) (map[string]string, error) {
	out := map[string]string{}
	for _, c := range cfgs {
		id, err := d.findConfigIDByName(ctx, c.Name)
		if err != nil {
			return nil, err
		}
		if id != "" {
			out[c.Name] = id
			continue
		}
		spec := swarm.ConfigSpec{
			Annotations: swarm.Annotations{
				Name:   c.Name,
				Labels: c.Labels,
			},
			Data: c.Bytes,
		}
		resp, err := d.c.ConfigCreate(ctx, spec)
		if err != nil {
			return nil, err
		}
		out[c.Name] = resp.ID
	}
	return out, nil
}

func (d *DockerClient) EnsureSecrets(ctx context.Context, secs []SecretPayload) (map[string]string, error) {
	out := map[string]string{}
	for _, s := range secs {
		id, err := d.findSecretIDByName(ctx, s.Name)
		if err != nil {
			return nil, err
		}
		if id != "" {
			out[s.Name] = id
			continue
		}
		spec := swarm.SecretSpec{
			Annotations: swarm.Annotations{
				Name:   s.Name,
				Labels: s.Labels,
			},
			Data: s.Bytes,
		}
		resp, err := d.c.SecretCreate(ctx, spec)
		if err != nil {
			return nil, err
		}
		out[s.Name] = resp.ID
	}
	return out, nil
}

// EnsureService: For Step 1 we only support create-if-missing; update wiring will land in Step 2.
func (d *DockerClient) EnsureService(ctx context.Context, ap ServiceApply) (string, bool, error) {
	svcID, err := d.findServiceIDByName(ctx, ap.Name)
	if err != nil {
		return "", false, err
	}
	if svcID != "" {
		return svcID, false, nil
	}
	spec := swarm.ServiceSpec{
		Annotations: swarm.Annotations{
			Name:   ap.Name,
			Labels: ap.Labels,
		},
	}
	resp, err := d.c.ServiceCreate(ctx, spec, swarm.ServiceCreateOptions{})
	if err != nil {
		return "", false, err
	}
	return resp.ID, true, nil
}

func (d *DockerClient) ListOwned(ctx context.Context, ownerLabels map[string]string) ([]OwnedObject, error) {
	var owned []OwnedObject
	f := filters.NewArgs()
	for k, v := range ownerLabels {
		f.Add("label", k+"="+v)
	}
	svcs, err := d.c.ServiceList(ctx, swarm.ServiceListOptions{Filters: f})
	if err != nil {
		return nil, err
	}
	for _, s := range svcs {
		owned = append(owned, OwnedObject{
			ID:     s.ID,
			Name:   s.Spec.Annotations.Name,
			Kind:   "service",
			Labels: s.Spec.Annotations.Labels,
		})
	}
	nws, err := d.c.NetworkList(ctx, network.ListOptions{Filters: f})
	if err != nil {
		return nil, err
	}
	for _, n := range nws {
		owned = append(owned, OwnedObject{
			ID:     n.ID,
			Name:   n.Name,
			Kind:   "network",
			Labels: n.Labels,
		})
	}
	cfgs, err := d.c.ConfigList(ctx, swarm.ConfigListOptions{Filters: f})
	if err != nil {
		return nil, err
	}
	for _, c := range cfgs {
		owned = append(owned, OwnedObject{
			ID:     c.ID,
			Name:   c.Spec.Name,
			Kind:   "config",
			Labels: c.Spec.Labels,
		})
	}
	secs, err := d.c.SecretList(ctx, swarm.SecretListOptions{Filters: f})
	if err != nil {
		return nil, err
	}
	for _, s := range secs {
		owned = append(owned, OwnedObject{
			ID:     s.ID,
			Name:   s.Spec.Name,
			Kind:   "secret",
			Labels: s.Spec.Labels,
		})
	}
	return owned, nil
}

func (d *DockerClient) Prune(ctx context.Context, objs []OwnedObject) error {
	for _, o := range objs {
		switch o.Kind {
		case "service":
			timeout := 10 * time.Second
			_ = d.c.ServiceRemove(ctx, o.ID)
			_ = timeout
		case "network":
			_ = d.c.NetworkRemove(ctx, o.ID)
		case "config":
			_ = d.c.ConfigRemove(ctx, o.ID)
		case "secret":
			_ = d.c.SecretRemove(ctx, o.ID)
		}
	}
	return nil
}

func (d *DockerClient) findServiceIDByName(ctx context.Context, name string) (string, error) {
	f := filters.NewArgs()
	f.Add("name", name)
	svcs, err := d.c.ServiceList(ctx, swarm.ServiceListOptions{Filters: f})
	if err != nil {
		return "", err
	}
	if len(svcs) > 0 {
		return svcs[0].ID, nil
	}
	return "", nil
}
func (d *DockerClient) findConfigIDByName(ctx context.Context, name string) (string, error) {
	f := filters.NewArgs()
	f.Add("name", name)
	cfgs, err := d.c.ConfigList(ctx, swarm.ConfigListOptions{Filters: f})
	if err != nil {
		return "", err
	}
	if len(cfgs) > 0 {
		return cfgs[0].ID, nil
	}
	return "", nil
}
func (d *DockerClient) findSecretIDByName(ctx context.Context, name string) (string, error) {
	f := filters.NewArgs()
	f.Add("name", name)
	secs, err := d.c.SecretList(ctx, swarm.SecretListOptions{Filters: f})
	if err != nil {
		return "", err
	}
	if len(secs) > 0 {
		return secs[0].ID, nil
	}
	return "", nil
}

func ifEmpty(v string, def string) string {
	if v == "" {
		return def
	}
	return v
}
