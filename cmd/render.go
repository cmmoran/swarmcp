package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cmmoran/swarmcp/internal/manifest"
	"github.com/cmmoran/swarmcp/internal/render"
)

func init() {
	cmd := &cobra.Command{
		Use:   "render",
		Short: "Render templates to stdout (summary)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			proj, err := manifest.LoadProject(projectPath)
			if err != nil {
				return err
			}
			r := render.NewEngine()
			eff, err := manifest.ResolveEffective(ctx, proj, r)
			if err != nil {
				return err
			}
			countCfg := 0
			countSec := 0
			for _, st := range eff.Stacks {
				for _, sv := range st.Services {
					countCfg += len(sv.Configs)
					countSec += len(sv.Secrets)
				}
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "rendered %d config(s) and %d secret(s) across stacks", countCfg, countSec)
			return nil
		},
	}
	rootCmd.AddCommand(cmd)
}
