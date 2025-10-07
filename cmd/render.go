package cmd

import (
	"context"
	"fmt"

	"github.com/infamousity/swarmcp/internal/manifest"
	"github.com/infamousity/swarmcp/internal/render"
	"github.com/spf13/cobra"
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
			r := render.NewEngine(render.Options{})
			eff, err := manifest.ResolveEffective(ctx, proj, r)
			if err != nil {
				return err
			}
			count := 0
			for _, st := range eff.Stacks {
				for _, sv := range st.Services {
					count += len(sv.RenderedConfigs)
				}
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "rendered %d config(s) across stacks", count)
			return nil
		},
	}
	rootCmd.AddCommand(cmd)
}
