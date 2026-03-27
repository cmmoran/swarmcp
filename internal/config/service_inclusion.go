package config

import "fmt"

func stackIncludedInTarget(stack Stack, deployment string, partition string, stackName string) bool {
	rules := stack.IncludedIn
	if len(rules) == 0 {
		return true
	}
	for _, rule := range rules {
		if inclusionRuleMatches(rule, stack.Mode, deployment, partition, stackName) {
			return true
		}
	}
	return false
}

func serviceIncludedInTarget(service Service, stackMode string, deployment string, partition string, stack string) bool {
	if len(service.IncludedIn) == 0 {
		return true
	}
	for _, rule := range service.IncludedIn {
		if inclusionRuleMatches(rule, stackMode, deployment, partition, stack) {
			return true
		}
	}
	return false
}

func inclusionRuleMatches(rule InclusionRule, stackMode string, deployment string, partition string, stack string) bool {
	if len(rule.Deployments) > 0 && !stringInSlice(rule.Deployments, deployment) {
		return false
	}
	if stackMode != "shared" && len(rule.Partitions) > 0 && !stringInSlice(rule.Partitions, partition) {
		return false
	}
	if len(rule.Stacks) > 0 && !stringInSlice(rule.Stacks, stack) {
		return false
	}
	return true
}

func filterServicesForTarget(services map[string]Service, stackMode string, deployment string, partition string, stack string) map[string]Service {
	if len(services) == 0 {
		return services
	}
	out := make(map[string]Service, len(services))
	for name, service := range services {
		if serviceIncludedInTarget(service, stackMode, deployment, partition, stack) {
			out[name] = service
		}
	}
	return out
}

func validateIncludedIn(scope string, rules []InclusionRule, cfg *Config) error {
	if len(rules) == 0 {
		return nil
	}
	var errs []string
	for i, rule := range rules {
		ruleScope := fmt.Sprintf("%s[%d]", scope, i)
		if len(rule.Deployments) == 0 && len(rule.Partitions) == 0 && len(rule.Stacks) == 0 {
			errs = append(errs, fmt.Sprintf("%s: at least one of deployments, partitions, or stacks is required", ruleScope))
		}
		for _, deployment := range rule.Deployments {
			if err := validateLogicalName(ruleScope+" deployment "+deployment, deployment); err != nil {
				errs = append(errs, err.Error())
				continue
			}
			if len(cfg.Project.Deployments) > 0 && !deploymentInProject(cfg, deployment) {
				errs = append(errs, fmt.Sprintf("%s.deployments: deployment %q not found in project.deployments", ruleScope, deployment))
			}
		}
		for _, partition := range rule.Partitions {
			if err := validateLogicalName(ruleScope+" partition "+partition, partition); err != nil {
				errs = append(errs, err.Error())
				continue
			}
			if !partitionInProject(cfg, partition) {
				errs = append(errs, fmt.Sprintf("%s.partitions: partition %q not found in project.partitions", ruleScope, partition))
			}
		}
		for _, stack := range rule.Stacks {
			if err := validateLogicalName(ruleScope+" stack "+stack, stack); err != nil {
				errs = append(errs, err.Error())
				continue
			}
			if _, ok := cfg.Stacks[stack]; !ok {
				errs = append(errs, fmt.Sprintf("%s.stacks: stack %q not found in stacks", ruleScope, stack))
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", joinErrors(errs))
	}
	return nil
}

func validateServiceIncludedIn(scope string, rules []InclusionRule, cfg *Config) error {
	return validateIncludedIn(scope, rules, cfg)
}

func validateStackIncludedIn(scope string, rules []InclusionRule, cfg *Config) error {
	return validateIncludedIn(scope, rules, cfg)
}

func (cfg *Config) StackIncludedInTarget(stackName string, partition string) bool {
	stack, ok := cfg.Stacks[stackName]
	if !ok {
		return false
	}
	stack.IncludedIn = cfg.stackIncludedInRules(stackName, partition)
	return stackIncludedInTarget(stack, cfg.Project.Deployment, partition, stackName)
}

func (cfg *Config) StackSelectedForRuntime(stackName string, partitionFilters []string) bool {
	stack, ok := cfg.Stacks[stackName]
	if !ok {
		return false
	}
	if stack.Mode != "partitioned" {
		return cfg.StackIncludedInTarget(stackName, "")
	}
	return len(cfg.StackRuntimePartitions(stackName, partitionFilters)) > 0
}

func (cfg *Config) StackRuntimePartitions(stackName string, filters []string) []string {
	stack, ok := cfg.Stacks[stackName]
	if !ok {
		return nil
	}
	if stack.Mode != "partitioned" {
		if cfg.StackIncludedInTarget(stackName, "") {
			return []string{""}
		}
		return nil
	}
	partitions := append([]string(nil), cfg.Project.Partitions...)
	if len(filters) > 0 {
		partitions = filterAllowedPartitions(partitions, filters)
	}
	out := make([]string, 0, len(partitions))
	for _, partition := range partitions {
		if cfg.StackIncludedInTarget(stackName, partition) {
			out = append(out, partition)
		}
	}
	return out
}

func (cfg *Config) StackAvailablePartitions(stackName string) []string {
	stack, ok := cfg.Stacks[stackName]
	if !ok {
		return nil
	}
	if stack.Mode != "partitioned" {
		if cfg.StackIncludedInTarget(stackName, "") {
			return []string{""}
		}
		return nil
	}
	return cfg.StackRuntimePartitions(stackName, nil)
}

func filterAllowedPartitions(candidates []string, allowed []string) []string {
	if len(candidates) == 0 || len(allowed) == 0 {
		return candidates
	}
	set := make(map[string]struct{}, len(allowed))
	for _, value := range allowed {
		set[value] = struct{}{}
	}
	out := make([]string, 0, len(candidates))
	for _, value := range candidates {
		if _, ok := set[value]; ok {
			out = append(out, value)
		}
	}
	return out
}

func (cfg *Config) stackIncludedInRules(stackName string, partition string) []InclusionRule {
	stack, ok := cfg.Stacks[stackName]
	if !ok {
		return nil
	}
	merged := copyInclusionRules(stack.IncludedIn)
	deployRules, deploySealed := overlayStackIncludedIn(cfg.deploymentOverlay(), stackName)
	stackDeployRules, stackDeploySealed := overlayStackIncludedInFromStack(cfg.stackDeploymentOverlay(stackName))
	if deployRules != nil && !deploySealed {
		merged = copyInclusionRules(deployRules)
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		rules, sealed := overlayStackIncludedIn(&overlay, stackName)
		if rules == nil || sealed {
			continue
		}
		merged = copyInclusionRules(rules)
	}
	if stackDeployRules != nil && !stackDeploySealed {
		merged = copyInclusionRules(stackDeployRules)
	}
	for _, overlay := range cfg.stackPartitionOverlays(stackName, partition) {
		rules, sealed := overlayStackIncludedInFromStack(&overlay)
		if rules == nil || sealed {
			continue
		}
		merged = copyInclusionRules(rules)
	}
	for _, overlay := range cfg.stackPartitionOverlays(stackName, partition) {
		rules, sealed := overlayStackIncludedInFromStack(&overlay)
		if rules == nil || !sealed {
			continue
		}
		merged = copyInclusionRules(rules)
	}
	if stackDeployRules != nil && stackDeploySealed {
		merged = copyInclusionRules(stackDeployRules)
	}
	for _, overlay := range cfg.partitionOverlays(partition) {
		rules, sealed := overlayStackIncludedIn(&overlay, stackName)
		if rules == nil || !sealed {
			continue
		}
		merged = copyInclusionRules(rules)
	}
	if deployRules != nil && deploySealed {
		merged = copyInclusionRules(deployRules)
	}
	return merged
}

func overlayStackIncludedIn(overlay *Overlay, stackName string) ([]InclusionRule, bool) {
	stack := overlayStack(overlay, stackName)
	if stack == nil {
		return nil, false
	}
	return copyInclusionRules(stack.IncludedIn), stack.Sealed
}

func overlayStackIncludedInFromStack(stack *OverlayStack) ([]InclusionRule, bool) {
	if stack == nil {
		return nil, false
	}
	return copyInclusionRules(stack.IncludedIn), stack.Sealed
}

func copyInclusionRules(rules []InclusionRule) []InclusionRule {
	if rules == nil {
		return nil
	}
	out := make([]InclusionRule, len(rules))
	for i, rule := range rules {
		out[i] = InclusionRule{
			Deployments: append([]string(nil), rule.Deployments...),
			Partitions:  append([]string(nil), rule.Partitions...),
			Stacks:      append([]string(nil), rule.Stacks...),
		}
	}
	return out
}

func validateRuntimeServiceDependencies(stackName string, partition string, services map[string]Service) error {
	if len(services) == 0 {
		return nil
	}
	var errs []string
	scope := fmt.Sprintf("stack %q", stackName)
	if partition != "" {
		scope = fmt.Sprintf("stack %q partition %q", stackName, partition)
	}
	for serviceName, service := range services {
		for _, dep := range service.DependsOn {
			if dep == "" {
				continue
			}
			if _, ok := services[dep]; ok {
				continue
			}
			errs = append(errs, fmt.Sprintf("%s service %q.depends_on: service %q is not included for this target", scope, serviceName, dep))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", joinErrors(errs))
	}
	return nil
}

func stringInSlice(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
