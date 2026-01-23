package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/cmmoran/swarmcp/internal/apply"
	"github.com/cmmoran/swarmcp/internal/cmdutil"
	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/state"
	"github.com/cmmoran/swarmcp/internal/swarm"
	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show planned changes vs current Swarm state",
	RunE: func(cmd *cobra.Command, args []string) error {
		projectCtx, err := cmdutil.LoadProjectContext(cmdutil.ProjectOptions{
			ConfigPath:  opts.ConfigPath,
			SecretsFile: opts.SecretsFile,
			ValuesFiles: opts.ValuesFiles,
			Deployment:  opts.Deployment,
			Context:     opts.Context,
			Partition:   opts.Partition,
			Offline:     opts.Offline,
			Debug:       opts.Debug,
		}, true, true)
		if err != nil {
			return err
		}
		cfg := projectCtx.Config
		partitionFilter := projectCtx.Partition
		desired, err := apply.BuildDesiredState(cfg, projectCtx.Secrets, projectCtx.Values, partitionFilter, opts.AllowMissing, !opts.NoInfer)
		if err != nil {
			return err
		}

		client, err := projectCtx.SwarmClient()
		if err != nil {
			return err
		}

		ctx := context.Background()
		preserve := config.PreserveUnusedResources(cfg)
		if flag := cmd.Flags().Lookup("preserve"); flag != nil && flag.Changed {
			preserve = opts.Preserve
		}
		if preserve < 0 {
			return fmt.Errorf("preserve must be >= 0")
		}
		report, err := apply.BuildStatus(ctx, client, cfg, desired, projectCtx.Values, partitionFilter, !opts.NoInfer, preserve)
		if err != nil {
			return err
		}

		warnings := cmdutil.VolumePlacementWarnings(cfg, partitionFilter, opts.Debug)
		sortServiceStates(report.Services)
		sortConfigSpecs(report.MissingConfigs)
		sortSecretSpecs(report.MissingSecrets)
		sortConfigs(report.StaleConfigs)
		sortSecrets(report.StaleSecrets)
		sortDriftItems(report.DriftConfigs)
		sortDriftItems(report.DriftSecrets)

		changedServices, missingServices := splitServiceStates(report.Services)

		out := cmd.OutOrStdout()
		cmdutil.PrintWarnings(out, warnings)
		fmt.Fprintf(out, "diff OK\nconfigs to create: %d\nsecrets to create: %d\nnetworks to create: %d\nconfigs to delete: %d\nsecrets to delete: %d\nconfigs preserved: %d\nsecrets preserved: %d\nconfigs skipped (in use): %d\nsecrets skipped (in use): %d\nservices to update: %d\nservices missing: %d\n", len(report.MissingConfigs), len(report.MissingSecrets), len(report.MissingNetworks), len(report.StaleConfigs), len(report.StaleSecrets), report.Preserved.ConfigsPreserved, report.Preserved.SecretsPreserved, report.SkippedDeletes.Configs, report.SkippedDeletes.Secrets, len(changedServices), len(missingServices))

		if len(report.MissingConfigs) > 0 {
			fmt.Fprintln(out, "configs to create:")
			for _, cfg := range report.MissingConfigs {
				fmt.Fprintf(out, "  - %s\n", cmdutil.FormatConfigItem(cfg.Name, cfg.Labels))
			}
		}
		if len(report.MissingSecrets) > 0 {
			fmt.Fprintln(out, "secrets to create:")
			for _, sec := range report.MissingSecrets {
				fmt.Fprintf(out, "  - %s\n", cmdutil.FormatConfigItem(sec.Name, sec.Labels))
			}
		}
		if len(report.StaleConfigs) > 0 {
			fmt.Fprintln(out, "configs to delete:")
			for _, cfg := range report.StaleConfigs {
				fmt.Fprintf(out, "  - %s\n", cmdutil.FormatConfigItem(cfg.Name, cfg.Labels))
			}
		}
		if len(report.StaleSecrets) > 0 {
			fmt.Fprintln(out, "secrets to delete:")
			for _, sec := range report.StaleSecrets {
				fmt.Fprintf(out, "  - %s\n", cmdutil.FormatConfigItem(sec.Name, sec.Labels))
			}
		}
		if len(report.DriftConfigs) > 0 {
			fmt.Fprintln(out, "configs with label drift:")
			for _, item := range report.DriftConfigs {
				fmt.Fprintf(out, "  - %s (%s)\n", cmdutil.FormatConfigItem(item.Name, item.Labels), item.Reason)
			}
		}
		if len(report.DriftSecrets) > 0 {
			fmt.Fprintln(out, "secrets with label drift:")
			for _, item := range report.DriftSecrets {
				fmt.Fprintf(out, "  - %s (%s)\n", cmdutil.FormatConfigItem(item.Name, item.Labels), item.Reason)
			}
		}
		if len(report.MissingNetworks) > 0 {
			fmt.Fprintln(out, "networks to create:")
			for _, net := range report.MissingNetworks {
				fmt.Fprintf(out, "  - %s\n", net.Name)
			}
		}
		if len(changedServices) > 0 {
			fmt.Fprintln(out, "services to update:")
			for _, state := range changedServices {
				line := cmdutil.ServiceScopeLabel(state.Stack, state.Partition, state.Service)
				if len(state.IntentDiffs) > 0 {
					line += " (" + strings.Join(state.IntentDiffs, ", ") + ")"
				}
				fmt.Fprintf(out, "  - %s\n", line)
			}
		}
		if unmanaged := unmanagedServiceStates(report.Services); len(unmanaged) > 0 {
			fmt.Fprintln(out, "services with unmanaged drift:")
			for _, state := range unmanaged {
				line := cmdutil.ServiceScopeLabel(state.Stack, state.Partition, state.Service)
				line += " (" + strings.Join(state.Unmanaged, ", ") + ")"
				fmt.Fprintf(out, "  - %s\n", line)
			}
		}
		if len(missingServices) > 0 {
			fmt.Fprintln(out, "services missing:")
			for _, state := range missingServices {
				fmt.Fprintf(out, "  - %s\n", cmdutil.ServiceScopeLabel(state.Stack, state.Partition, state.Service))
			}
		}
		if err := printStateDiff(out, opts.ConfigPath, report, changedServices, missingServices); err != nil {
			return err
		}
		return nil
	},
}

func sortConfigSpecs(items []swarm.ConfigSpec) {
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
}

func sortSecretSpecs(items []swarm.SecretSpec) {
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
}

func sortConfigs(items []swarm.Config) {
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
}

func sortSecrets(items []swarm.Secret) {
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
}

func splitServiceStates(states []apply.ServiceState) ([]apply.ServiceState, []apply.ServiceState) {
	var changed []apply.ServiceState
	var missing []apply.ServiceState
	for _, state := range states {
		if state.Missing {
			missing = append(missing, state)
			continue
		}
		if !state.IntentMatch {
			changed = append(changed, state)
		}
	}
	return changed, missing
}

func unmanagedServiceStates(states []apply.ServiceState) []apply.ServiceState {
	var unmanaged []apply.ServiceState
	for _, state := range states {
		if state.Missing {
			continue
		}
		if len(state.Unmanaged) > 0 {
			unmanaged = append(unmanaged, state)
		}
	}
	sort.Slice(unmanaged, func(i, j int) bool {
		if unmanaged[i].Stack != unmanaged[j].Stack {
			return unmanaged[i].Stack < unmanaged[j].Stack
		}
		if unmanaged[i].Partition != unmanaged[j].Partition {
			return unmanaged[i].Partition < unmanaged[j].Partition
		}
		return unmanaged[i].Service < unmanaged[j].Service
	})
	return unmanaged
}

func sortDriftItems(items []apply.DriftItem) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Name != items[j].Name {
			return items[i].Name < items[j].Name
		}
		return items[i].Reason < items[j].Reason
	})
}

func printStateDiff(out io.Writer, configPath string, report apply.StatusReport, changedServices, missingServices []apply.ServiceState) error {
	statePath, err := planStatePath(configPath)
	if err != nil {
		return err
	}
	cached, err := state.Read(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	fmt.Fprintf(out, "state cache: %s %s\n", cached.Command, cached.GeneratedAt)
	if cached.Command != "apply" {
		fmt.Fprintln(out, "state drift: skipped (last command was plan)")
		return nil
	}
	if cached.ConfigPath != "" && cached.ConfigPath != configPath {
		fmt.Fprintf(out, "state drift: skipped (config path mismatch: %s)\n", cached.ConfigPath)
		return nil
	}
	current := state.PlanSummary{
		NetworksCreated: len(report.MissingNetworks),
		ConfigsCreated:  len(report.MissingConfigs),
		SecretsCreated:  len(report.MissingSecrets),
		ConfigsRemoved:  len(report.StaleConfigs),
		SecretsRemoved:  len(report.StaleSecrets),
		ConfigsSkipped:  report.SkippedDeletes.Configs,
		SecretsSkipped:  report.SkippedDeletes.Secrets,
		ServicesCreated: len(missingServices),
		ServicesUpdated: len(changedServices),
		StacksDeployed:  uniqueServiceStacks(changedServices, missingServices),
	}
	deltas := diffStateSummary(cached.Plan, current)
	if len(deltas) == 0 {
		fmt.Fprintln(out, "state drift: none")
		return nil
	}
	fmt.Fprintln(out, "state drift:")
	for _, delta := range deltas {
		fmt.Fprintf(out, "  - %s: state=%d current=%d\n", delta.label, delta.stateValue, delta.currentValue)
	}
	return nil
}

type summaryDelta struct {
	label        string
	stateValue   int
	currentValue int
}

func diffStateSummary(previous, current state.PlanSummary) []summaryDelta {
	var deltas []summaryDelta
	appendDelta := func(label string, prev, cur int) {
		if prev != cur {
			deltas = append(deltas, summaryDelta{label: label, stateValue: prev, currentValue: cur})
		}
	}
	appendDelta("configs to create", previous.ConfigsCreated, current.ConfigsCreated)
	appendDelta("secrets to create", previous.SecretsCreated, current.SecretsCreated)
	appendDelta("networks to create", previous.NetworksCreated, current.NetworksCreated)
	appendDelta("configs to delete", previous.ConfigsRemoved, current.ConfigsRemoved)
	appendDelta("secrets to delete", previous.SecretsRemoved, current.SecretsRemoved)
	appendDelta("configs skipped", previous.ConfigsSkipped, current.ConfigsSkipped)
	appendDelta("secrets skipped", previous.SecretsSkipped, current.SecretsSkipped)
	appendDelta("services to create", previous.ServicesCreated, current.ServicesCreated)
	appendDelta("services to update", previous.ServicesUpdated, current.ServicesUpdated)
	appendDelta("stacks to deploy", previous.StacksDeployed, current.StacksDeployed)
	return deltas
}

func uniqueServiceStacks(changed, missing []apply.ServiceState) int {
	stacks := make(map[string]struct{})
	for _, state := range changed {
		stacks[state.Stack+"|"+state.Partition] = struct{}{}
	}
	for _, state := range missing {
		stacks[state.Stack+"|"+state.Partition] = struct{}{}
	}
	return len(stacks)
}
