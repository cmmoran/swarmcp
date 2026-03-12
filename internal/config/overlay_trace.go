package config

import "strings"

func ResolvedOverlayLayers(cfg *Config, fieldPath []string, partition string) []ExplainLayer {
	if len(fieldPath) == 0 {
		return nil
	}
	switch fieldPath[0] {
	case "project":
		return explainProjectOverlayLayers(cfg, fieldPath[1:], partition)
	case "stacks":
		if len(fieldPath) < 2 {
			return nil
		}
		stackName := fieldPath[1]
		if len(fieldPath) >= 4 && fieldPath[2] == "services" {
			return explainServiceOverlayLayers(cfg, stackName, fieldPath[3], fieldPath[4:], partition)
		}
		return explainStackOverlayLayers(cfg, stackName, fieldPath[2:], partition)
	default:
		return nil
	}
}

func explainProjectOverlayLayers(cfg *Config, relPath []string, partition string) []ExplainLayer {
	var layers []ExplainLayer
	if overlay := cfg.deploymentOverlay(); overlay != nil {
		if value, ok := lookupStructPathValue(overlay.Project, relPath); ok {
			layers = append(layers, ExplainLayer{Label: "project deployment overlay", Value: value})
		}
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		if value, ok := lookupStructPathValue(overlay.Project, relPath); ok {
			layers = append(layers, ExplainLayer{Label: "project partition overlay", Value: value})
		}
	}
	return layers
}

func explainStackOverlayLayers(cfg *Config, stackName string, relPath []string, partition string) []ExplainLayer {
	var layers []ExplainLayer
	if overlay := cfg.deploymentOverlay(); overlay != nil {
		if stack := overlayStack(overlay, stackName); stack != nil {
			if value, ok := lookupStackOverlayValue(*stack, relPath); ok {
				layers = append(layers, ExplainLayer{Label: "project deployment overlay", Value: value})
			}
		}
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		if stack := overlayStack(&overlay, stackName); stack != nil {
			if value, ok := lookupStackOverlayValue(*stack, relPath); ok {
				layers = append(layers, ExplainLayer{Label: "project partition overlay", Value: value})
			}
		}
	}
	if overlay := cfg.stackDeploymentOverlay(stackName); overlay != nil {
		if value, ok := lookupStackOverlayValue(*overlay, relPath); ok {
			layers = append(layers, ExplainLayer{Label: "stack deployment overlay", Value: value})
		}
	}
	for _, overlay := range cfg.stackPartitionOverlays(stackName, partition) {
		if value, ok := lookupStackOverlayValue(overlay, relPath); ok {
			layers = append(layers, ExplainLayer{Label: "stack partition overlay", Value: value})
		}
	}
	return layers
}

func explainServiceOverlayLayers(cfg *Config, stackName string, serviceName string, relPath []string, partition string) []ExplainLayer {
	var layers []ExplainLayer
	if overlay := cfg.deploymentOverlay(); overlay != nil {
		if value, ok := lookupOverlayServiceValue(overlayStackServices(overlay, stackName), serviceName, relPath); ok {
			layers = append(layers, ExplainLayer{Label: "project deployment overlay", Value: value})
		}
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		if value, ok := lookupOverlayServiceValue(overlayStackServices(&overlay, stackName), serviceName, relPath); ok {
			layers = append(layers, ExplainLayer{Label: "project partition overlay", Value: value})
		}
	}
	if overlay := cfg.stackDeploymentOverlay(stackName); overlay != nil {
		if value, ok := lookupOverlayServiceValue(overlay.Services, serviceName, relPath); ok {
			layers = append(layers, ExplainLayer{Label: "stack deployment overlay", Value: value})
		}
	}
	for _, overlay := range cfg.stackPartitionOverlays(stackName, partition) {
		if value, ok := lookupOverlayServiceValue(overlay.Services, serviceName, relPath); ok {
			layers = append(layers, ExplainLayer{Label: "stack partition overlay", Value: value})
		}
	}
	if value, ok := lookupOverlayServiceValue(cfg.serviceDeploymentOverlays(stackName, false), serviceName, relPath); ok {
		layers = append(layers, ExplainLayer{Label: "service deployment overlay", Value: value})
	}
	for _, overlayMap := range cfg.servicePartitionOverlayMaps(stackName, partition, false) {
		if value, ok := lookupOverlayServiceValue(overlayMap, serviceName, relPath); ok {
			layers = append(layers, ExplainLayer{Label: "service partition overlay", Value: value})
		}
	}
	for _, overlayMap := range cfg.servicePartitionOverlayMaps(stackName, partition, true) {
		if value, ok := lookupOverlayServiceValue(overlayMap, serviceName, relPath); ok {
			layers = append(layers, ExplainLayer{Label: "service partition overlay", Value: value})
		}
	}
	if value, ok := lookupOverlayServiceValue(cfg.serviceDeploymentOverlays(stackName, true), serviceName, relPath); ok {
		layers = append(layers, ExplainLayer{Label: "service deployment overlay", Value: value})
	}
	return layers
}

func lookupOverlayServiceValue(overlays map[string]OverlayService, serviceName string, relPath []string) (any, bool) {
	if len(overlays) == 0 {
		return nil, false
	}
	overlay, ok := overlays[serviceName]
	if !ok {
		return nil, false
	}
	return lookupPathValue(overlay.Fields, relPath)
}

func lookupStackOverlayValue(overlay OverlayStack, relPath []string) (any, bool) {
	mapped, err := structToMap(overlay)
	if err != nil {
		return nil, false
	}
	delete(mapped, "services")
	return lookupPathValue(mapped, relPath)
}

func lookupStructPathValue(value any, relPath []string) (any, bool) {
	mapped, err := structToMap(value)
	if err != nil {
		return nil, false
	}
	return lookupPathValue(mapped, relPath)
}

func sourceLabel(path string) string {
	return strings.TrimSpace(path)
}
