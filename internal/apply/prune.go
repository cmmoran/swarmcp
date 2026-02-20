package apply

import (
	"sort"

	"github.com/cmmoran/swarmcp/internal/render"
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

func PruneStaleResources(configs []swarm.Config, secrets []swarm.Secret, preserve int) ([]swarm.Config, []swarm.Secret, PruneResult) {
	if preserve < 0 {
		preserve = 0
	}
	prunedConfigs, configsPreserved := pruneConfigs(configs, preserve)
	prunedSecrets, secretsPreserved := pruneSecrets(secrets, preserve)
	return prunedConfigs, prunedSecrets, PruneResult{
		PreserveCount:    preserve,
		ConfigsPreserved: configsPreserved,
		SecretsPreserved: secretsPreserved,
	}
}

func pruneConfigs(configs []swarm.Config, preserve int) ([]swarm.Config, int) {
	if len(configs) == 0 {
		return nil, 0
	}
	if preserve <= 0 {
		ordered := append([]swarm.Config(nil), configs...)
		sortConfigsNewestFirst(ordered)
		return ordered, 0
	}
	type scopeKey struct {
		project   string
		stack     string
		partition string
		logical   string
	}
	grouped := make(map[scopeKey][]swarm.Config)
	for _, cfg := range configs {
		key := scopeKey{
			project:   labelOr(cfg.Labels, render.LabelProject, ""),
			stack:     normalizeScopeLabel(labelOr(cfg.Labels, render.LabelStack, "none")),
			partition: normalizeScopeLabel(labelOr(cfg.Labels, render.LabelPartition, "none")),
			logical:   labelOr(cfg.Labels, render.LabelName, cfg.Name),
		}
		grouped[key] = append(grouped[key], cfg)
	}

	deletes := make([]swarm.Config, 0, len(configs))
	preserved := 0
	for _, group := range grouped {
		ordered := append([]swarm.Config(nil), group...)
		sortConfigsNewestFirst(ordered)
		keep := preserve
		if keep > len(ordered) {
			keep = len(ordered)
		}
		preserved += keep
		deletes = append(deletes, ordered[keep:]...)
	}
	sortConfigsNewestFirst(deletes)
	return deletes, preserved
}

func pruneSecrets(secrets []swarm.Secret, preserve int) ([]swarm.Secret, int) {
	if len(secrets) == 0 {
		return nil, 0
	}
	if preserve <= 0 {
		ordered := append([]swarm.Secret(nil), secrets...)
		sortSecretsNewestFirst(ordered)
		return ordered, 0
	}
	type scopeKey struct {
		project   string
		stack     string
		partition string
		logical   string
	}
	grouped := make(map[scopeKey][]swarm.Secret)
	for _, sec := range secrets {
		key := scopeKey{
			project:   labelOr(sec.Labels, render.LabelProject, ""),
			stack:     normalizeScopeLabel(labelOr(sec.Labels, render.LabelStack, "none")),
			partition: normalizeScopeLabel(labelOr(sec.Labels, render.LabelPartition, "none")),
			logical:   labelOr(sec.Labels, render.LabelName, sec.Name),
		}
		grouped[key] = append(grouped[key], sec)
	}

	deletes := make([]swarm.Secret, 0, len(secrets))
	preserved := 0
	for _, group := range grouped {
		ordered := append([]swarm.Secret(nil), group...)
		sortSecretsNewestFirst(ordered)
		keep := preserve
		if keep > len(ordered) {
			keep = len(ordered)
		}
		preserved += keep
		deletes = append(deletes, ordered[keep:]...)
	}
	sortSecretsNewestFirst(deletes)
	return deletes, preserved
}

func sortConfigsNewestFirst(configs []swarm.Config) {
	sort.Slice(configs, func(i, j int) bool {
		left := configs[i].CreatedAt
		right := configs[j].CreatedAt
		if !left.Equal(right) {
			return left.After(right)
		}
		return configs[i].Name > configs[j].Name
	})
}

func sortSecretsNewestFirst(secrets []swarm.Secret) {
	sort.Slice(secrets, func(i, j int) bool {
		left := secrets[i].CreatedAt
		right := secrets[j].CreatedAt
		if !left.Equal(right) {
			return left.After(right)
		}
		return secrets[i].Name > secrets[j].Name
	})
}

func labelOr(labels map[string]string, key, fallback string) string {
	if labels == nil {
		return fallback
	}
	value := labels[key]
	if value == "" {
		return fallback
	}
	return value
}

func normalizeScopeLabel(value string) string {
	if value == "none" {
		return ""
	}
	return value
}
