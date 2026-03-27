package config

import (
	"fmt"
	"sort"

	"github.com/cmmoran/swarmcp/internal/yamlutil"
	"go.yaml.in/yaml/v4"
)

func DebugResolvedMap(cfg *Config, partition string, stackFilters []string) (map[string]any, error) {
	return debugResolvedMapWithTrace(cfg, partition, stackFilters, nil)
}

func debugResolvedMapWithTrace(cfg *Config, partition string, stackFilters []string, trace *LoadTrace) (map[string]any, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	allowedPartitions := resolvedModelPartitions(cfg, partition)
	projectMap, err := structToMap(cfg.Project)
	if err != nil {
		return nil, err
	}
	projectMap["sources"] = resolvedProjectSourcesWithTrace(cfg, partition, trace)
	projectMap["configs"] = cfg.projectConfigDefsWithTrace(partition, trace)
	projectMap["secrets"] = cfg.projectSecretDefsWithTrace(partition, trace)
	if partition != "" {
		projectMap["partitions"] = []string{partition}
	} else if len(allowedPartitions) > 0 {
		projectMap["partitions"] = allowedPartitions
	}

	stackNames := selectedStackNames(cfg, partition, stackFilters)
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
		stackMap["sources"] = resolvedStackSourcesWithTrace(cfg, stackName, partition, trace)
		stackMap["configs"] = cfg.stackConfigDefsWithTrace(stackName, partition, trace)
		stackMap["secrets"] = cfg.stackSecretDefsWithTrace(stackName, partition, trace)

		services, err := cfg.stackServicesWithTrace(stackName, partition, trace)
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

		partitionsMap, err := debugResolvedPartitions(cfg, stackName, stack, partition, allowedPartitions, trace)
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

func debugResolvedPartitions(cfg *Config, stackName string, stack Stack, partition string, allowedPartitions []string, trace *LoadTrace) (map[string]any, error) {
	if partition != "" {
		part, ok := stack.Partitions[partition]
		if !ok {
			return nil, nil
		}
		mapped, err := structToMap(part)
		if err != nil {
			return nil, err
		}
		mapped["sources"] = resolvedStackPartitionSourcesWithTrace(cfg, stackName, partition, trace)
		mapped["configs"] = cfg.stackPartitionConfigDefsWithTrace(stackName, partition, trace)
		mapped["secrets"] = cfg.stackPartitionSecretDefsWithTrace(stackName, partition, trace)
		return map[string]any{partition: mapped}, nil
	}

	names := cfg.StackPartitionNames(stackName)
	if len(allowedPartitions) > 0 {
		names = filterResolvedPartitionNames(names, allowedPartitions)
	}
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
		mapped["sources"] = resolvedStackPartitionSourcesWithTrace(cfg, stackName, name, trace)
		mapped["configs"] = cfg.stackPartitionConfigDefsWithTrace(stackName, name, trace)
		mapped["secrets"] = cfg.stackPartitionSecretDefsWithTrace(stackName, name, trace)
		partitions[name] = mapped
	}
	return partitions, nil
}

func resolvedModelPartitions(cfg *Config, selected string) []string {
	if selected != "" {
		return []string{selected}
	}
	partitions := append([]string(nil), cfg.Project.Partitions...)
	if cfg.Project.Deployment == "" {
		return partitions
	}
	target, ok := cfg.Project.Targets[cfg.Project.Deployment]
	if !ok || len(target.Partitions) == 0 {
		return partitions
	}
	return append([]string(nil), target.Partitions...)
}

func filterResolvedPartitionNames(names []string, allowed []string) []string {
	if len(names) == 0 || len(allowed) == 0 {
		return names
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = struct{}{}
	}
	filtered := make([]string, 0, len(names))
	for _, name := range names {
		if _, ok := allowedSet[name]; ok {
			filtered = append(filtered, name)
		}
	}
	return filtered
}

func selectedStackNames(cfg *Config, partition string, filters []string) []string {
	if len(filters) > 0 {
		out := make([]string, 0, len(filters))
		for _, name := range filters {
			if stackVisibleInResolvedModel(cfg, name, partition) {
				out = append(out, name)
			}
		}
		sort.Strings(out)
		return out
	}
	out := make([]string, 0, len(cfg.Stacks))
	for name := range cfg.Stacks {
		if stackVisibleInResolvedModel(cfg, name, partition) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func stackVisibleInResolvedModel(cfg *Config, stackName string, partition string) bool {
	stack, ok := cfg.Stacks[stackName]
	if !ok {
		return false
	}
	if partition != "" {
		if stack.Mode != "partitioned" {
			return cfg.StackIncludedInTarget(stackName, "")
		}
		return cfg.StackIncludedInTarget(stackName, partition)
	}
	return cfg.StackSelectedForRuntime(stackName, nil)
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
	return resolvedProjectSourcesWithTrace(cfg, partition, nil)
}

func resolvedProjectSourcesWithTrace(cfg *Config, partition string, trace *LoadTrace) Sources {
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
		recordProjectSourcesOverlayTrace(trace, "project deployment overlay", projectDeploy)
		merged = mergeSources(merged, projectDeploy)
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		if overlay.Project.Sealed || !hasSources(overlay.Project.Sources) {
			continue
		}
		recordProjectSourcesOverlayTrace(trace, "project partition overlay", overlay.Project.Sources)
		merged = mergeSources(merged, overlay.Project.Sources)
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		if !overlay.Project.Sealed || !hasSources(overlay.Project.Sources) {
			continue
		}
		recordProjectSourcesOverlayTrace(trace, "project partition overlay", overlay.Project.Sources)
		merged = mergeSources(merged, overlay.Project.Sources)
	}
	if hasSources(projectDeploy) && projectDeploySealed {
		recordProjectSourcesOverlayTrace(trace, "project deployment overlay", projectDeploy)
		merged = mergeSources(merged, projectDeploy)
	}
	return merged
}

func resolvedStackSources(cfg *Config, stackName string, partition string) Sources {
	return resolvedStackSourcesWithTrace(cfg, stackName, partition, nil)
}

func resolvedStackSourcesWithTrace(cfg *Config, stackName string, partition string, trace *LoadTrace) Sources {
	stack, ok := cfg.Stacks[stackName]
	if !ok {
		return Sources{}
	}
	merged := mergeSources(stack.Sources)
	if overlay := cfg.deploymentOverlay(); overlay != nil {
		if stackOverlay := overlayStack(overlay, stackName); stackOverlay != nil && !stackOverlay.Sealed && hasSources(stackOverlay.Sources) {
			recordStackSourcesOverlayTrace(trace, stackName, nil, "project deployment overlay", stackOverlay.Sources)
			merged = mergeSources(merged, stackOverlay.Sources)
		}
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		if stackOverlay := overlayStack(&overlay, stackName); stackOverlay != nil && !stackOverlay.Sealed && hasSources(stackOverlay.Sources) {
			recordStackSourcesOverlayTrace(trace, stackName, nil, "project partition overlay", stackOverlay.Sources)
			merged = mergeSources(merged, stackOverlay.Sources)
		}
	}
	if overlay := cfg.stackDeploymentOverlay(stackName); overlay != nil && !overlay.Sealed && hasSources(overlay.Sources) {
		recordStackSourcesOverlayTrace(trace, stackName, nil, "stack deployment overlay", overlay.Sources)
		merged = mergeSources(merged, overlay.Sources)
	}
	for _, overlay := range cfg.stackPartitionOverlays(stackName, partition) {
		if !overlay.Sealed && hasSources(overlay.Sources) {
			recordStackSourcesOverlayTrace(trace, stackName, nil, "stack partition overlay", overlay.Sources)
			merged = mergeSources(merged, overlay.Sources)
		}
	}
	for _, overlay := range cfg.stackPartitionOverlays(stackName, partition) {
		if overlay.Sealed && hasSources(overlay.Sources) {
			recordStackSourcesOverlayTrace(trace, stackName, nil, "stack partition overlay", overlay.Sources)
			merged = mergeSources(merged, overlay.Sources)
		}
	}
	if overlay := cfg.stackDeploymentOverlay(stackName); overlay != nil && overlay.Sealed && hasSources(overlay.Sources) {
		recordStackSourcesOverlayTrace(trace, stackName, nil, "stack deployment overlay", overlay.Sources)
		merged = mergeSources(merged, overlay.Sources)
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		if stackOverlay := overlayStack(&overlay, stackName); stackOverlay != nil && stackOverlay.Sealed && hasSources(stackOverlay.Sources) {
			recordStackSourcesOverlayTrace(trace, stackName, nil, "project partition overlay", stackOverlay.Sources)
			merged = mergeSources(merged, stackOverlay.Sources)
		}
	}
	if overlay := cfg.deploymentOverlay(); overlay != nil {
		if stackOverlay := overlayStack(overlay, stackName); stackOverlay != nil && stackOverlay.Sealed && hasSources(stackOverlay.Sources) {
			recordStackSourcesOverlayTrace(trace, stackName, nil, "project deployment overlay", stackOverlay.Sources)
			merged = mergeSources(merged, stackOverlay.Sources)
		}
	}
	return merged
}

func resolvedStackPartitionSources(cfg *Config, stackName string, partition string) Sources {
	return resolvedStackPartitionSourcesWithTrace(cfg, stackName, partition, nil)
}

func resolvedStackPartitionSourcesWithTrace(cfg *Config, stackName string, partition string, trace *LoadTrace) Sources {
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
			recordStackSourcesOverlayTrace(trace, stackName, []string{"partitions", partition}, "project deployment overlay", part.Sources)
			merged = mergeSources(merged, part.Sources)
		}
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		if part := overlayStackPartition(&overlay, stackName, partition); part != nil && !part.Sealed && hasSources(part.Sources) {
			recordStackSourcesOverlayTrace(trace, stackName, []string{"partitions", partition}, "project partition overlay", part.Sources)
			merged = mergeSources(merged, part.Sources)
		}
	}
	if overlay := cfg.stackDeploymentOverlay(stackName); overlay != nil {
		if part, ok := overlay.Partitions[partition]; ok && !part.Sealed && hasSources(part.Sources) {
			recordStackSourcesOverlayTrace(trace, stackName, []string{"partitions", partition}, "stack deployment overlay", part.Sources)
			merged = mergeSources(merged, part.Sources)
		}
	}
	for _, overlay := range cfg.stackPartitionOverlays(stackName, partition) {
		if part, ok := overlay.Partitions[partition]; ok && !part.Sealed && hasSources(part.Sources) {
			recordStackSourcesOverlayTrace(trace, stackName, []string{"partitions", partition}, "stack partition overlay", part.Sources)
			merged = mergeSources(merged, part.Sources)
		}
	}
	for _, overlay := range cfg.stackPartitionOverlays(stackName, partition) {
		if part, ok := overlay.Partitions[partition]; ok && part.Sealed && hasSources(part.Sources) {
			recordStackSourcesOverlayTrace(trace, stackName, []string{"partitions", partition}, "stack partition overlay", part.Sources)
			merged = mergeSources(merged, part.Sources)
		}
	}
	if overlay := cfg.stackDeploymentOverlay(stackName); overlay != nil {
		if part, ok := overlay.Partitions[partition]; ok && part.Sealed && hasSources(part.Sources) {
			recordStackSourcesOverlayTrace(trace, stackName, []string{"partitions", partition}, "stack deployment overlay", part.Sources)
			merged = mergeSources(merged, part.Sources)
		}
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		if part := overlayStackPartition(&overlay, stackName, partition); part != nil && part.Sealed && hasSources(part.Sources) {
			recordStackSourcesOverlayTrace(trace, stackName, []string{"partitions", partition}, "project partition overlay", part.Sources)
			merged = mergeSources(merged, part.Sources)
		}
	}
	if deployOverlay != nil {
		if part := overlayStackPartition(deployOverlay, stackName, partition); part != nil && part.Sealed && hasSources(part.Sources) {
			recordStackSourcesOverlayTrace(trace, stackName, []string{"partitions", partition}, "project deployment overlay", part.Sources)
			merged = mergeSources(merged, part.Sources)
		}
	}
	return merged
}

func recordProjectSourcesOverlayTrace(trace *LoadTrace, label string, sources Sources) {
	recordSourcesOverlayTrace(trace, []string{"project"}, label, sources)
}

func recordStackSourcesOverlayTrace(trace *LoadTrace, stackName string, extraPrefix []string, label string, sources Sources) {
	prefix := []string{"stacks", stackName}
	prefix = append(prefix, extraPrefix...)
	recordSourcesOverlayTrace(trace, prefix, label, sources)
}

func recordSourcesOverlayTrace(trace *LoadTrace, prefix []string, label string, sources Sources) {
	if trace == nil || !hasSources(sources) {
		return
	}
	path := append(append([]string(nil), prefix...), "sources")
	if len(trace.FieldPath) < len(path)+1 {
		return
	}
	for i := range path {
		if trace.FieldPath[i] != path[i] {
			return
		}
	}
	mapped, err := structToMap(sources)
	if err != nil {
		return
	}
	if value, ok := lookupPathValue(mapped, trace.FieldPath[len(path):]); ok {
		trace.recordOverlay(label, value)
	}
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
