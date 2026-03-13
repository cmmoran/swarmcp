package apply

import (
	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/render"
	"github.com/cmmoran/swarmcp/internal/templates"
	"github.com/docker/docker/api/types/mount"
)

type serviceIntentBuild struct {
	Rendered     config.Service
	ConfigMounts []ServiceMount
	SecretMounts []ServiceMount
	VolumeMounts []mount.Mount
	Networks     []string
	Labels       map[string]string
	Constraints  []string
	Intent       serviceIntent
}

func buildServiceIntent(cfg *config.Config, stackName string, stack config.Stack, partitionName string, serviceName string, service config.Service, values any, infer bool, defIndex map[defKey]string) (serviceIntentBuild, error) {
	networkEphemeral := ""
	if service.NetworkEphemeral != nil {
		networkEphemeral = config.EphemeralNetworkName(cfg, stackName, stack.Mode, partitionName, serviceName)
	}
	scope := templates.Scope{
		Project:          cfg.Project.Name,
		Deployment:       cfg.Project.Deployment,
		Stack:            stackName,
		Partition:        partitionName,
		Service:          serviceName,
		NetworksShared:   config.NetworksSharedString(cfg, partitionName),
		NetworkEphemeral: networkEphemeral,
	}
	var inferredConfigs map[string]struct{}
	var inferredSecrets map[string]struct{}
	var trace func(templates.TraceCall)
	if infer {
		inferredConfigs = make(map[string]struct{})
		inferredSecrets = make(map[string]struct{})
		trace = func(call templates.TraceCall) {
			switch call.Func {
			case "config_ref":
				inferredConfigs[call.Name] = struct{}{}
			case "secret_ref":
				inferredSecrets[call.Name] = struct{}{}
			}
		}
	}
	resolver, engine, scope, data := render.NewServiceTemplateEngine(cfg, scope, values, infer, trace)

	renderedService, err := render.RenderServiceTemplates(engine, scope, data, service)
	if err != nil {
		return serviceIntentBuild{}, err
	}
	if infer {
		renderedService.Configs = mergeConfigRefs(renderedService.Configs, inferredConfigs)
		renderedService.Secrets = mergeSecretRefs(renderedService.Secrets, inferredSecrets)
		extraConfigs, extraSecrets, exerr := render.InferTemplateRefDeps(cfg, scope, renderedService.Configs, renderedService.Secrets)
		if exerr != nil {
			return serviceIntentBuild{}, exerr
		}
		renderedService.Configs = mergeConfigRefs(renderedService.Configs, extraConfigs)
		renderedService.Secrets = mergeSecretRefs(renderedService.Secrets, extraSecrets)
	}

	configMounts, err := desiredConfigMounts(resolver, engine, data, defIndex, scope, renderedService.Configs, infer)
	if err != nil {
		return serviceIntentBuild{}, err
	}
	secretMounts, err := desiredSecretMounts(resolver, engine, data, defIndex, scope, renderedService.Secrets, infer)
	if err != nil {
		return serviceIntentBuild{}, err
	}
	volumeMounts, err := desiredVolumeMounts(cfg, engine, data, stackName, stack, partitionName, serviceName, renderedService)
	if err != nil {
		return serviceIntentBuild{}, err
	}
	serviceNetworks := desiredServiceNetworks(cfg, stackName, stack.Mode, partitionName, serviceName, renderedService)

	labels, err := serviceLabels(scope, serviceName, renderedService.Labels, resolver, data)
	if err != nil {
		return serviceIntentBuild{}, err
	}
	constraints := desiredPlacementConstraints(cfg, stackName, stack, partitionName, serviceName, renderedService)
	restartPolicy := config.MergeRestartPolicies(
		cfg.Project.RestartPolicy,
		stack.RestartPolicy,
		config.StackPartitionRestartPolicy(stack, partitionName),
		renderedService.RestartPolicy,
	)
	updatePolicy := config.MergeUpdatePolicies(
		cfg.Project.UpdateConfig,
		stack.UpdateConfig,
		config.StackPartitionUpdateConfig(stack, partitionName),
		renderedService.UpdateConfig,
	)
	rollbackPolicy := config.MergeUpdatePolicies(
		cfg.Project.RollbackConfig,
		stack.RollbackConfig,
		config.StackPartitionRollbackConfig(stack, partitionName),
		renderedService.RollbackConfig,
	)
	intent, err := intentFromConfig(renderedService, labels, constraints, configMounts, secretMounts, volumeMounts, serviceNetworks, restartPolicy, updatePolicy, rollbackPolicy)
	if err != nil {
		return serviceIntentBuild{}, err
	}

	return serviceIntentBuild{
		Rendered:     renderedService,
		ConfigMounts: configMounts,
		SecretMounts: secretMounts,
		VolumeMounts: volumeMounts,
		Networks:     serviceNetworks,
		Labels:       labels,
		Constraints:  constraints,
		Intent:       intent,
	}, nil
}
