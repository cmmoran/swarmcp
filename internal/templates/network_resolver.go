package templates

import "fmt"

type NetworkCIDRResolver interface {
	NetworkCIDRs(name string) ([]string, error)
}

type NetworkCIDRResolverFunc func(name string) ([]string, error)

func (f NetworkCIDRResolverFunc) NetworkCIDRs(name string) ([]string, error) {
	return f(name)
}

var networkCIDRResolver NetworkCIDRResolver

func SetNetworkCIDRResolver(resolver NetworkCIDRResolver) {
	networkCIDRResolver = resolver
}

func swarmNetworkCIDRs(args ...string) ([]string, error) {
	if len(args) > 1 {
		return nil, fmt.Errorf("swarm_network_cidrs expects 0 or 1 arguments")
	}
	name := ""
	if len(args) == 1 {
		name = args[0]
	}
	if networkCIDRResolver == nil {
		return nil, fmt.Errorf("swarm_network_cidrs is unavailable (no swarm resolver)")
	}
	return networkCIDRResolver.NetworkCIDRs(name)
}
