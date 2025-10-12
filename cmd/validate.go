package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cmmoran/swarmcp/internal/manifest"
)

func init() {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate manifests",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := manifest.LoadProject(projectPath)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "manifests OK")
			return nil
		},
	}
	rootCmd.AddCommand(cmd)
}
