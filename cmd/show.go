package cmd

import (
	"fmt"

	"github.com/cmmoran/swarmcp/internal/apply"
	"github.com/spf13/cobra"
)

var showCmd = &cobra.Command{
	Use:   "show plan-file",
	Short: "Show a saved SwarmCP plan",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		planFile, err := apply.ReadPlanFile(args[0])
		if err != nil {
			return err
		}
		if err := apply.ValidatePlanFile(planFile); err != nil {
			return err
		}
		printPlanFileSummary(cmd.OutOrStdout(), args[0], planFile)
		return nil
	},
}

func printPlanFileSummary(out interface {
	Write([]byte) (int, error)
}, path string, planFile apply.PlanFile) {
	planSummary := buildPlanSummary(planFile.Plan)
	stackNames, serviceCreates, serviceUpdates := planDeploySummary(planFile.Plan.StackDeploys)
	planSummary.StackNames = stackNames
	planSummary.ServicesCreated = serviceCreates
	planSummary.ServicesUpdated = serviceUpdates

	_, _ = fmt.Fprintln(out, "show OK")
	_, _ = fmt.Fprintf(out, "plan artifact: %s\n", path)
	_, _ = fmt.Fprintf(out, "api version: %s\n", planFile.APIVersion)
	if planFile.ToolVersion != "" {
		_, _ = fmt.Fprintf(out, "tool version: %s\n", planFile.ToolVersion)
	}
	if planFile.GeneratedAt != "" {
		_, _ = fmt.Fprintf(out, "generated at: %s\n", planFile.GeneratedAt)
	}
	_, _ = fmt.Fprintf(out, "project: %s\n", planFile.Project)
	if planFile.Deployment != "" {
		_, _ = fmt.Fprintf(out, "deployment: %s\n", planFile.Deployment)
	}
	if planFile.Partition != "" {
		_, _ = fmt.Fprintf(out, "partition: %s\n", planFile.Partition)
	}
	if planFile.Stack != "" {
		_, _ = fmt.Fprintf(out, "stack: %s\n", planFile.Stack)
	}
	if planFile.Context != "" {
		_, _ = fmt.Fprintf(out, "context: %s\n", planFile.Context)
	}
	_, _ = fmt.Fprintf(out, "secret mode: %s\n", apply.NormalizedPlanSecretMode(planFile))
	_, _ = fmt.Fprintf(out, "inputs: %d\n", len(planFile.Inputs))
	_, _ = fmt.Fprintf(out, "networks to create: %d\nconfigs to create: %d\nsecrets to create: %d\nstacks to deploy: %d\nconfigs to delete: %d\nsecrets to delete: %d\nconfigs skipped (in use): %d\nsecrets skipped (in use): %d\n", planSummary.NetworksCreated, planSummary.ConfigsCreated, planSummary.SecretsCreated, planSummary.StacksDeployed, planSummary.ConfigsRemoved, planSummary.SecretsRemoved, planSummary.ConfigsSkipped, planSummary.SecretsSkipped)
	if len(stackNames) > 0 {
		_, _ = fmt.Fprintln(out, "stacks:")
		for _, name := range stackNames {
			_, _ = fmt.Fprintf(out, "  - %s\n", name)
		}
	}
	if len(planFile.SecretSources) > 0 {
		_, _ = fmt.Fprintln(out, "secret sources:")
		for _, source := range planFile.SecretSources {
			_, _ = fmt.Fprintf(out, "  - %s (%s)\n", source.SecretName, pluralCount(len(source.Dependencies), "dependency", "dependencies"))
		}
	}
}

func pluralCount(count int, singular string, plural string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, singular)
	}
	return fmt.Sprintf("%d %s", count, plural)
}
