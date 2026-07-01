package cmd

import (
	"context"

	"github.com/cmmoran/swarmcp/internal/apply"
	"github.com/cmmoran/swarmcp/internal/cmdutil"
	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/spf13/cobra"
)

func buildApplyPlanArtifact(cmd *cobra.Command, projectCtx *cmdutil.ProjectContext, cfg *config.Config, desired apply.DesiredState, partitionFilters []string, stackFilters []string, opts Options) (apply.Plan, bool, error) {
	client, err := projectCtx.SwarmClient()
	if err != nil {
		return apply.Plan{}, false, err
	}
	plan, err := apply.BuildPlan(context.Background(), client, cfg, desired, projectCtx.Values, partitionFilters, stackFilters, !opts.NoInfer)
	if err != nil {
		return apply.Plan{}, false, err
	}

	if opts.Prune {
		preserve, err := resolvePreserve(cmd, cfg, opts)
		if err != nil {
			return apply.Plan{}, false, err
		}
		plan, _ = apply.PrunePlan(plan, preserve)
	} else {
		plan.DeleteConfigs = nil
		plan.DeleteSecrets = nil
	}

	pruneServices := opts.Prune || opts.PruneServices
	if pruneServices {
		existingDeploys := make(map[string]struct{}, len(plan.StackDeploys))
		for _, deploy := range plan.StackDeploys {
			existingDeploys[deploy.Name] = struct{}{}
		}
		pruneOnly := make(map[string]struct{})
		for _, name := range plan.PruneStacks {
			if _, ok := existingDeploys[name]; ok {
				continue
			}
			pruneOnly[name] = struct{}{}
		}
		if len(pruneOnly) > 0 {
			pruneDeploys, err := apply.BuildStackDeploys(cfg, desired, projectCtx.Values, partitionFilters, stackFilters, pruneOnly, nil, nil, !opts.NoInfer)
			if err != nil {
				return apply.Plan{}, false, err
			}
			plan.StackDeploys = mergeStackDeploys(plan.StackDeploys, pruneDeploys)
		}
	}

	plan = apply.FinalizePlanAssumptions(plan)

	return plan, pruneServices, nil
}
