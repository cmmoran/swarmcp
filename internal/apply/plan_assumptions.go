package apply

import (
	"context"
	"fmt"

	"github.com/cmmoran/swarmcp/internal/swarm"
)

func ValidatePlanAssumptions(ctx context.Context, client swarm.Client, assumptions PlanAssumptions) error {
	configs, err := client.ListConfigs(ctx)
	if err != nil {
		return err
	}
	secrets, err := client.ListSecrets(ctx)
	if err != nil {
		return err
	}
	services, err := client.ListServices(ctx)
	if err != nil {
		return err
	}
	networks, err := client.ListNetworks(ctx)
	if err != nil {
		return err
	}

	configByName := configsByName(configs)
	secretByName := secretsByName(secrets)
	serviceByName := servicesByName(services)
	networkByName := networksByName(networks)
	configIDs := configIDsByName(configs)
	secretIDs := secretIDsByName(secrets)
	inUseConfigIDs, inUseSecretIDs := collectInUseIDs(services, configIDs, secretIDs)

	for _, name := range assumptions.AbsentConfigs {
		if _, ok := configByName[name]; ok {
			return fmt.Errorf("plan assumption failed: config %q now exists", name)
		}
	}
	for _, name := range assumptions.AbsentSecrets {
		if _, ok := secretByName[name]; ok {
			return fmt.Errorf("plan assumption failed: secret %q now exists", name)
		}
	}
	for _, name := range assumptions.AbsentNetworks {
		if _, ok := networkByName[name]; ok {
			return fmt.Errorf("plan assumption failed: network %q now exists", name)
		}
	}
	for _, name := range assumptions.AbsentServices {
		if _, ok := serviceByName[name]; ok {
			return fmt.Errorf("plan assumption failed: service %q now exists", name)
		}
	}
	for _, expected := range assumptions.PresentConfigs {
		current, ok := configByName[expected.Name]
		if !ok {
			return fmt.Errorf("plan assumption failed: config %q no longer exists", expected.Name)
		}
		if current.ID != expected.ID {
			return fmt.Errorf("plan assumption failed: config %q id changed: got %s want %s", expected.Name, current.ID, expected.ID)
		}
		if _, ok := inUseConfigIDs[current.ID]; ok {
			return fmt.Errorf("plan assumption failed: config %q is now in use", expected.Name)
		}
	}
	for _, expected := range assumptions.PresentSecrets {
		current, ok := secretByName[expected.Name]
		if !ok {
			return fmt.Errorf("plan assumption failed: secret %q no longer exists", expected.Name)
		}
		if current.ID != expected.ID {
			return fmt.Errorf("plan assumption failed: secret %q id changed: got %s want %s", expected.Name, current.ID, expected.ID)
		}
		if _, ok := inUseSecretIDs[current.ID]; ok {
			return fmt.Errorf("plan assumption failed: secret %q is now in use", expected.Name)
		}
	}
	for _, expected := range assumptions.PresentServices {
		current, ok := serviceByName[expected.Name]
		if !ok {
			return fmt.Errorf("plan assumption failed: service %q no longer exists", expected.Name)
		}
		if current.ID != expected.ID {
			return fmt.Errorf("plan assumption failed: service %q id changed: got %s want %s", expected.Name, current.ID, expected.ID)
		}
		if current.Version != expected.Version {
			return fmt.Errorf("plan assumption failed: service %q version changed: got %d want %d", expected.Name, current.Version, expected.Version)
		}
	}
	return nil
}

func FinalizePlanAssumptions(plan Plan) Plan {
	plan.Assumptions = finalizedPlanAssumptions(plan)
	return plan
}

func finalizedPlanAssumptions(plan Plan) PlanAssumptions {
	current := plan.Assumptions
	deployedStacks := stackDeployNames(plan.StackDeploys)
	out := PlanAssumptions{
		AbsentConfigs:   filterStrings(current.AbsentConfigs, configNames(plan.CreateConfigs)),
		AbsentSecrets:   filterStrings(current.AbsentSecrets, secretNames(plan.CreateSecrets)),
		AbsentNetworks:  filterStrings(current.AbsentNetworks, networkNamesFromSpecs(plan.CreateNetworks)),
		AbsentServices:  current.AbsentServices,
		PresentConfigs:  filterResourceAssumptions(current.PresentConfigs, configResourceKeys(plan.DeleteConfigs)),
		PresentSecrets:  filterResourceAssumptions(current.PresentSecrets, secretResourceKeys(plan.DeleteSecrets)),
		PresentServices: filterServiceAssumptions(current.PresentServices, deployedStacks),
	}
	return normalizePlanAssumptions(out)
}

func stackDeployNames(deploys []StackDeploy) map[string]struct{} {
	out := make(map[string]struct{}, len(deploys))
	for _, deploy := range deploys {
		out[deploy.Name] = struct{}{}
	}
	return out
}

func filterServiceAssumptions(values []ServiceAssumption, deployedStacks map[string]struct{}) []ServiceAssumption {
	var out []ServiceAssumption
	for _, value := range values {
		if value.Stack == "" {
			continue
		}
		if _, ok := deployedStacks[value.Stack]; !ok {
			continue
		}
		out = append(out, value)
	}
	return out
}

func filterStrings(values []string, allowed map[string]struct{}) []string {
	var out []string
	for _, value := range values {
		if _, ok := allowed[value]; ok {
			out = append(out, value)
		}
	}
	return out
}

func filterResourceAssumptions(values []ResourceAssumption, allowed map[ResourceAssumption]struct{}) []ResourceAssumption {
	var out []ResourceAssumption
	for _, value := range values {
		if _, ok := allowed[value]; ok {
			out = append(out, value)
		}
	}
	return out
}

func configsByName(configs []swarm.Config) map[string]swarm.Config {
	out := make(map[string]swarm.Config, len(configs))
	for _, cfg := range configs {
		out[cfg.Name] = cfg
	}
	return out
}

func secretsByName(secrets []swarm.Secret) map[string]swarm.Secret {
	out := make(map[string]swarm.Secret, len(secrets))
	for _, sec := range secrets {
		out[sec.Name] = sec
	}
	return out
}

func servicesByName(services []swarm.Service) map[string]swarm.Service {
	out := make(map[string]swarm.Service, len(services))
	for _, svc := range services {
		out[svc.Name] = svc
	}
	return out
}

func networksByName(networks []swarm.Network) map[string]swarm.Network {
	out := make(map[string]swarm.Network, len(networks))
	for _, net := range networks {
		out[net.Name] = net
	}
	return out
}

func configIDsByName(configs []swarm.Config) map[string]string {
	out := make(map[string]string, len(configs))
	for _, cfg := range configs {
		out[cfg.Name] = cfg.ID
	}
	return out
}

func secretIDsByName(secrets []swarm.Secret) map[string]string {
	out := make(map[string]string, len(secrets))
	for _, sec := range secrets {
		out[sec.Name] = sec.ID
	}
	return out
}

func configNames(configs []swarm.ConfigSpec) map[string]struct{} {
	out := make(map[string]struct{}, len(configs))
	for _, cfg := range configs {
		out[cfg.Name] = struct{}{}
	}
	return out
}

func secretNames(secrets []swarm.SecretSpec) map[string]struct{} {
	out := make(map[string]struct{}, len(secrets))
	for _, sec := range secrets {
		out[sec.Name] = struct{}{}
	}
	return out
}

func networkNamesFromSpecs(networks []swarm.NetworkSpec) map[string]struct{} {
	out := make(map[string]struct{}, len(networks))
	for _, net := range networks {
		out[net.Name] = struct{}{}
	}
	return out
}

func configResourceKeys(configs []swarm.Config) map[ResourceAssumption]struct{} {
	out := make(map[ResourceAssumption]struct{}, len(configs))
	for _, cfg := range configs {
		out[ResourceAssumption{Name: cfg.Name, ID: cfg.ID}] = struct{}{}
	}
	return out
}

func secretResourceKeys(secrets []swarm.Secret) map[ResourceAssumption]struct{} {
	out := make(map[ResourceAssumption]struct{}, len(secrets))
	for _, sec := range secrets {
		out[ResourceAssumption{Name: sec.Name, ID: sec.ID}] = struct{}{}
	}
	return out
}
