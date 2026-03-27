package apply

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/sliceutil"
	"github.com/cmmoran/swarmcp/internal/swarm"
)

func buildDesiredNetworks(cfg *config.Config, partitionFilters []string, stackFilters []string) []swarm.NetworkSpec {
	serviceNetworks := collectServiceNetworks(cfg, partitionFilters, stackFilters)
	if len(serviceNetworks) == 0 {
		return nil
	}
	attachable := cfg.Project.Defaults.Networks.Attachable
	out := make([]swarm.NetworkSpec, 0, len(serviceNetworks))
	names := make([]string, 0, len(serviceNetworks))
	for name := range serviceNetworks {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		internal := true
		if name == egressNetworkName(cfg) {
			internal = false
		}
		out = append(out, swarm.NetworkSpec{
			Name:       name,
			Driver:     "overlay",
			Attachable: attachable,
			Internal:   internal,
		})
	}
	return out
}

func collectServiceNetworks(cfg *config.Config, partitionFilters []string, stackFilters []string) map[string]struct{} {
	out := make(map[string]struct{})
	for stackName, stack := range cfg.Stacks {
		if len(stackFilters) > 0 && !selectorContains(stackFilters, stackName) {
			continue
		}
		if !cfg.StackSelectedForRuntime(stackName, partitionFilters) {
			continue
		}
		partitions := []string{""}
		if stack.Mode == "partitioned" && len(cfg.Project.Partitions) > 0 {
			partitions = cfg.StackRuntimePartitions(stackName, partitionFilters)
		}
		for _, partitionName := range partitions {
			services, err := cfg.StackServices(stackName, partitionName)
			if err != nil {
				continue
			}
			if len(services) == 0 {
				continue
			}
			for serviceName, service := range services {
				for _, network := range desiredServiceExternalNetworks(cfg, stackName, stack.Mode, partitionName, serviceName, service) {
					out[network] = struct{}{}
				}
			}
		}
	}
	return out
}

func selectorContains(filters []string, value string) bool {
	for _, filter := range filters {
		if filter == value {
			return true
		}
	}
	return false
}

func desiredServiceExternalNetworks(cfg *config.Config, stackName string, stackMode string, partitionName string, serviceName string, service config.Service) []string {
	var networks []string
	stackNet := stackNetworkName(cfg.Project.Name, stackName, stackMode, partitionName)
	if stackNet != "" {
		networks = append(networks, stackNet)
	}
	if len(cfg.Project.Defaults.Networks.Shared) > 0 {
		networks = append(networks, config.SharedNetworkNames(cfg, partitionName)...)
	}
	if partitionName != "" {
		internal := internalNetworkName(cfg, partitionName)
		if internal != "" {
			networks = append(networks, internal)
		}
	}
	if service.Egress {
		egress := egressNetworkName(cfg)
		if egress != "" {
			networks = append(networks, egress)
		}
	}
	if len(networks) == 0 {
		return nil
	}
	return sliceutil.DedupeStringsPreserveOrder(networks)
}

func desiredServiceNetworks(cfg *config.Config, stackName string, stackMode string, partitionName string, serviceName string, service config.Service) []string {
	networks := desiredServiceExternalNetworks(cfg, stackName, stackMode, partitionName, serviceName, service)
	if service.NetworkEphemeral == nil {
		return networks
	}
	ephemeral := config.EphemeralNetworkName(cfg, stackName, stackMode, partitionName, serviceName)
	if ephemeral != "" {
		networks = append(networks, ephemeral)
	}
	return sliceutil.DedupeStringsPreserveOrder(networks)
}

func stackNetworkName(projectName, stackName, mode, partition string) string {
	if stackName == "" {
		return ""
	}
	return config.StackInstanceName(projectName, stackName, partition, mode)
}

func internalNetworkName(cfg *config.Config, partition string) string {
	base := strings.TrimSpace(cfg.Project.Defaults.Networks.Internal)
	if base == "" {
		base = fmt.Sprintf("%s_<partition>_internal", cfg.Project.Name)
	}
	return config.RenderNetworkTemplate(base, cfg.Project.Name, partition)
}

func egressNetworkName(cfg *config.Config) string {
	base := strings.TrimSpace(cfg.Project.Defaults.Networks.Egress)
	if base == "" {
		base = fmt.Sprintf("%s_egress", cfg.Project.Name)
	}
	return config.RenderNetworkTemplate(base, cfg.Project.Name, "")
}

func buildNetworkTargetIndex(networks []swarm.Network) map[string]string {
	out := make(map[string]string, len(networks)*3)
	for _, net := range networks {
		if net.Name != "" {
			out[net.Name] = net.Name
		}
		if net.ID != "" {
			out[net.ID] = net.Name
			addNetworkIDPrefix(out, net.ID, 12, net.Name)
			addNetworkIDPrefix(out, net.ID, 25, net.Name)
		}
	}
	return out
}

func addNetworkIDPrefix(targets map[string]string, id string, length int, name string) {
	if len(id) < length {
		return
	}
	prefix := id[:length]
	if _, ok := targets[prefix]; ok {
		return
	}
	targets[prefix] = name
}
