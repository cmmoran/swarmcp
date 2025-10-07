package cmd

import (
	"context"
	"time"

	"github.com/infamousity/swarmcp/internal/manifest"
	"github.com/infamousity/swarmcp/internal/reconcile"
	"github.com/infamousity/swarmcp/internal/render"
	"github.com/infamousity/swarmcp/internal/status"
	"github.com/infamousity/swarmcp/internal/swarm"
	"github.com/infamousity/swarmcp/internal/vault"
	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply the current plan",
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
			rec := reconcile.New(swarm.NewNoopClient(), vault.NewNoopClient())
			pl, err := rec.Plan(ctx, eff)
			if err != nil {
				return err
			}
			rep, err := rec.Apply(ctx, pl)
			if err != nil {
				return err
			}
			rep.LastAppliedAt = time.Now()
			status.PrintReport(cmd.OutOrStdout(), rep)
			return nil
		},
	}
	rootCmd.AddCommand(cmd)
}
