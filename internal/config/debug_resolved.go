package config

import (
	"fmt"
	"sort"

	"github.com/cmmoran/swarmcp/internal/yamlutil"
	"gopkg.in/yaml.v3"
)

func DebugResolvedMap(cfg *Config, partition string, stackFilters []string) (map[string]any, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	projectMap, err := structToMap(cfg.Project)
	if err != nil {
		return nil, err
	}
	projectMap["sources"] = resolvedProjectSources(cfg, partition)
	projectMap["configs"] = cfg.ProjectConfigDefs(partition)
	projectMap["secrets"] = cfg.ProjectSecretDefs(partition)
	if partition != "" {
		projectMap["partitions"] = []string{partition}
	}

	stackNames := selectedStackNames(cfg, stackFilters)
	stacksMap := make(map[string]any, len(stackNames))
	for _, stackName := range stackNames {
		stack, ok := cfg.Stacks[stackName]
		if !ok {
			continue
		}
		stackMap, err := structToMap(stack)
		if err != nil {
			return nil, err
		}
		delete(stackMap, "overlays")
		stackMap["sources"] = resolvedStackSources(cfg, stackName, partition)
		stackMap["configs"] = cfg.StackConfigDefs(stackName, partition)
		stackMap["secrets"] = cfg.StackSecretDefs(stackName, partition)

		services, err := cfg.StackServices(stackName, partition)
		if err != nil {
			return nil, err
		}
		serviceNames := make([]string, 0, len(services))
		for name := range services {
			serviceNames = append(serviceNames, name)
		}
		sort.Strings(serviceNames)
		serviceMap := make(map[string]any, len(serviceNames))
		for _, serviceName := range serviceNames {
			mapped, err := structToMap(services[serviceName])
			if err != nil {
				return nil, err
			}
			delete(mapped, "overlays")
			serviceMap[serviceName] = mapped
		}
		stackMap["services"] = serviceMap

		partitionsMap, err := debugResolvedPartitions(cfg, stackName, stack, partition)
		if err != nil {
			return nil, err
		}
		if len(partitionsMap) > 0 {
			stackMap["partitions"] = partitionsMap
		} else {
			delete(stackMap, "partitions")
		}

		stacksMap[stackName] = stackMap
	}

	return normalizeResolvedMap(map[string]any{
		"project": projectMap,
		"stacks":  stacksMap,
	})
}

func debugResolvedPartitions(cfg *Config, stackName string, stack Stack, partition string) (map[string]any, error) {
	if partition != "" {
		part, ok := stack.Partitions[partition]
		if !ok {
			return nil, nil
		}
		mapped, err := structToMap(part)
		if err != nil {
			return nil, err
		}
		mapped["sources"] = resolvedStackPartitionSources(cfg, stackName, partition)
		mapped["configs"] = cfg.StackPartitionConfigDefs(stackName, partition)
		mapped["secrets"] = cfg.StackPartitionSecretDefs(stackName, partition)
		return map[string]any{partition: mapped}, nil
	}

	names := cfg.StackPartitionNames(stackName)
	if len(names) == 0 {
		return nil, nil
	}
	partitions := make(map[string]any, len(names))
	for _, name := range names {
		part, ok := stack.Partitions[name]
		if !ok {
			part = StackPartition{}
		}
		mapped, err := structToMap(part)
		if err != nil {
			return nil, err
		}
		mapped["sources"] = resolvedStackPartitionSources(cfg, stackName, name)
		mapped["configs"] = cfg.StackPartitionConfigDefs(stackName, name)
		mapped["secrets"] = cfg.StackPartitionSecretDefs(stackName, name)
		partitions[name] = mapped
	}
	return partitions, nil
}

func selectedStackNames(cfg *Config, filters []string) []string {
	if len(filters) > 0 {
		out := append([]string(nil), filters...)
		sort.Strings(out)
		return out
	}
	out := make([]string, 0, len(cfg.Stacks))
	for name := range cfg.Stacks {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func structToMap(value any) (map[string]any, error) {
	encoded, err := yaml.Marshal(value)
	if err != nil {
		return nil, err
	}
	var out any
	if err := yaml.Unmarshal(encoded, &out); err != nil {
		return nil, err
	}
	normalized := yamlutil.NormalizeValue(out)
	if normalized == nil {
		return map[string]any{}, nil
	}
	mapped, ok := normalized.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("value is not a map")
	}
	return mapped, nil
}

func normalizeResolvedMap(value any) (map[string]any, error) {
	encoded, err := yaml.Marshal(value)
	if err != nil {
		return nil, err
	}
	var out any
	if err := yaml.Unmarshal(encoded, &out); err != nil {
		return nil, err
	}
	normalized := yamlutil.NormalizeValue(out)
	if normalized == nil {
		return map[string]any{}, nil
	}
	mapped, ok := normalized.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("resolved config is not a map")
	}
	return mapped, nil
}

func resolvedProjectSources(cfg *Config, partition string) Sources {
	base := cfg.Project.Sources
	deployOverlay := cfg.deploymentOverlay()
	projectDeploy := Sources{}
	projectDeploySealed := false
	if deployOverlay != nil {
		projectDeploy = deployOverlay.Project.Sources
		projectDeploySealed = deployOverlay.Project.Sealed
	}
	merged := mergeSources(base)
	if hasSources(projectDeploy) && !projectDeploySealed {
		merged = mergeSources(merged, projectDeploy)
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		if overlay.Project.Sealed || !hasSources(overlay.Project.Sources) {
			continue
		}
		merged = mergeSources(merged, overlay.Project.Sources)
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		if !overlay.Project.Sealed || !hasSources(overlay.Project.Sources) {
			continue
		}
		merged = mergeSources(merged, overlay.Project.Sources)
	}
	if hasSources(projectDeploy) && projectDeploySealed {
		merged = mergeSources(merged, projectDeploy)
	}
	return merged
}

func resolvedStackSources(cfg *Config, stackName string, partition string) Sources {
	stack, ok := cfg.Stacks[stackName]
	if !ok {
		return Sources{}
	}
	merged := mergeSources(stack.Sources)
	if overlay := cfg.deploymentOverlay(); overlay != nil {
		if stackOverlay := overlayStack(overlay, stackName); stackOverlay != nil && !stackOverlay.Sealed && hasSources(stackOverlay.Sources) {
			merged = mergeSources(merged, stackOverlay.Sources)
		}
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		if stackOverlay := overlayStack(&overlay, stackName); stackOverlay != nil && !stackOverlay.Sealed && hasSources(stackOverlay.Sources) {
			merged = mergeSources(merged, stackOverlay.Sources)
		}
	}
	if overlay := cfg.stackDeploymentOverlay(stackName); overlay != nil && !overlay.Sealed && hasSources(overlay.Sources) {
		merged = mergeSources(merged, overlay.Sources)
	}
	for _, overlay := range cfg.stackPartitionOverlays(stackName, partition) {
		if !overlay.Sealed && hasSources(overlay.Sources) {
			merged = mergeSources(merged, overlay.Sources)
		}
	}
	for _, overlay := range cfg.stackPartitionOverlays(stackName, partition) {
		if overlay.Sealed && hasSources(overlay.Sources) {
			merged = mergeSources(merged, overlay.Sources)
		}
	}
	if overlay := cfg.stackDeploymentOverlay(stackName); overlay != nil && overlay.Sealed && hasSources(overlay.Sources) {
		merged = mergeSources(merged, overlay.Sources)
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		if stackOverlay := overlayStack(&overlay, stackName); stackOverlay != nil && stackOverlay.Sealed && hasSources(stackOverlay.Sources) {
			merged = mergeSources(merged, stackOverlay.Sources)
		}
	}
	if overlay := cfg.deploymentOverlay(); overlay != nil {
		if stackOverlay := overlayStack(overlay, stackName); stackOverlay != nil && stackOverlay.Sealed && hasSources(stackOverlay.Sources) {
			merged = mergeSources(merged, stackOverlay.Sources)
		}
	}
	return merged
}

func resolvedStackPartitionSources(cfg *Config, stackName string, partition string) Sources {
	stack, ok := cfg.Stacks[stackName]
	if !ok {
		return Sources{}
	}
	base := Sources{}
	if part, ok := stack.Partitions[partition]; ok {
		base = part.Sources
	}
	merged := mergeSources(base)
	deployOverlay := cfg.deploymentOverlay()
	if deployOverlay != nil {
		if part := overlayStackPartition(deployOverlay, stackName, partition); part != nil && !part.Sealed && hasSources(part.Sources) {
			merged = mergeSources(merged, part.Sources)
		}
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		if part := overlayStackPartition(&overlay, stackName, partition); part != nil && !part.Sealed && hasSources(part.Sources) {
			merged = mergeSources(merged, part.Sources)
		}
	}
	if overlay := cfg.stackDeploymentOverlay(stackName); overlay != nil {
		if part, ok := overlay.Partitions[partition]; ok && !part.Sealed && hasSources(part.Sources) {
			merged = mergeSources(merged, part.Sources)
		}
	}
	for _, overlay := range cfg.stackPartitionOverlays(stackName, partition) {
		if part, ok := overlay.Partitions[partition]; ok && !part.Sealed && hasSources(part.Sources) {
			merged = mergeSources(merged, part.Sources)
		}
	}
	for _, overlay := range cfg.stackPartitionOverlays(stackName, partition) {
		if part, ok := overlay.Partitions[partition]; ok && part.Sealed && hasSources(part.Sources) {
			merged = mergeSources(merged, part.Sources)
		}
	}
	if overlay := cfg.stackDeploymentOverlay(stackName); overlay != nil {
		if part, ok := overlay.Partitions[partition]; ok && part.Sealed && hasSources(part.Sources) {
			merged = mergeSources(merged, part.Sources)
		}
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		if part := overlayStackPartition(&overlay, stackName, partition); part != nil && part.Sealed && hasSources(part.Sources) {
			merged = mergeSources(merged, part.Sources)
		}
	}
	if deployOverlay != nil {
		if part := overlayStackPartition(deployOverlay, stackName, partition); part != nil && part.Sealed && hasSources(part.Sources) {
			merged = mergeSources(merged, part.Sources)
		}
	}
	return merged
}

func mergeSources(base Sources, overlays ...Sources) Sources {
	merged := base
	for _, overlay := range overlays {
		if overlay.URL != "" {
			merged.URL = overlay.URL
		}
		if overlay.Ref != "" {
			merged.Ref = overlay.Ref
		}
		if overlay.Path != "" {
			merged.Path = overlay.Path
		}
		if overlay.Base != "" {
			merged.Base = overlay.Base
		}
	}
	return merged
}
