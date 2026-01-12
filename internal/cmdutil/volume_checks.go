package cmdutil

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/sliceutil"
)

type volumeRequirement struct {
	Stack     string
	Partition string
	Service   string
	Volumes   []string
	Roles     []string
}

func VolumePlacementWarnings(cfg *config.Config, partitionFilter string, debug bool) []string {
	deployment := cfg.Project.Deployment
	if deployment == "" {
		return nil
	}
	target, ok := cfg.Project.Targets[deployment]
	if !ok {
		return nil
	}
	if !hasVolumeRequirements(cfg, partitionFilter) {
		return nil
	}

	selected := selectDeploymentNodes(cfg.Project.Nodes, target)
	if len(selected) == 0 {
		warnings := []string{fmt.Sprintf("volume checks skipped: no deployment target nodes selected for %q", deployment)}
		if debug {
			warnings = append(warnings, formatTargetDebug(cfg, deployment, target))
		}
		return warnings
	}

	var warnings []string
	warnings = append(warnings, nodeVolumeLabelWarnings(cfg, selected)...)

	requirements := collectVolumeRequirements(cfg, partitionFilter)
	for _, req := range requirements {
		if len(req.Volumes) == 0 && len(req.Roles) == 0 {
			continue
		}
		required := uniqueSorted(req.Volumes)
		union := make(map[string]struct{})
		hasAll := false
		hasRoles := false
		for _, node := range selected {
			for _, name := range node.Volumes {
				union[name] = struct{}{}
			}
			if nodeHasVolumes(node, required) {
				hasAll = true
			}
			if nodeHasRoles(node, req.Roles) {
				hasRoles = true
			}
		}
		if len(required) > 0 {
			missing := missingVolumes(required, union)
			if len(missing) > 0 {
				warnings = append(warnings, fmt.Sprintf("volume check: missing volumes for %s: %s", ServiceScopeLabel(req.Stack, req.Partition, req.Service), strings.Join(missing, ", ")))
			}
			if !hasAll {
				warnings = append(warnings, fmt.Sprintf("volume check: no deployment target node provides all volumes for %s (required: %s)", ServiceScopeLabel(req.Stack, req.Partition, req.Service), strings.Join(required, ", ")))
			}
		}
		if len(req.Roles) > 0 && !hasRoles {
			warnings = append(warnings, fmt.Sprintf("volume check: no deployment target node satisfies required roles for %s (required: %s)", ServiceScopeLabel(req.Stack, req.Partition, req.Service), strings.Join(uniqueSorted(req.Roles), ", ")))
		}
	}

	return warnings
}

func RequiredVolumes(cfg *config.Config, partitionFilter string) map[string]struct{} {
	requirements := collectVolumeRequirements(cfg, partitionFilter)
	if len(requirements) == 0 {
		return nil
	}
	required := make(map[string]struct{})
	for _, req := range requirements {
		for _, name := range req.Volumes {
			if name == "" {
				continue
			}
			required[name] = struct{}{}
		}
	}
	return required
}

func formatTargetDebug(cfg *config.Config, deployment string, target config.DeploymentTarget) string {
	nodes := make([]string, 0, len(cfg.Project.Nodes))
	for name := range cfg.Project.Nodes {
		nodes = append(nodes, name)
	}
	sort.Strings(nodes)
	include := formatSelector(target.Include)
	exclude := formatSelector(target.Exclude)
	return fmt.Sprintf("volume checks debug: deployment=%s nodes=[%s] include={%s} exclude={%s}", deployment, strings.Join(nodes, ", "), include, exclude)
}

func formatSelector(selector config.NodeSelector) string {
	var parts []string
	if len(selector.Names) > 0 {
		parts = append(parts, "names="+strings.Join(selector.Names, "|"))
	}
	if len(selector.Labels) > 0 {
		labels := make([]string, 0, len(selector.Labels))
		for key, value := range selector.Labels {
			labels = append(labels, key+"="+value)
		}
		sort.Strings(labels)
		parts = append(parts, "labels="+strings.Join(labels, ","))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, " ")
}

func collectVolumeRequirements(cfg *config.Config, partitionFilter string) []volumeRequirement {
	var requirements []volumeRequirement
	serviceStandard := config.ServiceStandardName(cfg)
	for stackName, stack := range cfg.Stacks {
		partitions := []string{""}
		if stack.Mode == "partitioned" && len(cfg.Project.Partitions) > 0 {
			partitions = sliceutil.FilterPartition(cfg.Project.Partitions, partitionFilter)
		}
		for _, partition := range partitions {
			services, err := cfg.StackServices(stackName, partition)
			if err != nil {
				return nil
			}
			if len(services) == 0 {
				continue
			}
			for serviceName, service := range services {
				if len(service.Volumes) == 0 {
					continue
				}
				var names []string
				var roles []string
				for _, ref := range service.Volumes {
					if ref.Name == "" {
						if ref.Standard == "" {
							continue
						}
						if ref.Standard == serviceStandard {
							names = append(names, serviceVolumeName(stackName, stack.Mode, partition, serviceName))
							continue
						}
						if standard, ok := cfg.Project.Defaults.Volumes.Standards[ref.Standard]; ok {
							roles = append(roles, standard.Requires.Roles...)
						}
						continue
					}
					names = append(names, ref.Name)
				}
				requirements = append(requirements, volumeRequirement{
					Stack:     stackName,
					Partition: partition,
					Service:   serviceName,
					Volumes:   names,
					Roles:     roles,
				})
			}
		}
	}
	return requirements
}

func hasVolumeRequirements(cfg *config.Config, partitionFilter string) bool {
	serviceStandard := config.ServiceStandardName(cfg)
	for stackName, stack := range cfg.Stacks {
		partitions := []string{""}
		if stack.Mode == "partitioned" && len(cfg.Project.Partitions) > 0 {
			partitions = sliceutil.FilterPartition(cfg.Project.Partitions, partitionFilter)
		}
		for _, partition := range partitions {
			services, err := cfg.StackServices(stackName, partition)
			if err != nil {
				return false
			}
			if len(services) == 0 {
				continue
			}
			for _, service := range services {
				for _, ref := range service.Volumes {
					if ref.Name != "" || ref.Standard != "" {
						if ref.Standard != "" && ref.Standard != serviceStandard {
							return true
						}
						return true
					}
				}
			}
		}
	}
	return false
}

func serviceVolumeName(stackName string, stackMode string, partition string, serviceName string) string {
	resolved := stackName
	if stackMode == "partitioned" && partition != "" {
		resolved = partition + "_" + stackName
	}
	return resolved + "." + serviceName
}

func nodeVolumeLabelWarnings(cfg *config.Config, nodes map[string]config.NodeSpec) []string {
	labelKey := strings.TrimSpace(cfg.Project.Defaults.Volumes.NodeLabelKey)
	if labelKey == "" {
		return nil
	}
	var warnings []string
	for name, node := range nodes {
		for _, volume := range node.Volumes {
			label := labelKey + "." + volume
			if node.Labels[label] != "true" {
				warnings = append(warnings, fmt.Sprintf("volume check: node %q missing label %s=true for volume %q", name, label, volume))
			}
		}
	}
	return warnings
}

func nodeHasVolumes(node config.NodeSpec, required []string) bool {
	if len(required) == 0 {
		return true
	}
	if len(node.Volumes) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(node.Volumes))
	for _, name := range node.Volumes {
		set[name] = struct{}{}
	}
	for _, name := range required {
		if _, ok := set[name]; !ok {
			return false
		}
	}
	return true
}

func nodeHasRoles(node config.NodeSpec, required []string) bool {
	if len(required) == 0 {
		return true
	}
	roles := node.Roles
	if len(roles) == 0 {
		roles = []string{"worker"}
	}
	set := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		set[role] = struct{}{}
	}
	for _, role := range required {
		if _, ok := set[role]; !ok {
			return false
		}
	}
	return true
}

func missingVolumes(required []string, available map[string]struct{}) []string {
	var missing []string
	for _, name := range required {
		if _, ok := available[name]; !ok {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing
}

func uniqueSorted(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
