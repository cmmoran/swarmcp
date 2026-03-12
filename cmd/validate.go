package cmd

import (
	"fmt"

	"github.com/cmmoran/swarmcp/internal/cmdutil"
	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate configuration and templates",
	RunE: func(cmd *cobra.Command, args []string) error {
		configPath, err := primaryConfigPath()
		if err != nil {
			return err
		}
		deployment, err := singleSelector("deployment", opts.Deployments)
		if err != nil {
			return err
		}
		partition, err := singleSelector("partition", opts.Partitions)
		if err != nil {
			return err
		}
		projectOpts := cmdutil.ProjectOptions{
			ConfigPaths:        normalizeConfigPaths(opts.ConfigPaths),
			ReleaseConfigPaths: normalizeConfigPaths(opts.ReleaseConfigs),
			ConfigPath:         configPath,
			Deployment:         deployment,
			Context:            opts.Context,
			Partition:          partition,
			Offline:            opts.Offline,
			Debug:              opts.Debug,
		}
		cfg, _, err := cmdutil.LoadProjectConfig(projectOpts)
		if err != nil {
			return err
		}
		_, err = cmdutil.ResolveProjectScope(cfg, projectOpts)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "config OK")
		return nil
	},
}
