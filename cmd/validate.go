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
		_, err := cmdutil.LoadProjectContext(cmdutil.ProjectOptions{
			ConfigPath: opts.ConfigPath,
			Deployment: opts.Deployment,
			Context:    opts.Context,
			Partition:  opts.Partition,
			Offline:    opts.Offline,
		}, false, false)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "config OK")
		return nil
	},
}
