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
		configPaths, err := effectiveProjectConfigPaths()
		if err != nil {
			return err
		}
		releaseConfigPaths := effectiveReleaseConfigPaths()
		configPath := configPaths[0]
		deployment, err := singleSelector("deployment", opts.Deployments)
		if err != nil {
			return err
		}
		partition, err := singleSelector("partition", opts.Partitions)
		if err != nil {
			return err
		}
		_, err = cmdutil.LoadProjectContext(cmdutil.ProjectOptions{
			ConfigPaths:        configPaths,
			ReleaseConfigPaths: releaseConfigPaths,
			ConfigPath:         configPath,
			Deployment:         deployment,
			Context:            opts.Context,
			Partition:          partition,
			Offline:            opts.Offline,
			Debug:              opts.Debug,
		}, false, false)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "config OK")
		return nil
	},
}
