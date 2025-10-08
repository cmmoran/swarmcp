package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cmmoran/swarmcp/internal/manifest"
	"github.com/cmmoran/swarmcp/internal/reconcile"
	"github.com/cmmoran/swarmcp/internal/render"
	"github.com/cmmoran/swarmcp/internal/swarm"
	"github.com/spf13/cobra"
)

var planJSON bool

func init() {
	var planCmd = &cobra.Command{
		Use:   "plan",
		Short: "Compute changes without applying",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			proj, err := manifest.LoadProject(projectPath)
			if err != nil {
				return err
			}
			r := render.NewEngine()
			eff, eferr := manifest.ResolveEffective(ctx, proj, r)
			if eferr != nil {
				return eferr
			}

			var cli swarm.Client
			switch driver {
			case "docker":
				dc, dcerr := swarm.NewDockerClient()
				if dcerr != nil {
					return dcerr
				}
				cli = dc
			default:
				cli = swarm.NewNoopClient()
			}

			rec := reconcile.New(cli)
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
			for _, s := range pl.Summary {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), s)
			}
			return nil
		},
	}
	planCmd.Flags().BoolVar(&planJSON, "json", false, "Output plan as JSON")
	rootCmd.AddCommand(planCmd)
}
