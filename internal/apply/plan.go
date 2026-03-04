package apply

import (
	"context"
	"sort"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/render"
	"github.com/cmmoran/swarmcp/internal/swarm"
)

type Plan struct {
	CreateConfigs  []swarm.ConfigSpec
	CreateSecrets  []swarm.SecretSpec
	CreateNetworks []swarm.NetworkSpec
	DeleteConfigs  []swarm.Config
	DeleteSecrets  []swarm.Secret
	SkippedDeletes SkippedDeletes
	StackDeploys   []StackDeploy
	PruneStacks    []string
}

type SkippedDeletes struct {
	Configs int
	Secrets int
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

	return plan, nil
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
