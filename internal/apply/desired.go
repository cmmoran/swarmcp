package apply

import (
	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/render"
	"github.com/cmmoran/swarmcp/internal/secrets"
	"github.com/cmmoran/swarmcp/internal/swarm"
)

type DesiredState struct {
	Configs  []swarm.ConfigSpec
	Secrets  []swarm.SecretSpec
	Networks []swarm.NetworkSpec
	Defs     []render.RenderedDef
	Missing  []string
}

func BuildDesiredState(cfg *config.Config, store *secrets.Store, values any, partitionFilter string, allowMissing bool, infer bool) (DesiredState, error) {
	summary, err := render.RenderProject(cfg, store, values, partitionFilter, allowMissing, infer)
	if err != nil {
		return DesiredState{}, err
	}

	return DesiredStateFromSummary(cfg, summary, partitionFilter), nil
}

func DesiredNetworks(cfg *config.Config, partitionFilter string) []swarm.NetworkSpec {
	return buildDesiredNetworks(cfg, partitionFilter)
}

func DesiredStateFromSummary(cfg *config.Config, summary render.Summary, partitionFilter string) DesiredState {
	desired := DesiredState{
		Defs:    summary.Defs,
		Missing: summary.MissingSecrets,
	}
	desired.Networks = buildDesiredNetworks(cfg, partitionFilter)
	for _, def := range summary.Defs {
		physical, hash := render.PhysicalName(def.Name, def.Content)
		labels := render.Labels(def.ScopeID, def.Name, hash)
		switch def.Kind {
		case "config":
			desired.Configs = append(desired.Configs, swarm.ConfigSpec{
				Name:   physical,
				Labels: labels,
				Data:   []byte(def.Content),
			})
		case "secret":
			desired.Secrets = append(desired.Secrets, swarm.SecretSpec{
				Name:   physical,
				Labels: labels,
				Data:   []byte(def.Content),
			})
		}
	}
	return desired
}
