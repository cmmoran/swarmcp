package cmd

import (
	"fmt"
	"strings"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"
)

var explainCmd = &cobra.Command{
	Use:   "explain <field-path>",
	Short: "Explain resolved config provenance for a field",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		configPaths, err := effectiveProjectConfigPaths()
		if err != nil {
			return err
		}
		deployment, err := singleSelector("deployment", opts.Deployments)
		if err != nil {
			return fmt.Errorf("explain is single-target only: %w", err)
		}
		partition, err := singleSelector("partition", opts.Partitions)
		if err != nil {
			return fmt.Errorf("explain is single-target only: %w", err)
		}
		stack, err := singleSelector("stack", opts.Stacks)
		if err != nil {
			return fmt.Errorf("explain is single-target only: %w", err)
		}
		result, err := config.ExplainConfigPath(config.ExplainOptions{
			ConfigPaths:        configPaths,
			ReleaseConfigPaths: effectiveReleaseConfigPaths(),
			Deployment:         deployment,
			Partition:          partition,
			Stack:              stack,
			LoadOptions:        config.LoadOptions{Offline: opts.Offline, Debug: opts.Debug},
		}, args[0])
		if err != nil {
			return err
		}

		out := cmd.OutOrStdout()
		_, _ = fmt.Fprintln(out, "explain OK")
		_, _ = fmt.Fprintf(out, "path: %s\n", result.Path)
		_, _ = fmt.Fprintln(out, "final:")
		if err := printExplainValue(out, result.Final); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out, "layers:")
		for i, layer := range result.Layers {
			_, _ = fmt.Fprintf(out, "%d. %s\n", i+1, layer.Label)
			if err := printExplainValue(out, layer.Value); err != nil {
				return err
			}
		}
		_, _ = fmt.Fprintln(out, "winner:")
		_, _ = fmt.Fprintf(out, "- %s\n", result.Winner)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(explainCmd)
}

func printExplainValue(out interface{ Write([]byte) (int, error) }, value any) error {
	encoded, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(strings.TrimRight(string(encoded), "\n"), "\n") {
		_, _ = out.Write([]byte("  " + line + "\n"))
	}
	return nil
}
