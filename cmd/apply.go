package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cmmoran/swarmcp/internal/apply"
	"github.com/cmmoran/swarmcp/internal/cmdutil"
	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/state"
	"github.com/spf13/cobra"
)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply desired state to Swarm",
	RunE: func(cmd *cobra.Command, args []string) error {
		configPaths, err := effectiveProjectConfigPaths()
		if err != nil {
			return err
		}
		releaseConfigPaths := effectiveReleaseConfigPaths()
		configPath := configPaths[0]
		deployments := deploymentTargets(opts.Deployments)
		partitionFilters := normalizeSelectors(opts.Partitions)
		stackFilters := normalizeSelectors(opts.Stacks)
		outputMode := strings.TrimSpace(opts.Output)
		if outputMode == "" {
			outputMode = "auto"
		}
		if err := apply.ValidateDeployOutputMode(outputMode); err != nil {
			return err
		}
		outputFlagSet := cmd.Flags().Changed("output")
		noUI := opts.NoUI || outputFlagSet
		out := cmd.OutOrStdout()

		for deploymentIndex, deployment := range deployments {
			if len(deployments) > 1 {
				if deploymentIndex > 0 {
					_, _ = fmt.Fprintln(out)
				}
				label := deployment
				if label == "" {
					label = "(default)"
				}
				_, _ = fmt.Fprintf(out, "target deployment: %s\n", label)
			}

			projectCtx, err := cmdutil.LoadProjectContext(cmdutil.ProjectOptions{
				ConfigPaths:        configPaths,
				ReleaseConfigPaths: releaseConfigPaths,
				ConfigPath:         configPath,
				SecretsFile:        opts.SecretsFile,
				ValuesFiles:        opts.ValuesFiles,
				Deployment:         deployment,
				Context:            opts.Context,
				Offline:            opts.Offline,
				Debug:              opts.Debug,
			}, true, true)
			if err != nil {
				return err
			}
			cfg := projectCtx.Config
			for _, partition := range partitionFilters {
				if !cmdutil.PartitionInProject(cfg, partition) {
					return fmt.Errorf("partition %q not found in project.partitions", partition)
				}
			}
			for _, stack := range stackFilters {
				if !cmdutil.StackInProject(cfg, stack) {
					return fmt.Errorf("stack %q not found in stacks", stack)
				}
			}

			values := projectCtx.Values
			pruneServices := opts.Prune || opts.PruneServices
			desired, err := apply.BuildDesiredState(cfg, projectCtx.Secrets, values, partitionFilters, stackFilters, opts.AllowMissing, !opts.NoInfer)
			if err != nil {
				return err
			}

			contextName := projectCtx.ContextName
			client, err := projectCtx.SwarmClient()
			if err != nil {
				return err
			}

			ctx := context.Background()
			plan, err := apply.BuildPlan(ctx, client, cfg, desired, values, partitionFilters, stackFilters, !opts.NoInfer)
			if err != nil {
				return err
			}

			prune := opts.Prune
			preserve, err := resolvePreserve(cmd, cfg, opts)
			if err != nil {
				return err
			}
			var pruneResult apply.PruneResult
			if prune {
				plan, pruneResult = apply.PrunePlan(plan, preserve)
				if opts.Confirm {
					confirmed, err := confirmPruneConfigs(cmd, plan, preserve, opts.Confirm)
					if err != nil {
						return err
					}
					if !confirmed {
						prune = false
						plan.DeleteConfigs = nil
						plan.DeleteSecrets = nil
					}
				}
			} else {
				plan.DeleteConfigs = nil
				plan.DeleteSecrets = nil
			}
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
					pruneDeploys, err := apply.BuildStackDeploys(cfg, desired, values, partitionFilters, stackFilters, pruneOnly, nil, nil, !opts.NoInfer)
					if err != nil {
						return err
					}
					plan.StackDeploys = mergeStackDeploys(plan.StackDeploys, pruneDeploys)
				}
			}
			if pruneServices && len(plan.StackDeploys) > 0 && opts.Confirm {
				confirmed, err := confirmPruneServices(cmd, len(plan.StackDeploys), opts.Confirm)
				if err != nil {
					return err
				}
				if !confirmed {
					pruneServices = false
				}
			}

			planSummary := buildPlanSummary(plan)
			partitionState := ""
			if len(partitionFilters) == 1 {
				partitionState = partitionFilters[0]
			}
			stackState := ""
			if len(stackFilters) == 1 {
				stackState = stackFilters[0]
			}
			skipCache := len(deployments) > 1 || len(partitionFilters) > 1 || len(stackFilters) > 1
			cached, cacheOK := loadStateCache(configPath, cfg, partitionState, stackState)
			skipApply := !skipCache && cacheOK && cached.Command == "apply" && planSummaryZero(planSummary) && planSummariesEqual(cached.Plan, planSummary)
			if !skipApply {
				stackParallel := 0
				if opts.Serial {
					stackParallel = 1
				}
				if err := apply.Apply(ctx, client, plan, contextName, pruneServices, stackParallel, noUI, outputMode, outputFlagSet); err != nil {
					return err
				}
			}

			statePath, err := planStatePath(configPath)
			if err != nil {
				return err
			}
			stackNames, serviceCreates, serviceUpdates := planDeploySummary(plan.StackDeploys)
			if len(stackNames) == 0 {
				stackNames = nil
			}
			planSummary.ServicesCreated = serviceCreates
			planSummary.ServicesUpdated = serviceUpdates
			planSummary.StackNames = stackNames
			stateSnapshot := state.State{
				Version:     state.CurrentVersion,
				GeneratedAt: time.Now().UTC().Format(time.RFC3339),
				Command:     "apply",
				ConfigPath:  configPath,
				Project:     cfg.Project.Name,
				Deployment:  cfg.Project.Deployment,
				Partition:   partitionState,
				Stack:       stackState,
				Plan:        planSummary,
			}
			if err := state.Write(statePath, stateSnapshot); err != nil {
				return err
			}

			if skipApply {
				_, _ = fmt.Fprintln(out, "apply OK (no changes)")
			} else {
				_, _ = fmt.Fprintln(out, "apply OK")
			}
			_, _ = fmt.Fprintf(out, "networks created: %d\nconfigs created: %d\nsecrets created: %d\nstacks deployed: %d\nconfigs removed: %d\nsecrets removed: %d\nconfigs skipped (in use): %d\nsecrets skipped (in use): %d\n", planSummary.NetworksCreated, planSummary.ConfigsCreated, planSummary.SecretsCreated, planSummary.StacksDeployed, planSummary.ConfigsRemoved, planSummary.SecretsRemoved, planSummary.ConfigsSkipped, planSummary.SecretsSkipped)
			if prune {
				_, _ = fmt.Fprintf(out, "prune enabled: preserve=%d configs preserved: %d secrets preserved: %d\n", pruneResult.PreserveCount, pruneResult.ConfigsPreserved, pruneResult.SecretsPreserved)
			} else {
				_, _ = fmt.Fprintln(out, "prune disabled: unused configs/secrets preserved")
			}
			if pruneServices {
				_, _ = fmt.Fprintln(out, "prune services enabled: stack deploy uses --prune")
			} else {
				_, _ = fmt.Fprintln(out, "prune services disabled")
			}
			if len(desired.Missing) > 0 {
				sort.Strings(desired.Missing)
				_, _ = fmt.Fprintf(out, "missing secrets (placeholders): %d\n", len(desired.Missing))
				for _, item := range desired.Missing {
					_, _ = fmt.Fprintf(out, "  - %s\n", item)
				}
			}
		}
		return nil
	},
}

func init() {
	applyCmd.Flags().BoolVar(&opts.Serial, "serial", false, "Deploy stacks one at a time during apply")
	applyCmd.Flags().BoolVar(&opts.NoUI, "no-ui", false, "Disable stack deployment UI and emit buffered output per stack")
	applyCmd.Flags().StringVar(&opts.Output, "output", "auto", "Deploy output mode for apply: auto|summary|stack|error-only (explicitly setting this implies --no-ui)")
}

func planStatePath(configPath string) (string, error) {
	if strings.TrimSpace(configPath) == "" {
		return "", fmt.Errorf("config path is required to write state")
	}
	base := filepath.Base(configPath)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	if name == "" {
		return "", fmt.Errorf("config filename is required to write state")
	}
	dir := filepath.Dir(configPath)
	stateDir := filepath.Join(dir, ".swarmcp")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(stateDir, name+".state"), nil
}

func planDeploySummary(deploys []apply.StackDeploy) ([]string, int, int) {
	stacks := make([]string, 0, len(deploys))
	serviceCreates := 0
	serviceUpdates := 0
	for _, deploy := range deploys {
		stacks = append(stacks, deploy.Name)
		serviceCreates += deploy.ServiceCreates
		serviceUpdates += deploy.ServiceUpdates
	}
	return stacks, serviceCreates, serviceUpdates
}

func mergeStackDeploys(primary, extra []apply.StackDeploy) []apply.StackDeploy {
	if len(extra) == 0 {
		return primary
	}
	if len(primary) == 0 {
		return extra
	}
	byName := make(map[string]apply.StackDeploy, len(primary)+len(extra))
	for _, deploy := range extra {
		byName[deploy.Name] = deploy
	}
	for _, deploy := range primary {
		byName[deploy.Name] = deploy
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]apply.StackDeploy, 0, len(names))
	for _, name := range names {
		out = append(out, byName[name])
	}
	return out
}

func buildPlanSummary(plan apply.Plan) state.PlanSummary {
	return state.PlanSummary{
		NetworksCreated: len(plan.CreateNetworks),
		ConfigsCreated:  len(plan.CreateConfigs),
		SecretsCreated:  len(plan.CreateSecrets),
		StacksDeployed:  len(plan.StackDeploys),
		ConfigsRemoved:  len(plan.DeleteConfigs),
		SecretsRemoved:  len(plan.DeleteSecrets),
		ConfigsSkipped:  plan.SkippedDeletes.Configs,
		SecretsSkipped:  plan.SkippedDeletes.Secrets,
	}
}

func planSummaryZero(summary state.PlanSummary) bool {
	return summary.NetworksCreated == 0 &&
		summary.ConfigsCreated == 0 &&
		summary.SecretsCreated == 0 &&
		summary.StacksDeployed == 0 &&
		summary.ConfigsRemoved == 0 &&
		summary.SecretsRemoved == 0 &&
		summary.ConfigsSkipped == 0 &&
		summary.SecretsSkipped == 0
}

func planSummariesEqual(left, right state.PlanSummary) bool {
	return left.NetworksCreated == right.NetworksCreated &&
		left.ConfigsCreated == right.ConfigsCreated &&
		left.SecretsCreated == right.SecretsCreated &&
		left.StacksDeployed == right.StacksDeployed &&
		left.ConfigsRemoved == right.ConfigsRemoved &&
		left.SecretsRemoved == right.SecretsRemoved &&
		left.ConfigsSkipped == right.ConfigsSkipped &&
		left.SecretsSkipped == right.SecretsSkipped
}

func loadStateCache(configPath string, cfg *config.Config, partition string, stack string) (state.State, bool) {
	statePath, err := planStatePath(configPath)
	if err != nil {
		return state.State{}, false
	}
	cached, err := state.Read(statePath)
	if err != nil {
		return state.State{}, false
	}
	if cached.ConfigPath != "" && cached.ConfigPath != configPath {
		return state.State{}, false
	}
	if cached.Project != "" && cached.Project != cfg.Project.Name {
		return state.State{}, false
	}
	if cached.Deployment != "" && cached.Deployment != cfg.Project.Deployment {
		return state.State{}, false
	}
	if cached.Partition != "" && cached.Partition != partition {
		return state.State{}, false
	}
	if cached.Stack != "" && cached.Stack != stack {
		return state.State{}, false
	}
	return cached, true
}
