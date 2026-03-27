package apply

import (
	"fmt"
	"sort"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/render"
	"github.com/cmmoran/swarmcp/internal/templates"
	"github.com/docker/docker/api/types/mount"
)

type BindPathRequirement struct {
	Scope       templates.Scope
	Source      string
	Target      string
	Constraints []string
}

func PlanBindPaths(cfg *config.Config, values any, partitionFilters []string, stackFilters []string) ([]BindPathRequirement, error) {
	var out []BindPathRequirement
	for stackName, stack := range cfg.Stacks {
		if len(stackFilters) > 0 && !selectorContains(stackFilters, stackName) {
			continue
		}
		if !cfg.StackSelectedForRuntime(stackName, partitionFilters) {
			continue
		}
		partitions := []string{""}
		if stack.Mode == "partitioned" && len(cfg.Project.Partitions) > 0 {
			partitions = cfg.StackRuntimePartitions(stackName, partitionFilters)
		}
		for _, partitionName := range partitions {
			services, err := cfg.StackServices(stackName, partitionName)
			if err != nil {
				return nil, err
			}
			if len(services) == 0 {
				continue
			}
			for serviceName, service := range services {
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
				// Bind path planning should not fail on missing refs in templates.
				_, engine, templateScope, templateData := render.NewServiceTemplateEngine(cfg, scope, values, true, nil)
				renderedService, err := render.RenderServiceTemplates(engine, templateScope, templateData, service)
				if err != nil {
					return nil, err
				}
				mounts, err := desiredVolumeMounts(cfg, engine, templateData, stackName, stack, partitionName, serviceName, renderedService)
				if err != nil {
					return nil, err
				}
				if len(mounts) == 0 {
					continue
				}
				constraints := desiredPlacementConstraints(cfg, stackName, stack, partitionName, serviceName, renderedService)
				for _, m := range mounts {
					if m.Type != mount.TypeBind {
						continue
					}
					out = append(out, BindPathRequirement{
						Scope:       scope,
						Source:      m.Source,
						Target:      m.Target,
						Constraints: constraints,
					})
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		if out[i].Target != out[j].Target {
			return out[i].Target < out[j].Target
		}
		if out[i].Scope.Stack != out[j].Scope.Stack {
			return out[i].Scope.Stack < out[j].Scope.Stack
		}
		return out[i].Scope.Service < out[j].Scope.Service
	})
	return out, nil
}

func formatBindPathLine(scope templates.Scope, source, target string) string {
	if scope.Partition == "" {
		return fmt.Sprintf("%s -> %s (stack %q service %q)", source, target, scope.Stack, scope.Service)
	}
	return fmt.Sprintf("%s -> %s (stack %q partition %q service %q)", source, target, scope.Stack, scope.Partition, scope.Service)
}

func FormatBindPathLine(req BindPathRequirement) string {
	return formatBindPathLine(req.Scope, req.Source, req.Target)
}
