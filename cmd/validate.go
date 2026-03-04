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
		deployment, err := singleSelector("deployment", opts.Deployments)
		if err != nil {
			return err
		}
		partition, err := singleSelector("partition", opts.Partitions)
		if err != nil {
			return err
		}
		_, err = cmdutil.LoadProjectContext(cmdutil.ProjectOptions{
			ConfigPath: opts.ConfigPath,
			Deployment: deployment,
			Context:    opts.Context,
			Partition:  partition,
			Offline:    opts.Offline,
			Debug:      opts.Debug,
		}, false, false)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "config OK")
		return nil
	},
}
