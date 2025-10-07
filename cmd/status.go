package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show last reconcile status (placeholder)",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "status: not yet implemented")
			return nil
		},
	}
	rootCmd.AddCommand(cmd)
}
