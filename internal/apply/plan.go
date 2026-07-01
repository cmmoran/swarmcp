package apply

import (
	"context"
	"sort"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/render"
	"github.com/cmmoran/swarmcp/internal/swarm"
)

type Plan struct {
	CreateConfigs  []swarm.ConfigSpec  `yaml:"create_configs,omitempty" json:"create_configs,omitempty"`
	CreateSecrets  []swarm.SecretSpec  `yaml:"create_secrets,omitempty" json:"create_secrets,omitempty"`
	CreateNetworks []swarm.NetworkSpec `yaml:"create_networks,omitempty" json:"create_networks,omitempty"`
	DeleteConfigs  []swarm.Config      `yaml:"delete_configs,omitempty" json:"delete_configs,omitempty"`
	DeleteSecrets  []swarm.Secret      `yaml:"delete_secrets,omitempty" json:"delete_secrets,omitempty"`
	SkippedDeletes SkippedDeletes      `yaml:"skipped_deletes,omitempty" json:"skipped_deletes,omitempty"`
	StackDeploys   []StackDeploy       `yaml:"stack_deploys,omitempty" json:"stack_deploys,omitempty"`
	PruneStacks    []string            `yaml:"prune_stacks,omitempty" json:"prune_stacks,omitempty"`
	Assumptions    PlanAssumptions     `yaml:"assumptions,omitempty" json:"assumptions,omitempty"`
}

type SkippedDeletes struct {
	Configs int `yaml:"configs,omitempty" json:"configs,omitempty"`
	Secrets int `yaml:"secrets,omitempty" json:"secrets,omitempty"`
}

type PlanAssumptions struct {
	AbsentConfigs   []string             `yaml:"absent_configs,omitempty" json:"absent_configs,omitempty"`
	AbsentSecrets   []string             `yaml:"absent_secrets,omitempty" json:"absent_secrets,omitempty"`
	AbsentNetworks  []string             `yaml:"absent_networks,omitempty" json:"absent_networks,omitempty"`
	AbsentServices  []string             `yaml:"absent_services,omitempty" json:"absent_services,omitempty"`
	PresentConfigs  []ResourceAssumption `yaml:"present_configs,omitempty" json:"present_configs,omitempty"`
	PresentSecrets  []ResourceAssumption `yaml:"present_secrets,omitempty" json:"present_secrets,omitempty"`
	PresentServices []ServiceAssumption  `yaml:"present_services,omitempty" json:"present_services,omitempty"`
}

type ResourceAssumption struct {
	Name string `yaml:"name" json:"name"`
	ID   string `yaml:"id" json:"id"`
}

type ServiceAssumption struct {
	Name    string `yaml:"name" json:"name"`
	ID      string `yaml:"id" json:"id"`
	Stack   string `yaml:"stack,omitempty" json:"stack,omitempty"`
	Version uint64 `yaml:"version" json:"version"`
}

func BuildPlan(ctx context.Context, client swarm.Client, cfg *config.Config, desired DesiredState, values any, partitionFilters []string, stackFilters []string, infer bool) (Plan, error) {
	projectName := cfg.Project.Name
	existingConfigs, err := client.ListConfigs(ctx)
	if err != nil {
		return Plan{}, err
	}
	existingSecrets, err := client.ListSecrets(ctx)
	if err != nil {
		return Plan{}, err
	}
	existingServices, err := client.ListServices(ctx)
	if err != nil {
		return Plan{}, err
	}
	existingNetworks, err := client.ListNetworks(ctx)
	if err != nil {
		return Plan{}, err
	}

	existingConfigNames := make(map[string]struct{}, len(existingConfigs))
	for _, cfg := range existingConfigs {
		existingConfigNames[cfg.Name] = struct{}{}
	}
	existingSecretNames := make(map[string]struct{}, len(existingSecrets))
	for _, sec := range existingSecrets {
		existingSecretNames[sec.Name] = struct{}{}
	}
	configIDs := make(map[string]string, len(existingConfigs))
	for _, cfg := range existingConfigs {
		configIDs[cfg.Name] = cfg.ID
	}
	secretIDs := make(map[string]string, len(existingSecrets))
	for _, sec := range existingSecrets {
		secretIDs[sec.Name] = sec.ID
	}
	networkNames := make(map[string]struct{}, len(existingNetworks))
	networkTargets := buildNetworkTargetIndex(existingNetworks)
	for _, net := range existingNetworks {
		networkNames[net.Name] = struct{}{}
	}
	inUseConfigIDs, inUseSecretIDs := collectInUseIDs(existingServices, configIDs, secretIDs)
	desiredConfigNames := make(map[string]struct{}, len(desired.Configs))
	desiredSecretNames := make(map[string]struct{}, len(desired.Secrets))
	desiredServiceKeys := make(map[string]struct{})
	expected, err := expectedServices(cfg, partitionFilters, stackFilters)
	if err != nil {
		return Plan{}, err
	}
	for _, svc := range expected {
		key := serviceKey{
			project:   cfg.Project.Name,
			stack:     svc.Stack,
			partition: svc.Partition,
			service:   svc.Name,
		}
		desiredServiceKeys[key.labelKey()] = struct{}{}
	}

	var plan Plan
	for _, cfg := range desired.Configs {
		if _, seen := desiredConfigNames[cfg.Name]; seen {
			continue
		}
		desiredConfigNames[cfg.Name] = struct{}{}
		if _, ok := existingConfigNames[cfg.Name]; ok {
			continue
		}
		plan.CreateConfigs = append(plan.CreateConfigs, cfg)
	}
	for _, sec := range desired.Secrets {
		if _, seen := desiredSecretNames[sec.Name]; seen {
			continue
		}
		desiredSecretNames[sec.Name] = struct{}{}
		if _, ok := existingSecretNames[sec.Name]; ok {
			continue
		}
		plan.CreateSecrets = append(plan.CreateSecrets, sec)
	}
	for _, net := range desired.Networks {
		if _, ok := networkNames[net.Name]; ok {
			continue
		}
		plan.CreateNetworks = append(plan.CreateNetworks, net)
	}
	for _, cfg := range existingConfigs {
		if !isManagedProject(cfg.Labels, projectName) {
			continue
		}
		if _, ok := inUseConfigIDs[cfg.ID]; ok {
			plan.SkippedDeletes.Configs++
			continue
		}
		if _, ok := desiredConfigNames[cfg.Name]; ok {
			continue
		}
		plan.DeleteConfigs = append(plan.DeleteConfigs, cfg)
	}
	for _, sec := range existingSecrets {
		if !isManagedProject(sec.Labels, projectName) {
			continue
		}
		if _, ok := inUseSecretIDs[sec.ID]; ok {
			plan.SkippedDeletes.Secrets++
			continue
		}
		if _, ok := desiredSecretNames[sec.Name]; ok {
			continue
		}
		plan.DeleteSecrets = append(plan.DeleteSecrets, sec)
	}

	creates, updates, err := buildServiceChanges(cfg, desired, values, existingServices, networkTargets, partitionFilters, stackFilters, infer)
	if err != nil {
		return Plan{}, err
	}
	affected := make(map[string]struct{})
	if len(creates) > 0 || len(updates) > 0 {
		for _, create := range creates {
			if stack, ok := cfg.Stacks[create.Stack]; ok {
				affected[config.StackInstanceName(cfg.Project.Name, create.Stack, create.Partition, stack.Mode)] = struct{}{}
			}
		}
		for _, update := range updates {
			if stack, ok := cfg.Stacks[update.Stack]; ok {
				affected[config.StackInstanceName(cfg.Project.Name, update.Stack, update.Partition, stack.Mode)] = struct{}{}
			}
		}
	}
	if len(affected) > 0 {
		filter := affected
		stackDeploys, err := BuildStackDeploys(cfg, desired, values, partitionFilters, stackFilters, filter, creates, updates, infer)
		if err != nil {
			return Plan{}, err
		}
		plan.StackDeploys = stackDeploys
	}

	pruneStacks := make(map[string]struct{})
	for _, svc := range existingServices {
		if !isManagedProject(svc.Labels, projectName) {
			continue
		}
		stack := svc.Labels[render.LabelStack]
		service := svc.Labels[render.LabelService]
		partition := svc.Labels[render.LabelPartition]
		if stack == "" || service == "" || partition == "" {
			continue
		}
		stackCfg, ok := cfg.Stacks[stack]
		if !ok {
			continue
		}
		partitionName := partition
		if partitionName == "none" {
			partitionName = ""
		}
		if len(stackFilters) > 0 && !selectorContains(stackFilters, stack) {
			continue
		}
		if stackCfg.Mode == "partitioned" && len(partitionFilters) > 0 && !selectorContains(partitionFilters, partitionName) {
			continue
		}
		key := serviceKey{
			project:   projectName,
			stack:     stack,
			partition: partitionName,
			service:   service,
		}
		if _, ok := desiredServiceKeys[key.labelKey()]; ok {
			continue
		}
		name := config.StackInstanceName(cfg.Project.Name, stack, partitionName, stackCfg.Mode)
		pruneStacks[name] = struct{}{}
	}
	if len(pruneStacks) > 0 {
		plan.PruneStacks = sortedKeys(pruneStacks)
	}
	plan.Assumptions = buildPlanAssumptions(plan, creates, existingServices)

	return plan, nil
}

func buildPlanAssumptions(plan Plan, creates []ServiceCreate, existingServices []swarm.Service) PlanAssumptions {
	var out PlanAssumptions
	for _, cfg := range plan.CreateConfigs {
		out.AbsentConfigs = append(out.AbsentConfigs, cfg.Name)
	}
	for _, sec := range plan.CreateSecrets {
		out.AbsentSecrets = append(out.AbsentSecrets, sec.Name)
	}
	for _, net := range plan.CreateNetworks {
		out.AbsentNetworks = append(out.AbsentNetworks, net.Name)
	}
	for _, cfg := range plan.DeleteConfigs {
		out.PresentConfigs = append(out.PresentConfigs, ResourceAssumption{Name: cfg.Name, ID: cfg.ID})
	}
	for _, sec := range plan.DeleteSecrets {
		out.PresentSecrets = append(out.PresentSecrets, ResourceAssumption{Name: sec.Name, ID: sec.ID})
	}
	for _, create := range creates {
		out.AbsentServices = append(out.AbsentServices, create.Name)
	}
	deployedStacks := make(map[string]struct{}, len(plan.StackDeploys)+len(plan.PruneStacks))
	for _, deploy := range plan.StackDeploys {
		deployedStacks[deploy.Name] = struct{}{}
	}
	for _, name := range plan.PruneStacks {
		deployedStacks[name] = struct{}{}
	}
	for _, svc := range existingServices {
		stackName := stackNameFromService(svc)
		if _, ok := deployedStacks[stackName]; !ok {
			continue
		}
		out.PresentServices = append(out.PresentServices, ServiceAssumption{
			Name:    svc.Name,
			ID:      svc.ID,
			Stack:   stackName,
			Version: svc.Version,
		})
	}
	return normalizePlanAssumptions(out)
}

func stackNameFromService(svc swarm.Service) string {
	if svc.Labels == nil {
		return ""
	}
	project := svc.Labels[render.LabelProject]
	stack := svc.Labels[render.LabelStack]
	partition := svc.Labels[render.LabelPartition]
	if partition == "none" {
		partition = ""
	}
	if project == "" || stack == "" {
		return ""
	}
	mode := "shared"
	if partition != "" {
		mode = "partitioned"
	}
	return config.StackInstanceName(project, stack, partition, mode)
}

func normalizePlanAssumptions(in PlanAssumptions) PlanAssumptions {
	in.AbsentConfigs = sortedUniqueStrings(in.AbsentConfigs)
	in.AbsentSecrets = sortedUniqueStrings(in.AbsentSecrets)
	in.AbsentNetworks = sortedUniqueStrings(in.AbsentNetworks)
	in.AbsentServices = sortedUniqueStrings(in.AbsentServices)
	sort.Slice(in.PresentConfigs, func(i, j int) bool {
		if in.PresentConfigs[i].Name == in.PresentConfigs[j].Name {
			return in.PresentConfigs[i].ID < in.PresentConfigs[j].ID
		}
		return in.PresentConfigs[i].Name < in.PresentConfigs[j].Name
	})
	sort.Slice(in.PresentSecrets, func(i, j int) bool {
		if in.PresentSecrets[i].Name == in.PresentSecrets[j].Name {
			return in.PresentSecrets[i].ID < in.PresentSecrets[j].ID
		}
		return in.PresentSecrets[i].Name < in.PresentSecrets[j].Name
	})
	sort.Slice(in.PresentServices, func(i, j int) bool {
		if in.PresentServices[i].Name == in.PresentServices[j].Name {
			return in.PresentServices[i].ID < in.PresentServices[j].ID
		}
		return in.PresentServices[i].Name < in.PresentServices[j].Name
	})
	return in
}

func sortedUniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		seen[value] = struct{}{}
	}
	return sortedKeys(seen)
}

func sortedKeys(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func Apply(ctx context.Context, client swarm.Client, plan Plan, contextName string, pruneServices bool, stackParallel int, noUI bool, outputMode string, outputExplicit bool) error {
	for _, net := range plan.CreateNetworks {
		if _, err := client.CreateNetwork(ctx, net); err != nil {
			return err
		}
	}
	for _, cfg := range plan.CreateConfigs {
		if _, err := client.CreateConfig(ctx, cfg); err != nil {
			return err
		}
	}
	for _, sec := range plan.CreateSecrets {
		if _, err := client.CreateSecret(ctx, sec); err != nil {
			return err
		}
	}
	if err := DeployStacks(ctx, plan.StackDeploys, contextName, pruneServices, stackParallel, noUI, outputMode, outputExplicit); err != nil {
		return err
	}
	for _, cfg := range plan.DeleteConfigs {
		if err := client.RemoveConfig(ctx, cfg.ID); err != nil {
			return err
		}
	}
	for _, sec := range plan.DeleteSecrets {
		if err := client.RemoveSecret(ctx, sec.ID); err != nil {
			return err
		}
	}
	return nil
}

func collectInUseIDs(services []swarm.Service, configIDs map[string]string, secretIDs map[string]string) (map[string]struct{}, map[string]struct{}) {
	inUseConfigs := make(map[string]struct{})
	inUseSecrets := make(map[string]struct{})
	for _, svc := range services {
		container := svc.Spec.TaskTemplate.ContainerSpec
		if container == nil {
			continue
		}
		for _, ref := range container.Configs {
			if ref == nil {
				continue
			}
			id := ref.ConfigID
			if id == "" && ref.ConfigName != "" {
				id = configIDs[ref.ConfigName]
			}
			if id != "" {
				inUseConfigs[id] = struct{}{}
			}
		}
		for _, ref := range container.Secrets {
			if ref == nil {
				continue
			}
			id := ref.SecretID
			if id == "" && ref.SecretName != "" {
				id = secretIDs[ref.SecretName]
			}
			if id != "" {
				inUseSecrets[id] = struct{}{}
			}
		}
	}
	return inUseConfigs, inUseSecrets
}
