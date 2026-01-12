package cmdutil

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/cmmoran/swarmcp/internal/sliceutil"
	"github.com/cmmoran/swarmcp/internal/swarm"
	"github.com/cmmoran/swarmcp/internal/templates"
)

type swarmNetworkResolver struct {
	contextName string
	client      swarm.Client
	clientErr   error
	loaded      bool
	byName      map[string][]string
	all         []string
}

func ConfigureTemplateNetworkResolver(contextName string) {
	templates.SetNetworkCIDRResolver(&swarmNetworkResolver{contextName: contextName})
}

func (r *swarmNetworkResolver) NetworkCIDRs(name string) ([]string, error) {
	if err := r.ensureClient(); err != nil {
		if errors.Is(err, swarm.ErrNotImplemented) {
			return nil, fmt.Errorf("swarm network lookup not available (context %q)", r.contextName)
		}
		return nil, err
	}
	if err := r.loadNetworks(); err != nil {
		return nil, err
	}
	if name == "" {
		return append([]string(nil), r.all...), nil
	}
	subnets, ok := r.byName[name]
	if !ok {
		return nil, fmt.Errorf("swarm network %q not found", name)
	}
	return append([]string(nil), subnets...), nil
}

func (r *swarmNetworkResolver) ensureClient() error {
	if r.client != nil || r.clientErr != nil {
		return r.clientErr
	}
	client, err := swarm.NewClient(r.contextName)
	if err != nil {
		r.clientErr = err
		return err
	}
	r.client = client
	return nil
}

func (r *swarmNetworkResolver) loadNetworks() error {
	if r.loaded {
		return nil
	}
	r.loaded = true
	networks, err := r.client.ListNetworks(context.Background())
	if err != nil {
		return err
	}
	byName := make(map[string][]string)
	var all []string
	for _, net := range networks {
		if net.Driver != "overlay" || net.Scope != "swarm" || len(net.Subnets) == 0 {
			continue
		}
		byName[net.Name] = append(byName[net.Name], net.Subnets...)
		all = append(all, net.Subnets...)
	}
	for name, subnets := range byName {
		sort.Strings(subnets)
		byName[name] = sliceutil.DedupeSortedStrings(subnets)
	}
	sort.Strings(all)
	r.byName = byName
	r.all = sliceutil.DedupeSortedStrings(all)
	return nil
}
