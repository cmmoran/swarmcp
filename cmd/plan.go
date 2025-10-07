package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/infamousity/swarmcp/internal/diff"
	"github.com/infamousity/swarmcp/internal/manifest"
	"github.com/infamousity/swarmcp/internal/reconcile"
	"github.com/infamousity/swarmcp/internal/render"
	"github.com/infamousity/swarmcp/internal/swarm"
	"github.com/infamousity/swarmcp/internal/vault"
	"github.com/spf13/cobra"
)

var planJSON bool

func init() {
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Compute changes without applying",
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
			if planJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(pl)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "— PLAN —")
			printPlan(pl, cmd)
			return nil
		},
	}
	cmd.Flags().BoolVar(&planJSON, "json", false, "Output plan as JSON")
	rootCmd.AddCommand(cmd)
}

func printPlan(pl *diff.Plan, cmd *cobra.Command) {
	for _, c := range pl.Creates {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "create: %s %s", c.Kind, c.Name)
	}
	for _, u := range pl.Updates {
		fmt.Fprintf(cmd.OutOrStdout(), "update: %s %s", u.Kind, u.Name)
	}
	for _, d := range pl.Deletes {
		fmt.Fprintf(cmd.OutOrStdout(), "delete: %s %s", d.Kind, d.Name)
	}
}
