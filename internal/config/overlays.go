package config

import (
	"fmt"
	"sort"

	"github.com/cmmoran/swarmcp/internal/yamlutil"
	"gopkg.in/yaml.v3"
)

type Overlays struct {
	Deployments map[string]Overlay `yaml:"deployments"`
	Partitions  map[string]Overlay `yaml:"partitions"`
}

type Overlay struct {
	Project OverlayProject          `yaml:"project"`
	Stacks  map[string]OverlayStack `yaml:"stacks"`
}

type OverlayProject struct {
	Sources Sources              `yaml:"sources"`
	Configs map[string]ConfigDef `yaml:"configs"`
	Secrets map[string]SecretDef `yaml:"secrets"`
}

type OverlayStack struct {
	Sources    Sources                     `yaml:"sources"`
	Configs    ConfigDefsOrRefs            `yaml:"configs"`
	Secrets    SecretDefsOrRefs            `yaml:"secrets"`
	Partitions map[string]OverlayPartition `yaml:"partitions"`
	Services   map[string]OverlayService   `yaml:"services"`
}

type OverlayPartition struct {
	Sources Sources              `yaml:"sources"`
	Configs ConfigDefsOrRefs     `yaml:"configs"`
	Secrets SecretDefsOrRefs     `yaml:"secrets"`
}

type OverlayService map[string]any

func (o *OverlayService) UnmarshalYAML(value *yaml.Node) error {
	var out any
	if err := value.Decode(&out); err != nil {
		return err
	}
	normalized := yamlutil.NormalizeValue(out)
	if normalized == nil {
		*o = OverlayService{}
		return nil
	}
	mapped, ok := normalized.(map[string]any)
	if !ok {
		return fmt.Errorf("service overlay must be a map")
	}
	*o = OverlayService(mapped)
	return nil
}

func (cfg *Config) ProjectConfigDefs(partition string) map[string]ConfigDef {
	deploy := overlayProjectConfigs(cfg.deploymentOverlay())
	part := overlayProjectConfigs(cfg.partitionOverlay(partition))
	return mergeConfigDefs(cfg.Project.Configs, deploy, part)
}

func (cfg *Config) ProjectSecretDefs(partition string) map[string]SecretDef {
	deploy := overlayProjectSecrets(cfg.deploymentOverlay())
	part := overlayProjectSecrets(cfg.partitionOverlay(partition))
	return applySecretDefDefaults(mergeSecretDefs(cfg.Project.Secrets, deploy, part))
}

func (cfg *Config) StackConfigDefs(stackName string, partition string) map[string]ConfigDef {
	base := cfg.stackConfigs(stackName)
	deploy := overlayStackConfigs(cfg.deploymentOverlay(), stackName)
	part := overlayStackConfigs(cfg.partitionOverlay(partition), stackName)
	return mergeConfigDefs(base, deploy, part)
}

func (cfg *Config) StackSecretDefs(stackName string, partition string) map[string]SecretDef {
	base := cfg.stackSecrets(stackName)
	deploy := overlayStackSecrets(cfg.deploymentOverlay(), stackName)
	part := overlayStackSecrets(cfg.partitionOverlay(partition), stackName)
	return applySecretDefDefaults(mergeSecretDefs(base, deploy, part))
}

func (cfg *Config) StackPartitionConfigDefs(stackName string, partition string) map[string]ConfigDef {
	base := cfg.stackPartitionConfigs(stackName, partition)
	deploy := overlayStackPartitionConfigs(cfg.deploymentOverlay(), stackName, partition)
	part := overlayStackPartitionConfigs(cfg.partitionOverlay(partition), stackName, partition)
	return mergeConfigDefs(base, deploy, part)
}

func (cfg *Config) StackPartitionSecretDefs(stackName string, partition string) map[string]SecretDef {
	base := cfg.stackPartitionSecrets(stackName, partition)
	deploy := overlayStackPartitionSecrets(cfg.deploymentOverlay(), stackName, partition)
	part := overlayStackPartitionSecrets(cfg.partitionOverlay(partition), stackName, partition)
	return applySecretDefDefaults(mergeSecretDefs(base, deploy, part))
}

func (cfg *Config) deploymentOverlay() *Overlay {
	if cfg.Project.Deployment == "" {
		return nil
	}
	overlay, ok := cfg.Overlays.Deployments[cfg.Project.Deployment]
	if !ok {
		return nil
	}
	return &overlay
}

func (cfg *Config) partitionOverlay(partition string) *Overlay {
	if partition == "" {
		return nil
	}
	overlay, ok := cfg.Overlays.Partitions[partition]
	if !ok {
		return nil
	}
	return &overlay
}

func overlayProjectConfigs(overlay *Overlay) map[string]ConfigDef {
	if overlay == nil {
		return nil
	}
	return overlay.Project.Configs
}

func overlayProjectSecrets(overlay *Overlay) map[string]SecretDef {
	if overlay == nil {
		return nil
	}
	return overlay.Project.Secrets
}

func overlayStackConfigs(overlay *Overlay, stackName string) map[string]ConfigDef {
	stack := overlayStack(overlay, stackName)
	if stack == nil {
		return nil
	}
	return stack.Configs.Defs
}

func overlayStackSecrets(overlay *Overlay, stackName string) map[string]SecretDef {
	stack := overlayStack(overlay, stackName)
	if stack == nil {
		return nil
	}
	return stack.Secrets.Defs
}

func overlayStackServices(overlay *Overlay, stackName string) map[string]OverlayService {
	stack := overlayStack(overlay, stackName)
	if stack == nil {
		return nil
	}
	return stack.Services
}

func overlayStackPartitionConfigs(overlay *Overlay, stackName string, partition string) map[string]ConfigDef {
	part := overlayStackPartition(overlay, stackName, partition)
	if part == nil {
		return nil
	}
	return part.Configs.Defs
}

func overlayStackPartitionSecrets(overlay *Overlay, stackName string, partition string) map[string]SecretDef {
	part := overlayStackPartition(overlay, stackName, partition)
	if part == nil {
		return nil
	}
	return part.Secrets.Defs
}

func overlayStack(overlay *Overlay, stackName string) *OverlayStack {
	if overlay == nil {
		return nil
	}
	stack, ok := overlay.Stacks[stackName]
	if !ok {
		return nil
	}
	return &stack
}

func overlayStackPartition(overlay *Overlay, stackName string, partition string) *OverlayPartition {
	stack := overlayStack(overlay, stackName)
	if stack == nil {
		return nil
	}
	part, ok := stack.Partitions[partition]
	if !ok {
		return nil
	}
	return &part
}

func mergeConfigDefs(base map[string]ConfigDef, overlays ...map[string]ConfigDef) map[string]ConfigDef {
	if len(overlays) == 0 {
		return base
	}
	hasOverlay := false
	for _, overlay := range overlays {
		if len(overlay) > 0 {
			hasOverlay = true
			break
		}
	}
	if !hasOverlay {
		return base
	}
	out := make(map[string]ConfigDef, len(base))
	for name, def := range base {
		out[name] = def
	}
	for _, overlay := range overlays {
		for name, def := range overlay {
			if existing, ok := out[name]; ok {
				out[name] = mergeConfigDef(existing, def)
				continue
			}
			out[name] = def
		}
	}
	return out
}

func mergeSecretDefs(base map[string]SecretDef, overlays ...map[string]SecretDef) map[string]SecretDef {
	if len(overlays) == 0 {
		return base
	}
	hasOverlay := false
	for _, overlay := range overlays {
		if len(overlay) > 0 {
			hasOverlay = true
			break
		}
	}
	if !hasOverlay {
		return base
	}
	out := make(map[string]SecretDef, len(base))
	for name, def := range base {
		out[name] = def
	}
	for _, overlay := range overlays {
		for name, def := range overlay {
			if existing, ok := out[name]; ok {
				out[name] = mergeSecretDef(existing, def)
				continue
			}
			out[name] = def
		}
	}
	return out
}

func mergeConfigDef(base ConfigDef, overlay ConfigDef) ConfigDef {
	if overlay.Source != "" {
		base.Source = overlay.Source
	}
	if overlay.Target != "" {
		base.Target = overlay.Target
	}
	if overlay.UID != "" {
		base.UID = overlay.UID
	}
	if overlay.GID != "" {
		base.GID = overlay.GID
	}
	if overlay.Mode != "" {
		base.Mode = overlay.Mode
	}
	return base
}

func mergeSecretDef(base SecretDef, overlay SecretDef) SecretDef {
	if overlay.Source != "" {
		base.Source = overlay.Source
	}
	if overlay.Target != "" {
		base.Target = overlay.Target
	}
	if overlay.UID != "" {
		base.UID = overlay.UID
	}
	if overlay.GID != "" {
		base.GID = overlay.GID
	}
	if overlay.Mode != "" {
		base.Mode = overlay.Mode
	}
	return base
}

func serviceToMap(service Service) (map[string]any, error) {
	encoded, err := yaml.Marshal(service)
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
		return nil, fmt.Errorf("service is not a map")
	}
	return mapped, nil
}

func mergeServiceOverlay(base Service, overlay OverlayService) (Service, error) {
	baseMap, err := serviceToMap(base)
	if err != nil {
		return Service{}, err
	}
	overlayMap := map[string]any(overlay)
	merged, err := mergeMaps(baseMap, overlayMap)
	if err != nil {
		return Service{}, err
	}
	return decodeServiceMap(merged)
}

func mergeServices(base map[string]Service, overlays ...map[string]OverlayService) (map[string]Service, error) {
	if len(overlays) == 0 {
		return base, nil
	}
	hasOverlay := false
	for _, overlay := range overlays {
		if len(overlay) > 0 {
			hasOverlay = true
			break
		}
	}
	if !hasOverlay {
		return base, nil
	}
	out := make(map[string]Service, len(base))
	for name, svc := range base {
		out[name] = svc
	}
	for _, overlay := range overlays {
		for name, svc := range overlay {
			if existing, ok := out[name]; ok {
				merged, err := mergeServiceOverlay(existing, svc)
				if err != nil {
					return nil, err
				}
				out[name] = merged
				continue
			}
			merged, err := decodeServiceMap(map[string]any(svc))
			if err != nil {
				return nil, err
			}
			out[name] = merged
		}
	}
	return out, nil
}

func (cfg *Config) stackConfigs(stackName string) map[string]ConfigDef {
	stack, ok := cfg.Stacks[stackName]
	if !ok {
		return nil
	}
	return stack.Configs.Defs
}

func (cfg *Config) stackSecrets(stackName string) map[string]SecretDef {
	stack, ok := cfg.Stacks[stackName]
	if !ok {
		return nil
	}
	return stack.Secrets.Defs
}

func (cfg *Config) stackServices(stackName string) map[string]Service {
	stack, ok := cfg.Stacks[stackName]
	if !ok {
		return nil
	}
	return stack.Services
}

func (cfg *Config) StackServices(stackName string, partition string) (map[string]Service, error) {
	base := cfg.stackServices(stackName)
	deploy := overlayStackServices(cfg.deploymentOverlay(), stackName)
	part := overlayStackServices(cfg.partitionOverlay(partition), stackName)
	return mergeServices(base, deploy, part)
}

func (cfg *Config) stackPartitionConfigs(stackName string, partition string) map[string]ConfigDef {
	stack, ok := cfg.Stacks[stackName]
	if !ok {
		return nil
	}
	part, ok := stack.Partitions[partition]
	if !ok {
		return nil
	}
	return part.Configs.Defs
}

func (cfg *Config) stackPartitionSecrets(stackName string, partition string) map[string]SecretDef {
	stack, ok := cfg.Stacks[stackName]
	if !ok {
		return nil
	}
	part, ok := stack.Partitions[partition]
	if !ok {
		return nil
	}
	return part.Secrets.Defs
}

func (cfg *Config) StackPartitionNames(stackName string) []string {
	seen := make(map[string]bool)
	if stack, ok := cfg.Stacks[stackName]; ok {
		for name := range stack.Partitions {
			seen[name] = true
		}
	}
	addOverlayPartitions := func(overlay *Overlay) {
		stack := overlayStack(overlay, stackName)
		if stack == nil {
			return
		}
		for name := range stack.Partitions {
			seen[name] = true
		}
	}
	addOverlayPartitions(cfg.deploymentOverlay())
	for _, overlay := range cfg.Overlays.Partitions {
		addOverlayPartitions(&overlay)
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
