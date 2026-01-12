package apply

import (
	"sort"

	"github.com/cmmoran/swarmcp/internal/swarm"
)

type PruneResult struct {
	PreserveCount    int
	ConfigsPreserved int
	SecretsPreserved int
}

func PrunePlan(plan Plan, preserve int) (Plan, PruneResult) {
	if preserve < 0 {
		preserve = 0
	}
	deletes := plan.DeleteConfigs
	var planResultConfigs int
	plan.DeleteConfigs, planResultConfigs = pruneConfigs(deletes, preserve)
	var planResultSecrets int
	plan.DeleteSecrets, planResultSecrets = pruneSecrets(plan.DeleteSecrets, preserve)
	return plan, PruneResult{
		PreserveCount:    preserve,
		ConfigsPreserved: planResultConfigs,
		SecretsPreserved: planResultSecrets,
	}
}

func pruneConfigs(configs []swarm.Config, preserve int) ([]swarm.Config, int) {
	if len(configs) == 0 {
		return nil, 0
	}
	ordered := append([]swarm.Config(nil), configs...)
	sort.Slice(ordered, func(i, j int) bool {
		left := ordered[i].CreatedAt
		right := ordered[j].CreatedAt
		if !left.Equal(right) {
			return left.After(right)
		}
		return ordered[i].Name > ordered[j].Name
	})
	if preserve >= len(ordered) {
		return nil, len(ordered)
	}
	if preserve <= 0 {
		return ordered, 0
	}
	return ordered[preserve:], preserve
}

func pruneSecrets(secrets []swarm.Secret, preserve int) ([]swarm.Secret, int) {
	if len(secrets) == 0 {
		return nil, 0
	}
	ordered := append([]swarm.Secret(nil), secrets...)
	sort.Slice(ordered, func(i, j int) bool {
		left := ordered[i].CreatedAt
		right := ordered[j].CreatedAt
		if !left.Equal(right) {
			return left.After(right)
		}
		return ordered[i].Name > ordered[j].Name
	})
	if preserve >= len(ordered) {
		return nil, len(ordered)
	}
	if preserve <= 0 {
		return ordered, 0
	}
	return ordered[preserve:], preserve
}
