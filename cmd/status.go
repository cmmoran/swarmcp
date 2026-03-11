package cmd

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/cmmoran/swarmcp/internal/apply"
	"github.com/cmmoran/swarmcp/internal/cmdutil"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current Swarm status vs desired state",
	RunE: func(cmd *cobra.Command, args []string) error {
		configPaths, err := effectiveProjectConfigPaths()
		if err != nil {
			return err
		}
		releaseConfigPaths := effectiveReleaseConfigPaths()
		configPath := configPaths[0]
		out := cmd.OutOrStdout()
		deployments := deploymentTargets(opts.Deployments)
		partitionFilters := normalizeSelectors(opts.Partitions)
		stackFilters := normalizeSelectors(opts.Stacks)

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
			desired, err := apply.BuildDesiredState(cfg, projectCtx.Secrets, projectCtx.Values, partitionFilters, stackFilters, opts.AllowMissing, !opts.NoInfer)
			if err != nil {
				return err
			}

			client, err := projectCtx.SwarmClient()
			if err != nil {
				return err
			}

			ctx := context.Background()
			preserve, err := resolvePreserve(cmd, cfg, opts)
			if err != nil {
				return err
			}
			report, err := apply.BuildStatus(ctx, client, cfg, desired, projectCtx.Values, partitionFilters, stackFilters, !opts.NoInfer, preserve)
			if err != nil {
				return err
			}

			warnings := cmdutil.VolumePlacementWarnings(cfg, partitionFilters, stackFilters, opts.Debug)
			sortServiceStates(report.Services)
			cmdutil.PrintWarnings(out, warnings)
			_, _ = fmt.Fprintf(out, "status OK\nconfigs missing: %d\nsecrets missing: %d\nnetworks missing: %d\nconfigs stale: %d\nsecrets stale: %d\nconfigs drift: %d\nsecrets drift: %d\nconfigs preserved: %d\nsecrets preserved: %d\nconfigs skipped (in use): %d\nsecrets skipped (in use): %d\n", len(report.MissingConfigs), len(report.MissingSecrets), len(report.MissingNetworks), len(report.StaleConfigs), len(report.StaleSecrets), len(report.DriftConfigs), len(report.DriftSecrets), report.Preserved.ConfigsPreserved, report.Preserved.SecretsPreserved, report.SkippedDeletes.Configs, report.SkippedDeletes.Secrets)
			printServiceSummary(out, report.Services)
			printServiceStates(out, report.Services)
			if opts.Debug {
				printServiceIntentDetails(out, report.Services)
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

func sortServiceStates(states []apply.ServiceState) {
	sort.Slice(states, func(i, j int) bool {
		if states[i].Stack != states[j].Stack {
			return states[i].Stack < states[j].Stack
		}
		if states[i].Partition != states[j].Partition {
			return states[i].Partition < states[j].Partition
		}
		return states[i].Service < states[j].Service
	})
}

func printServiceSummary(out io.Writer, states []apply.ServiceState) {
	var okCount, changedCount, missingCount int
	for _, state := range states {
		if state.Missing {
			missingCount++
			continue
		}
		if state.IntentMatch {
			okCount++
		} else {
			changedCount++
		}
	}
	_, _ = fmt.Fprintf(out, "services ok: %d\nservices changed: %d\nservices missing: %d\n", okCount, changedCount, missingCount)
}

func printServiceStates(out io.Writer, states []apply.ServiceState) {
	if len(states) == 0 {
		return
	}
	_, _ = fmt.Fprintln(out, "services:")
	for _, state := range states {
		_, _ = fmt.Fprintf(out, "  - %s\n", formatServiceState(state))
	}
}

func formatServiceState(state apply.ServiceState) string {
	scope := cmdutil.ServiceScopeLabel(state.Stack, state.Partition, state.Service)
	if state.Missing {
		return fmt.Sprintf("%s missing", scope)
	}
	intent := "ok"
	if !state.IntentMatch {
		intent = "changed"
	}
	diff := ""
	if len(state.IntentDiffs) > 0 {
		diff = " (" + strings.Join(state.IntentDiffs, ", ") + ")"
	}
	unmanaged := ""
	if len(state.Unmanaged) > 0 {
		unmanaged = " unmanaged=(" + strings.Join(state.Unmanaged, ", ") + ")"
	}
	mounts := "ok"
	if !state.MountsMatch {
		mounts = "changed"
	}
	if state.Desired < 0 || state.Running < 0 {
		return fmt.Sprintf("%s intent=%s%s mounts=%s health=%s%s", scope, intent, diff, mounts, state.Health, unmanaged)
	}
	return fmt.Sprintf("%s intent=%s%s mounts=%s health=%s desired=%d running=%d%s", scope, intent, diff, mounts, state.Health, state.Desired, state.Running, unmanaged)
}

func printServiceIntentDetails(out io.Writer, states []apply.ServiceState) {
	printed := false
	for _, state := range states {
		if len(state.IntentDetails) == 0 {
			continue
		}
		if !printed {
			_, _ = fmt.Fprintln(out, "intent details:")
			printed = true
		}
		scope := cmdutil.ServiceScopeLabel(state.Stack, state.Partition, state.Service)
		for _, detail := range state.IntentDetails {
			_, _ = fmt.Fprintf(out, "  - %s %s current=%s desired=%s\n", scope, detail.Field, detail.Current, detail.Desired)
		}
	}
}
