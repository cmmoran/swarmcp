package config

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/cmmoran/swarmcp/internal/yamlutil"
	"github.com/dlclark/regexp2"
	"go.yaml.in/yaml/v4"
)

type Overlays struct {
	Deployments map[string]Overlay `yaml:"deployments"`
	Partitions  PartitionOverlays  `yaml:"partitions"`
}

type StackOverlays struct {
	Deployments map[string]OverlayStack `yaml:"deployments"`
	Partitions  StackPartitionOverlays  `yaml:"partitions"`
}

type ServiceOverlays struct {
	Deployments map[string]OverlayService `yaml:"deployments"`
	Partitions  ServicePartitionOverlays  `yaml:"partitions"`
}

type Overlay struct {
	Project OverlayProject          `yaml:"project"`
	Stacks  map[string]OverlayStack `yaml:"stacks"`
}

type OverlayMatch struct {
	Partition OverlayMatchPartition `yaml:"partition"`
}

type OverlayMatchPartition struct {
	Type    string `yaml:"type"`
	Pattern string `yaml:"pattern"`
}

func (m *OverlayMatchPartition) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case 0:
		return nil
	case yaml.ScalarNode:
		var out string
		if err := value.Decode(&out); err != nil {
			return err
		}
		m.Pattern = out
		return nil
	case yaml.MappingNode:
		type matchAlias OverlayMatchPartition
		var out matchAlias
		if err := value.Decode(&out); err != nil {
			return err
		}
		m.Type = out.Type
		m.Pattern = out.Pattern
		return nil
	default:
		return fmt.Errorf("match.partition must be a string or map")
	}
}

type PartitionOverlay struct {
	Name    string       `yaml:"name"`
	Match   OverlayMatch `yaml:"match"`
	Overlay `yaml:",inline"`
}

type PartitionOverlays struct {
	Rules []PartitionOverlay
}

type StackPartitionOverlay struct {
	Name         string       `yaml:"name"`
	Match        OverlayMatch `yaml:"match"`
	OverlayStack `yaml:",inline"`
}

type StackPartitionOverlays struct {
	Rules []StackPartitionOverlay
}

type ServicePartitionOverlay struct {
	Name    string
	Match   OverlayMatch
	Service OverlayService
}

type ServicePartitionOverlays struct {
	Rules []ServicePartitionOverlay
}

func (p *PartitionOverlays) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == 0 {
		return nil
	}
	switch value.Kind {
	case yaml.MappingNode:
		var raw map[string]Overlay
		if err := value.Decode(&raw); err != nil {
			return err
		}
		names := make([]string, 0, len(raw))
		for name := range raw {
			names = append(names, name)
		}
		sort.Strings(names)
		rules := make([]PartitionOverlay, 0, len(names))
		for _, name := range names {
			rules = append(rules, PartitionOverlay{
				Name:    name,
				Match:   OverlayMatch{Partition: OverlayMatchPartition{Pattern: name}},
				Overlay: raw[name],
			})
		}
		p.Rules = rules
		return nil
	case yaml.SequenceNode:
		var raw []PartitionOverlay
		if err := value.Decode(&raw); err != nil {
			return err
		}
		for i := range raw {
			if raw[i].Name == "" {
				raw[i].Name = fmt.Sprintf("rule_%d", i+1)
			}
		}
		p.Rules = raw
		return nil
	default:
		return fmt.Errorf("overlays.partitions must be a map or list")
	}
}

func (p PartitionOverlays) Matching(partition string) []PartitionOverlay {
	if len(p.Rules) == 0 || partition == "" {
		return nil
	}
	matched := make([]PartitionOverlay, 0, len(p.Rules))
	for _, rule := range p.Rules {
		if matchPartition(rule.Match.Partition, partition) {
			matched = append(matched, rule)
		}
	}
	return matched
}

func (p *StackPartitionOverlays) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == 0 {
		return nil
	}
	switch value.Kind {
	case yaml.MappingNode:
		var raw map[string]OverlayStack
		if err := value.Decode(&raw); err != nil {
			return err
		}
		names := make([]string, 0, len(raw))
		for name := range raw {
			names = append(names, name)
		}
		sort.Strings(names)
		rules := make([]StackPartitionOverlay, 0, len(names))
		for _, name := range names {
			rules = append(rules, StackPartitionOverlay{
				Name:         name,
				Match:        OverlayMatch{Partition: OverlayMatchPartition{Pattern: name}},
				OverlayStack: raw[name],
			})
		}
		p.Rules = rules
		return nil
	case yaml.SequenceNode:
		var raw []StackPartitionOverlay
		if err := value.Decode(&raw); err != nil {
			return err
		}
		for i := range raw {
			if raw[i].Name == "" {
				raw[i].Name = fmt.Sprintf("rule_%d", i+1)
			}
		}
		p.Rules = raw
		return nil
	default:
		return fmt.Errorf("overlays.partitions must be a map or list")
	}
}

func (p StackPartitionOverlays) Matching(partition string) []StackPartitionOverlay {
	if len(p.Rules) == 0 || partition == "" {
		return nil
	}
	matched := make([]StackPartitionOverlay, 0, len(p.Rules))
	for _, rule := range p.Rules {
		if matchPartition(rule.Match.Partition, partition) {
			matched = append(matched, rule)
		}
	}
	return matched
}

func (p *ServicePartitionOverlays) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == 0 {
		return nil
	}
	switch value.Kind {
	case yaml.MappingNode:
		var raw map[string]any
		if err := value.Decode(&raw); err != nil {
			return err
		}
		names := make([]string, 0, len(raw))
		for name := range raw {
			names = append(names, name)
		}
		sort.Strings(names)
		rules := make([]ServicePartitionOverlay, 0, len(names))
		for _, name := range names {
			val, ok := raw[name]
			if !ok {
				continue
			}
			service, err := overlayServiceFromValue(val)
			if err != nil {
				return err
			}
			rules = append(rules, ServicePartitionOverlay{
				Name:    name,
				Match:   OverlayMatch{Partition: OverlayMatchPartition{Pattern: name}},
				Service: service,
			})
		}
		p.Rules = rules
		return nil
	case yaml.SequenceNode:
		var raw []map[string]any
		if err := value.Decode(&raw); err != nil {
			return err
		}
		rules := make([]ServicePartitionOverlay, 0, len(raw))
		for i, rule := range raw {
			name := ""
			if rawName, ok := rule["name"]; ok {
				if val, ok := rawName.(string); ok {
					name = val
				}
			}
			match, err := overlayMatchFromValue(rule["match"])
			if err != nil {
				return err
			}
			delete(rule, "name")
			delete(rule, "match")
			service, err := overlayServiceFromValue(rule)
			if err != nil {
				return err
			}
			if name == "" {
				name = fmt.Sprintf("rule_%d", i+1)
			}
			rules = append(rules, ServicePartitionOverlay{
				Name:    name,
				Match:   match,
				Service: service,
			})
		}
		p.Rules = rules
		return nil
	default:
		return fmt.Errorf("overlays.partitions must be a map or list")
	}
}

func (p ServicePartitionOverlays) Matching(partition string) []ServicePartitionOverlay {
	if len(p.Rules) == 0 || partition == "" {
		return nil
	}
	matched := make([]ServicePartitionOverlay, 0, len(p.Rules))
	for _, rule := range p.Rules {
		if matchPartition(rule.Match.Partition, partition) {
			matched = append(matched, rule)
		}
	}
	return matched
}

type OverlayProject struct {
	Sealed  bool                 `yaml:"sealed"`
	Sources Sources              `yaml:"sources"`
	Configs map[string]ConfigDef `yaml:"configs"`
	Secrets map[string]SecretDef `yaml:"secrets"`
}

type OverlayStack struct {
	Sealed     bool                        `yaml:"sealed"`
	IncludedIn []InclusionRule             `yaml:"included_in"`
	Sources    Sources                     `yaml:"sources"`
	Configs    ConfigDefsOrRefs            `yaml:"configs"`
	Secrets    SecretDefsOrRefs            `yaml:"secrets"`
	Partitions map[string]OverlayPartition `yaml:"partitions"`
	Services   map[string]OverlayService   `yaml:"services"`
}

type OverlayPartition struct {
	Sealed  bool             `yaml:"sealed"`
	Sources Sources          `yaml:"sources"`
	Configs ConfigDefsOrRefs `yaml:"configs"`
	Secrets SecretDefsOrRefs `yaml:"secrets"`
}

type OverlayService struct {
	Sealed bool
	Fields map[string]any
}

func (o *OverlayService) UnmarshalYAML(value *yaml.Node) error {
	var out any
	if err := value.Decode(&out); err != nil {
		return err
	}
	overlay, err := overlayServiceFromValue(out)
	if err != nil {
		return err
	}
	*o = overlay
	return nil
}

func overlayServiceFromValue(value any) (OverlayService, error) {
	normalized := yamlutil.NormalizeValue(value)
	if normalized == nil {
		return OverlayService{}, nil
	}
	mapped, ok := normalized.(map[string]any)
	if !ok {
		return OverlayService{}, fmt.Errorf("service overlay must be a map")
	}
	return overlayServiceFromMap(mapped)
}

func overlayServiceFromMap(mapped map[string]any) (OverlayService, error) {
	if mapped == nil {
		return OverlayService{}, nil
	}
	overlay := OverlayService{
		Fields: map[string]any{},
	}
	if sealed, ok := mapped["sealed"]; ok {
		val, ok := sealed.(bool)
		if !ok {
			return OverlayService{}, fmt.Errorf("service overlay sealed must be a bool")
		}
		overlay.Sealed = val
		delete(mapped, "sealed")
	}
	for key, val := range mapped {
		overlay.Fields[key] = val
	}
	return overlay, nil
}

func overlayMatchFromValue(value any) (OverlayMatch, error) {
	if value == nil {
		return OverlayMatch{}, nil
	}
	encoded, err := yaml.Marshal(value)
	if err != nil {
		return OverlayMatch{}, err
	}
	var out OverlayMatch
	if err := yaml.Unmarshal(encoded, &out); err != nil {
		return OverlayMatch{}, err
	}
	return out, nil
}

func (cfg *Config) ProjectConfigDefs(partition string) map[string]ConfigDef {
	return cfg.projectConfigDefsWithTrace(partition, nil)
}

func (cfg *Config) projectConfigDefsWithTrace(partition string, trace *LoadTrace) map[string]ConfigDef {
	deployDefs, deploySealed := overlayProjectConfigs(cfg.deploymentOverlay())
	overlays := cfg.partitionOverlays(partition)
	merged := mergeConfigDefs(cfg.Project.Configs)
	if len(deployDefs) > 0 && !deploySealed {
		recordConfigDefOverlayTrace(trace, []string{"project"}, "project deployment overlay", deployDefs)
		merged = mergeConfigDefs(merged, deployDefs)
	}
	for _, overlay := range overlays {
		defs, sealed := overlayProjectConfigs(&overlay)
		if len(defs) == 0 || sealed {
			continue
		}
		recordConfigDefOverlayTrace(trace, []string{"project"}, "project partition overlay", defs)
		merged = mergeConfigDefs(merged, defs)
	}
	for _, overlay := range overlays {
		defs, sealed := overlayProjectConfigs(&overlay)
		if len(defs) == 0 || !sealed {
			continue
		}
		recordConfigDefOverlayTrace(trace, []string{"project"}, "project partition overlay", defs)
		merged = mergeConfigDefs(merged, defs)
	}
	if len(deployDefs) > 0 && deploySealed {
		recordConfigDefOverlayTrace(trace, []string{"project"}, "project deployment overlay", deployDefs)
		merged = mergeConfigDefs(merged, deployDefs)
	}
	return merged
}

func (cfg *Config) ProjectSecretDefs(partition string) map[string]SecretDef {
	return cfg.projectSecretDefsWithTrace(partition, nil)
}

func (cfg *Config) projectSecretDefsWithTrace(partition string, trace *LoadTrace) map[string]SecretDef {
	deployDefs, deploySealed := overlayProjectSecrets(cfg.deploymentOverlay())
	merged := mergeSecretDefs(cfg.Project.Secrets)
	if len(deployDefs) > 0 && !deploySealed {
		recordSecretDefOverlayTrace(trace, []string{"project"}, "project deployment overlay", deployDefs)
		merged = mergeSecretDefs(merged, deployDefs)
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		defs, sealed := overlayProjectSecrets(&overlay)
		if len(defs) == 0 || sealed {
			continue
		}
		recordSecretDefOverlayTrace(trace, []string{"project"}, "project partition overlay", defs)
		merged = mergeSecretDefs(merged, defs)
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		defs, sealed := overlayProjectSecrets(&overlay)
		if len(defs) == 0 || !sealed {
			continue
		}
		recordSecretDefOverlayTrace(trace, []string{"project"}, "project partition overlay", defs)
		merged = mergeSecretDefs(merged, defs)
	}
	if len(deployDefs) > 0 && deploySealed {
		recordSecretDefOverlayTrace(trace, []string{"project"}, "project deployment overlay", deployDefs)
		merged = mergeSecretDefs(merged, deployDefs)
	}
	return applySecretDefDefaults(merged)
}

func (cfg *Config) StackConfigDefs(stackName string, partition string) map[string]ConfigDef {
	return cfg.stackConfigDefsWithTrace(stackName, partition, nil)
}

func (cfg *Config) stackConfigDefsWithTrace(stackName string, partition string, trace *LoadTrace) map[string]ConfigDef {
	base := cfg.stackConfigs(stackName)
	deployDefs, deploySealed := overlayStackConfigs(cfg.deploymentOverlay(), stackName)
	stackDeployDefs, stackDeploySealed := overlayStackConfigsFromStack(cfg.stackDeploymentOverlay(stackName))
	merged := mergeConfigDefs(base)
	if len(deployDefs) > 0 && !deploySealed {
		recordConfigDefOverlayTrace(trace, []string{"stacks", stackName}, "project deployment overlay", deployDefs)
		merged = mergeConfigDefs(merged, deployDefs)
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		defs, sealed := overlayStackConfigs(&overlay, stackName)
		if len(defs) == 0 || sealed {
			continue
		}
		recordConfigDefOverlayTrace(trace, []string{"stacks", stackName}, "project partition overlay", defs)
		merged = mergeConfigDefs(merged, defs)
	}
	if len(stackDeployDefs) > 0 && !stackDeploySealed {
		recordConfigDefOverlayTrace(trace, []string{"stacks", stackName}, "stack deployment overlay", stackDeployDefs)
		merged = mergeConfigDefs(merged, stackDeployDefs)
	}
	for _, overlay := range cfg.stackPartitionOverlays(stackName, partition) {
		defs, sealed := overlayStackConfigsFromStack(&overlay)
		if len(defs) == 0 || sealed {
			continue
		}
		recordConfigDefOverlayTrace(trace, []string{"stacks", stackName}, "stack partition overlay", defs)
		merged = mergeConfigDefs(merged, defs)
	}
	for _, overlay := range cfg.stackPartitionOverlays(stackName, partition) {
		defs, sealed := overlayStackConfigsFromStack(&overlay)
		if len(defs) == 0 || !sealed {
			continue
		}
		recordConfigDefOverlayTrace(trace, []string{"stacks", stackName}, "stack partition overlay", defs)
		merged = mergeConfigDefs(merged, defs)
	}
	if len(stackDeployDefs) > 0 && stackDeploySealed {
		recordConfigDefOverlayTrace(trace, []string{"stacks", stackName}, "stack deployment overlay", stackDeployDefs)
		merged = mergeConfigDefs(merged, stackDeployDefs)
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		defs, sealed := overlayStackConfigs(&overlay, stackName)
		if len(defs) == 0 || !sealed {
			continue
		}
		recordConfigDefOverlayTrace(trace, []string{"stacks", stackName}, "project partition overlay", defs)
		merged = mergeConfigDefs(merged, defs)
	}
	if len(deployDefs) > 0 && deploySealed {
		recordConfigDefOverlayTrace(trace, []string{"stacks", stackName}, "project deployment overlay", deployDefs)
		merged = mergeConfigDefs(merged, deployDefs)
	}
	return merged
}

func (cfg *Config) StackSecretDefs(stackName string, partition string) map[string]SecretDef {
	return cfg.stackSecretDefsWithTrace(stackName, partition, nil)
}

func (cfg *Config) stackSecretDefsWithTrace(stackName string, partition string, trace *LoadTrace) map[string]SecretDef {
	base := cfg.stackSecrets(stackName)
	deployDefs, deploySealed := overlayStackSecrets(cfg.deploymentOverlay(), stackName)
	stackDeployDefs, stackDeploySealed := overlayStackSecretsFromStack(cfg.stackDeploymentOverlay(stackName))
	merged := mergeSecretDefs(base)
	if len(deployDefs) > 0 && !deploySealed {
		recordSecretDefOverlayTrace(trace, []string{"stacks", stackName}, "project deployment overlay", deployDefs)
		merged = mergeSecretDefs(merged, deployDefs)
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		defs, sealed := overlayStackSecrets(&overlay, stackName)
		if len(defs) == 0 || sealed {
			continue
		}
		recordSecretDefOverlayTrace(trace, []string{"stacks", stackName}, "project partition overlay", defs)
		merged = mergeSecretDefs(merged, defs)
	}
	if len(stackDeployDefs) > 0 && !stackDeploySealed {
		recordSecretDefOverlayTrace(trace, []string{"stacks", stackName}, "stack deployment overlay", stackDeployDefs)
		merged = mergeSecretDefs(merged, stackDeployDefs)
	}
	for _, overlay := range cfg.stackPartitionOverlays(stackName, partition) {
		defs, sealed := overlayStackSecretsFromStack(&overlay)
		if len(defs) == 0 || sealed {
			continue
		}
		recordSecretDefOverlayTrace(trace, []string{"stacks", stackName}, "stack partition overlay", defs)
		merged = mergeSecretDefs(merged, defs)
	}
	for _, overlay := range cfg.stackPartitionOverlays(stackName, partition) {
		defs, sealed := overlayStackSecretsFromStack(&overlay)
		if len(defs) == 0 || !sealed {
			continue
		}
		recordSecretDefOverlayTrace(trace, []string{"stacks", stackName}, "stack partition overlay", defs)
		merged = mergeSecretDefs(merged, defs)
	}
	if len(stackDeployDefs) > 0 && stackDeploySealed {
		recordSecretDefOverlayTrace(trace, []string{"stacks", stackName}, "stack deployment overlay", stackDeployDefs)
		merged = mergeSecretDefs(merged, stackDeployDefs)
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		defs, sealed := overlayStackSecrets(&overlay, stackName)
		if len(defs) == 0 || !sealed {
			continue
		}
		recordSecretDefOverlayTrace(trace, []string{"stacks", stackName}, "project partition overlay", defs)
		merged = mergeSecretDefs(merged, defs)
	}
	if len(deployDefs) > 0 && deploySealed {
		recordSecretDefOverlayTrace(trace, []string{"stacks", stackName}, "project deployment overlay", deployDefs)
		merged = mergeSecretDefs(merged, deployDefs)
	}
	return applySecretDefDefaults(merged)
}

func (cfg *Config) StackPartitionConfigDefs(stackName string, partition string) map[string]ConfigDef {
	return cfg.stackPartitionConfigDefsWithTrace(stackName, partition, nil)
}

func (cfg *Config) stackPartitionConfigDefsWithTrace(stackName string, partition string, trace *LoadTrace) map[string]ConfigDef {
	base := cfg.stackPartitionConfigs(stackName, partition)
	deployDefs, deploySealed := overlayStackPartitionConfigs(cfg.deploymentOverlay(), stackName, partition)
	stackDeployDefs, stackDeploySealed := overlayStackPartitionConfigsFromStack(cfg.stackDeploymentOverlay(stackName), partition)
	merged := mergeConfigDefs(base)
	if len(deployDefs) > 0 && !deploySealed {
		recordConfigDefOverlayTrace(trace, []string{"stacks", stackName, "partitions", partition}, "project deployment overlay", deployDefs)
		merged = mergeConfigDefs(merged, deployDefs)
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		defs, sealed := overlayStackPartitionConfigs(&overlay, stackName, partition)
		if len(defs) == 0 || sealed {
			continue
		}
		recordConfigDefOverlayTrace(trace, []string{"stacks", stackName, "partitions", partition}, "project partition overlay", defs)
		merged = mergeConfigDefs(merged, defs)
	}
	if len(stackDeployDefs) > 0 && !stackDeploySealed {
		recordConfigDefOverlayTrace(trace, []string{"stacks", stackName, "partitions", partition}, "stack deployment overlay", stackDeployDefs)
		merged = mergeConfigDefs(merged, stackDeployDefs)
	}
	for _, overlay := range cfg.stackPartitionOverlays(stackName, partition) {
		defs, sealed := overlayStackPartitionConfigsFromStack(&overlay, partition)
		if len(defs) == 0 || sealed {
			continue
		}
		recordConfigDefOverlayTrace(trace, []string{"stacks", stackName, "partitions", partition}, "stack partition overlay", defs)
		merged = mergeConfigDefs(merged, defs)
	}
	for _, overlay := range cfg.stackPartitionOverlays(stackName, partition) {
		defs, sealed := overlayStackPartitionConfigsFromStack(&overlay, partition)
		if len(defs) == 0 || !sealed {
			continue
		}
		recordConfigDefOverlayTrace(trace, []string{"stacks", stackName, "partitions", partition}, "stack partition overlay", defs)
		merged = mergeConfigDefs(merged, defs)
	}
	if len(stackDeployDefs) > 0 && stackDeploySealed {
		recordConfigDefOverlayTrace(trace, []string{"stacks", stackName, "partitions", partition}, "stack deployment overlay", stackDeployDefs)
		merged = mergeConfigDefs(merged, stackDeployDefs)
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		defs, sealed := overlayStackPartitionConfigs(&overlay, stackName, partition)
		if len(defs) == 0 || !sealed {
			continue
		}
		recordConfigDefOverlayTrace(trace, []string{"stacks", stackName, "partitions", partition}, "project partition overlay", defs)
		merged = mergeConfigDefs(merged, defs)
	}
	if len(deployDefs) > 0 && deploySealed {
		recordConfigDefOverlayTrace(trace, []string{"stacks", stackName, "partitions", partition}, "project deployment overlay", deployDefs)
		merged = mergeConfigDefs(merged, deployDefs)
	}
	return merged
}

func (cfg *Config) StackPartitionSecretDefs(stackName string, partition string) map[string]SecretDef {
	return cfg.stackPartitionSecretDefsWithTrace(stackName, partition, nil)
}

func (cfg *Config) stackPartitionSecretDefsWithTrace(stackName string, partition string, trace *LoadTrace) map[string]SecretDef {
	base := cfg.stackPartitionSecrets(stackName, partition)
	deployDefs, deploySealed := overlayStackPartitionSecrets(cfg.deploymentOverlay(), stackName, partition)
	stackDeployDefs, stackDeploySealed := overlayStackPartitionSecretsFromStack(cfg.stackDeploymentOverlay(stackName), partition)
	merged := mergeSecretDefs(base)
	if len(deployDefs) > 0 && !deploySealed {
		recordSecretDefOverlayTrace(trace, []string{"stacks", stackName, "partitions", partition}, "project deployment overlay", deployDefs)
		merged = mergeSecretDefs(merged, deployDefs)
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		defs, sealed := overlayStackPartitionSecrets(&overlay, stackName, partition)
		if len(defs) == 0 || sealed {
			continue
		}
		recordSecretDefOverlayTrace(trace, []string{"stacks", stackName, "partitions", partition}, "project partition overlay", defs)
		merged = mergeSecretDefs(merged, defs)
	}
	if len(stackDeployDefs) > 0 && !stackDeploySealed {
		recordSecretDefOverlayTrace(trace, []string{"stacks", stackName, "partitions", partition}, "stack deployment overlay", stackDeployDefs)
		merged = mergeSecretDefs(merged, stackDeployDefs)
	}
	for _, overlay := range cfg.stackPartitionOverlays(stackName, partition) {
		defs, sealed := overlayStackPartitionSecretsFromStack(&overlay, partition)
		if len(defs) == 0 || sealed {
			continue
		}
		recordSecretDefOverlayTrace(trace, []string{"stacks", stackName, "partitions", partition}, "stack partition overlay", defs)
		merged = mergeSecretDefs(merged, defs)
	}
	for _, overlay := range cfg.stackPartitionOverlays(stackName, partition) {
		defs, sealed := overlayStackPartitionSecretsFromStack(&overlay, partition)
		if len(defs) == 0 || !sealed {
			continue
		}
		recordSecretDefOverlayTrace(trace, []string{"stacks", stackName, "partitions", partition}, "stack partition overlay", defs)
		merged = mergeSecretDefs(merged, defs)
	}
	if len(stackDeployDefs) > 0 && stackDeploySealed {
		recordSecretDefOverlayTrace(trace, []string{"stacks", stackName, "partitions", partition}, "stack deployment overlay", stackDeployDefs)
		merged = mergeSecretDefs(merged, stackDeployDefs)
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		defs, sealed := overlayStackPartitionSecrets(&overlay, stackName, partition)
		if len(defs) == 0 || !sealed {
			continue
		}
		recordSecretDefOverlayTrace(trace, []string{"stacks", stackName, "partitions", partition}, "project partition overlay", defs)
		merged = mergeSecretDefs(merged, defs)
	}
	if len(deployDefs) > 0 && deploySealed {
		recordSecretDefOverlayTrace(trace, []string{"stacks", stackName, "partitions", partition}, "project deployment overlay", deployDefs)
		merged = mergeSecretDefs(merged, deployDefs)
	}
	return applySecretDefDefaults(merged)
}

func recordConfigDefOverlayTrace(trace *LoadTrace, prefix []string, label string, defs map[string]ConfigDef) {
	recordDefinitionOverlayTrace(trace, prefix, "configs", label, len(defs), func(name string) (map[string]any, bool) {
		def, ok := defs[name]
		if !ok {
			return nil, false
		}
		mapped, err := structToMap(def)
		if err != nil {
			return nil, false
		}
		return mapped, true
	})
}

func recordSecretDefOverlayTrace(trace *LoadTrace, prefix []string, label string, defs map[string]SecretDef) {
	recordDefinitionOverlayTrace(trace, prefix, "secrets", label, len(defs), func(name string) (map[string]any, bool) {
		def, ok := defs[name]
		if !ok {
			return nil, false
		}
		mapped, err := structToMap(def)
		if err != nil {
			return nil, false
		}
		return mapped, true
	})
}

func recordDefinitionOverlayTrace(trace *LoadTrace, prefix []string, kind string, label string, count int, lookup func(name string) (map[string]any, bool)) {
	if trace == nil || count == 0 {
		return
	}
	basePath := append(append([]string(nil), prefix...), kind)
	if len(trace.FieldPath) < len(basePath)+2 {
		return
	}
	for i := range basePath {
		if trace.FieldPath[i] != basePath[i] {
			return
		}
	}
	name := trace.FieldPath[len(basePath)]
	mapped, ok := lookup(name)
	if !ok {
		return
	}
	if value, ok := lookupPathValue(mapped, trace.FieldPath[len(basePath)+1:]); ok {
		trace.recordOverlay(label, value)
	}
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

func (cfg *Config) partitionOverlays(partition string) []Overlay {
	if partition == "" {
		return nil
	}
	matches := cfg.Overlays.Partitions.Matching(partition)
	if len(matches) == 0 {
		return nil
	}
	out := make([]Overlay, 0, len(matches))
	for _, match := range matches {
		out = append(out, match.Overlay)
	}
	return out
}

func (cfg *Config) stackDeploymentOverlay(stackName string) *OverlayStack {
	if cfg.Project.Deployment == "" {
		return nil
	}
	stack, ok := cfg.Stacks[stackName]
	if !ok {
		return nil
	}
	overlay, ok := stack.Overlays.Deployments[cfg.Project.Deployment]
	if !ok {
		return nil
	}
	return &overlay
}

func (cfg *Config) stackPartitionOverlays(stackName string, partition string) []OverlayStack {
	if partition == "" {
		return nil
	}
	stack, ok := cfg.Stacks[stackName]
	if !ok {
		return nil
	}
	matches := stack.Overlays.Partitions.Matching(partition)
	if len(matches) == 0 {
		return nil
	}
	out := make([]OverlayStack, 0, len(matches))
	for _, match := range matches {
		out = append(out, match.OverlayStack)
	}
	return out
}

func (cfg *Config) serviceDeploymentOverlays(stackName string, sealed bool) map[string]OverlayService {
	if cfg.Project.Deployment == "" {
		return nil
	}
	stack, ok := cfg.Stacks[stackName]
	if !ok {
		return nil
	}
	out := map[string]OverlayService{}
	for serviceName, service := range stack.Services {
		overlay, ok := service.Overlays.Deployments[cfg.Project.Deployment]
		if !ok {
			continue
		}
		if overlay.Sealed != sealed {
			continue
		}
		out[serviceName] = overlay
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (cfg *Config) servicePartitionOverlayMaps(stackName string, partition string, sealed bool) []map[string]OverlayService {
	if partition == "" {
		return nil
	}
	stack, ok := cfg.Stacks[stackName]
	if !ok {
		return nil
	}
	names := make([]string, 0, len(stack.Services))
	for name := range stack.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	var out []map[string]OverlayService
	for _, name := range names {
		service := stack.Services[name]
		for _, match := range service.Overlays.Partitions.Matching(partition) {
			if match.Service.Sealed != sealed {
				continue
			}
			if out == nil {
				out = []map[string]OverlayService{}
			}
			out = append(out, map[string]OverlayService{
				name: match.Service,
			})
		}
	}
	return out
}

func overlayProjectConfigs(overlay *Overlay) (map[string]ConfigDef, bool) {
	if overlay == nil {
		return nil, false
	}
	return overlay.Project.Configs, overlay.Project.Sealed
}

func overlayProjectSecrets(overlay *Overlay) (map[string]SecretDef, bool) {
	if overlay == nil {
		return nil, false
	}
	return overlay.Project.Secrets, overlay.Project.Sealed
}

func overlayStackConfigs(overlay *Overlay, stackName string) (map[string]ConfigDef, bool) {
	stack := overlayStack(overlay, stackName)
	if stack == nil {
		return nil, false
	}
	return stack.Configs.Defs, stack.Sealed
}

func overlayStackConfigsFromStack(overlay *OverlayStack) (map[string]ConfigDef, bool) {
	if overlay == nil {
		return nil, false
	}
	return overlay.Configs.Defs, overlay.Sealed
}

func overlayStackSecrets(overlay *Overlay, stackName string) (map[string]SecretDef, bool) {
	stack := overlayStack(overlay, stackName)
	if stack == nil {
		return nil, false
	}
	return stack.Secrets.Defs, stack.Sealed
}

func overlayStackSecretsFromStack(overlay *OverlayStack) (map[string]SecretDef, bool) {
	if overlay == nil {
		return nil, false
	}
	return overlay.Secrets.Defs, overlay.Sealed
}

func overlayStackServices(overlay *Overlay, stackName string) map[string]OverlayService {
	stack := overlayStack(overlay, stackName)
	if stack == nil {
		return nil
	}
	return stack.Services
}

func splitOverlayServices(overlay *Overlay, stackName string) (map[string]OverlayService, map[string]OverlayService) {
	stack := overlayStack(overlay, stackName)
	if stack == nil {
		return nil, nil
	}
	return splitOverlayServiceMap(stack.Services, stack.Sealed)
}

func splitOverlayStackServices(stack *OverlayStack) (map[string]OverlayService, map[string]OverlayService) {
	if stack == nil {
		return nil, nil
	}
	return splitOverlayServiceMap(stack.Services, stack.Sealed)
}

func splitOverlayServiceMap(services map[string]OverlayService, sealed bool) (map[string]OverlayService, map[string]OverlayService) {
	if len(services) == 0 {
		return nil, nil
	}
	unsealed := map[string]OverlayService{}
	sealedMap := map[string]OverlayService{}
	for name, svc := range services {
		if sealed || svc.Sealed {
			sealedMap[name] = svc
		} else {
			unsealed[name] = svc
		}
	}
	if len(unsealed) == 0 {
		unsealed = nil
	}
	if len(sealedMap) == 0 {
		sealedMap = nil
	}
	return unsealed, sealedMap
}

func overlayStackPartitionConfigs(overlay *Overlay, stackName string, partition string) (map[string]ConfigDef, bool) {
	part := overlayStackPartition(overlay, stackName, partition)
	if part == nil {
		return nil, false
	}
	return part.Configs.Defs, part.Sealed
}

func overlayStackPartitionConfigsFromStack(overlay *OverlayStack, partition string) (map[string]ConfigDef, bool) {
	if overlay == nil {
		return nil, false
	}
	part, ok := overlay.Partitions[partition]
	if !ok {
		return nil, false
	}
	return part.Configs.Defs, part.Sealed
}

func overlayStackPartitionSecrets(overlay *Overlay, stackName string, partition string) (map[string]SecretDef, bool) {
	part := overlayStackPartition(overlay, stackName, partition)
	if part == nil {
		return nil, false
	}
	return part.Secrets.Defs, part.Sealed
}

func overlayStackPartitionSecretsFromStack(overlay *OverlayStack, partition string) (map[string]SecretDef, bool) {
	if overlay == nil {
		return nil, false
	}
	part, ok := overlay.Partitions[partition]
	if !ok {
		return nil, false
	}
	return part.Secrets.Defs, part.Sealed
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
	delete(baseMap, "overlays")
	overlayMap := overlay.Fields
	if overlayMap != nil {
		delete(overlayMap, "overlays")
	}
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
			overlayMap := svc.Fields
			if overlayMap == nil {
				overlayMap = map[string]any{}
			}
			merged, err := decodeServiceMap(overlayMap)
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
	return cfg.stackServicesWithTrace(stackName, partition, nil)
}

func (cfg *Config) stackServicesWithTrace(stackName string, partition string, trace *LoadTrace) (map[string]Service, error) {
	stackCfg, ok := cfg.Stacks[stackName]
	if !ok {
		return nil, nil
	}
	effectivePartition := partition
	if stackCfg.Mode != "partitioned" {
		effectivePartition = ""
	}
	if !cfg.StackIncludedInTarget(stackName, effectivePartition) {
		return map[string]Service{}, nil
	}
	base := cfg.stackServices(stackName)
	deployUnsealed, deploySealed := splitOverlayServices(cfg.deploymentOverlay(), stackName)
	stackDeployUnsealed, stackDeploySealed := splitOverlayStackServices(cfg.stackDeploymentOverlay(stackName))
	recordServiceOverlayTrace(trace, stackName, "project deployment overlay", deployUnsealed)
	merged, err := mergeServices(base, deployUnsealed)
	if err != nil {
		return nil, err
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		unsealed, _ := splitOverlayServices(&overlay, stackName)
		recordServiceOverlayTrace(trace, stackName, "project partition overlay", unsealed)
		merged, err = mergeServices(merged, unsealed)
		if err != nil {
			return nil, err
		}
	}
	recordServiceOverlayTrace(trace, stackName, "stack deployment overlay", stackDeployUnsealed)
	merged, err = mergeServices(merged, stackDeployUnsealed)
	if err != nil {
		return nil, err
	}
	for _, overlay := range cfg.stackPartitionOverlays(stackName, partition) {
		unsealed, _ := splitOverlayStackServices(&overlay)
		recordServiceOverlayTrace(trace, stackName, "stack partition overlay", unsealed)
		merged, err = mergeServices(merged, unsealed)
		if err != nil {
			return nil, err
		}
	}
	serviceDeployUnsealed := cfg.serviceDeploymentOverlays(stackName, false)
	recordServiceOverlayTrace(trace, stackName, "service deployment overlay", serviceDeployUnsealed)
	merged, err = mergeServices(merged, serviceDeployUnsealed)
	if err != nil {
		return nil, err
	}
	for _, overlay := range cfg.servicePartitionOverlayMaps(stackName, partition, false) {
		recordServiceOverlayTrace(trace, stackName, "service partition overlay", overlay)
		merged, err = mergeServices(merged, overlay)
		if err != nil {
			return nil, err
		}
	}
	for _, overlay := range cfg.servicePartitionOverlayMaps(stackName, partition, true) {
		recordServiceOverlayTrace(trace, stackName, "service partition overlay", overlay)
		merged, err = mergeServices(merged, overlay)
		if err != nil {
			return nil, err
		}
	}
	serviceDeploySealed := cfg.serviceDeploymentOverlays(stackName, true)
	recordServiceOverlayTrace(trace, stackName, "service deployment overlay", serviceDeploySealed)
	merged, err = mergeServices(merged, serviceDeploySealed)
	if err != nil {
		return nil, err
	}
	for _, overlay := range cfg.stackPartitionOverlays(stackName, partition) {
		_, sealed := splitOverlayStackServices(&overlay)
		recordServiceOverlayTrace(trace, stackName, "stack partition overlay", sealed)
		merged, err = mergeServices(merged, sealed)
		if err != nil {
			return nil, err
		}
	}
	recordServiceOverlayTrace(trace, stackName, "stack deployment overlay", stackDeploySealed)
	merged, err = mergeServices(merged, stackDeploySealed)
	if err != nil {
		return nil, err
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		_, sealed := splitOverlayServices(&overlay, stackName)
		recordServiceOverlayTrace(trace, stackName, "project partition overlay", sealed)
		merged, err = mergeServices(merged, sealed)
		if err != nil {
			return nil, err
		}
	}
	recordServiceOverlayTrace(trace, stackName, "project deployment overlay", deploySealed)
	merged, err = mergeServices(merged, deploySealed)
	if err != nil {
		return nil, err
	}
	filtered := filterServicesForTarget(merged, stackCfg.Mode, cfg.Project.Deployment, effectivePartition, stackName)
	if err := validateRuntimeServiceDependencies(stackName, partition, filtered); err != nil {
		return nil, err
	}
	return filtered, nil
}

func recordServiceOverlayTrace(trace *LoadTrace, stackName string, label string, overlays map[string]OverlayService) {
	if trace == nil || len(trace.FieldPath) < 5 {
		return
	}
	if trace.FieldPath[0] != "stacks" || trace.FieldPath[1] != stackName || trace.FieldPath[2] != "services" {
		return
	}
	serviceName := trace.FieldPath[3]
	overlay, ok := overlays[serviceName]
	if !ok {
		return
	}
	if value, ok := lookupPathValue(overlay.Fields, trace.FieldPath[4:]); ok {
		trace.recordOverlay(label, value)
	}
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
	addStackOverlayPartitions := func(overlay *OverlayStack) {
		if overlay == nil {
			return
		}
		for name := range overlay.Partitions {
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
	for _, overlay := range cfg.Overlays.Partitions.Rules {
		addOverlayPartitions(&overlay.Overlay)
	}
	addStackOverlayPartitions(cfg.stackDeploymentOverlay(stackName))
	if stack, ok := cfg.Stacks[stackName]; ok {
		for _, overlay := range stack.Overlays.Partitions.Rules {
			addStackOverlayPartitions(&overlay.OverlayStack)
		}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func matchPartition(match OverlayMatchPartition, partition string) bool {
	if partition == "" {
		return false
	}
	pattern := strings.TrimSpace(match.Pattern)
	if pattern == "" {
		return true
	}
	matchType := strings.ToLower(strings.TrimSpace(match.Type))
	if matchType == "" {
		if hasGlob(pattern) {
			matchType = "glob"
		} else {
			matchType = "exact"
		}
	}
	switch matchType {
	case "exact":
		return pattern == partition
	case "glob":
		ok, err := path.Match(pattern, partition)
		if err != nil {
			return false
		}
		return ok
	case "regexp":
		re, err := regexp2.Compile(pattern, regexp2.RE2)
		if err != nil {
			return false
		}
		ok, _ := re.MatchString(partition)
		return ok
	default:
		return false
	}
}

func hasGlob(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}
