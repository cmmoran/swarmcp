package apply

import (
	"github.com/cmmoran/swarmcp/internal/render"
	"github.com/cmmoran/swarmcp/internal/templates"
)

func SecretSourcesForPlan(desired DesiredState, plan Plan) []PlanSecretSource {
	created := make(map[string]struct{}, len(plan.CreateSecrets))
	for _, secret := range plan.CreateSecrets {
		created[secret.Name] = struct{}{}
	}
	out := make([]PlanSecretSource, 0)
	for _, def := range desired.Defs {
		if def.Kind != "secret" || len(def.SecretDependencies) == 0 {
			continue
		}
		secretName, _ := render.PhysicalName(def.Name, def.Content)
		if _, ok := created[secretName]; !ok {
			continue
		}
		out = append(out, PlanSecretSource{
			SecretName:   secretName,
			LogicalName:  def.Name,
			Scope:        planScope(def.ScopeID),
			Dependencies: planSecretDependencies(def.SecretDependencies),
		})
	}
	return out
}

func planSecretDependencies(deps []render.SecretDependency) []PlanSecretDependency {
	out := make([]PlanSecretDependency, 0, len(deps))
	for _, dep := range deps {
		out = append(out, PlanSecretDependency{
			Name:     dep.Name,
			Scope:    planScope(dep.Scope),
			Hash:     dep.Hash,
			Provider: dep.Metadata.Provider,
			Mount:    dep.Metadata.Mount,
			Path:     dep.Metadata.Path,
			Key:      dep.Metadata.Key,
			Version:  dep.Metadata.Version,
		})
	}
	return out
}

func planScope(scope templates.Scope) PlanScope {
	return PlanScope{
		Project:    scope.Project,
		Deployment: scope.Deployment,
		Stack:      scope.Stack,
		Partition:  scope.Partition,
		Service:    scope.Service,
	}
}
