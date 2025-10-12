package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/cmmoran/swarmcp/internal/manifest"
	"github.com/cmmoran/swarmcp/internal/reconcile"
	"github.com/cmmoran/swarmcp/internal/render"
	"github.com/cmmoran/swarmcp/internal/swarm"
)

func init() {
	var applyCmd = &cobra.Command{
		Use:   "apply",
		Short: "Apply the current plan",
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

			var cli swarm.Client
			switch driver {
			case "docker":
				dc, err := swarm.NewDockerClient()
				if err != nil {
					return err
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

			// Apply (naive for now)
			_, err = cli.EnsureNetworks(ctx, pl.Networks)
			if err != nil {
				return err
			}
			_, err = cli.EnsureConfigs(ctx, pl.Configs)
			if err != nil {
				return err
			}
			for _, sa := range pl.Services {
				_, _, err := cli.EnsureService(ctx, sa)
				if err != nil {
					return err
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Applied at: %s\n", time.Now().Format(time.RFC3339))
			return nil
		},
	}
	rootCmd.AddCommand(applyCmd)
}
