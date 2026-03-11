package cmd

import (
	"context"
	"fmt"
	"sort"

	"github.com/cmmoran/swarmcp/internal/apply"
	"github.com/cmmoran/swarmcp/internal/cmdutil"
	"github.com/cmmoran/swarmcp/internal/swarm"
	"github.com/spf13/cobra"
)

var bootstrapCmd = &cobra.Command{
	Use:   "bootstrap",
	Short: "Run non-destructive setup steps",
}

var bootstrapNetworksCmd = &cobra.Command{
	Use:   "networks",
	Short: "Create required overlay networks for the project",
	RunE: func(cmd *cobra.Command, args []string) error {
		configPaths, err := effectiveProjectConfigPaths()
		if err != nil {
			return err
		}
		releaseConfigPaths := effectiveReleaseConfigPaths()
		configPath := configPaths[0]
		deployment, err := singleSelector("deployment", opts.Deployments)
		if err != nil {
			return err
		}
		partition, err := singleSelector("partition", opts.Partitions)
		if err != nil {
			return err
		}
		projectCtx, err := cmdutil.LoadProjectContext(cmdutil.ProjectOptions{
			ConfigPaths:        configPaths,
			ReleaseConfigPaths: releaseConfigPaths,
			ConfigPath:         configPath,
			Deployment:         deployment,
			Context:            opts.Context,
			Partition:          partition,
			Offline:            opts.Offline,
			Debug:              opts.Debug,
		}, false, false)
		if err != nil {
			return err
		}
		cfg := projectCtx.Config
		partitionFilters := normalizeSelectors(opts.Partitions)
		stackFilters := normalizeSelectors(opts.Stacks)

		client, err := projectCtx.SwarmClient()
		if err != nil {
			return err
		}

		desired := apply.DesiredNetworks(cfg, partitionFilters, stackFilters)
		if len(desired) == 0 {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "bootstrap networks OK\nnetworks to create: 0")
			return nil
		}

		existing, err := client.ListNetworks(context.Background())
		if err != nil {
			return err
		}
		existingNames := make(map[string]struct{}, len(existing))
		for _, network := range existing {
			existingNames[network.Name] = struct{}{}
		}

		var toCreate []swarm.NetworkSpec
		for _, network := range desired {
			if _, ok := existingNames[network.Name]; ok {
				continue
			}
			toCreate = append(toCreate, network)
		}

		for _, network := range toCreate {
			if _, err := client.CreateNetwork(context.Background(), network); err != nil {
				return err
			}
		}

		out := cmd.OutOrStdout()
		_, _ = fmt.Fprintf(out, "bootstrap networks OK\nnetworks to create: %d\n", len(toCreate))
		if len(toCreate) > 0 {
			sort.Slice(toCreate, func(i, j int) bool { return toCreate[i].Name < toCreate[j].Name })
			_, _ = fmt.Fprintln(out, "networks created:")
			for _, network := range toCreate {
				_, _ = fmt.Fprintf(out, "  - %s\n", network.Name)
			}
		}
		return nil
	},
}

var bootstrapLabelsCmd = &cobra.Command{
	Use:   "labels",
	Short: "Ensure node labels match project config",
	RunE: func(cmd *cobra.Command, args []string) error {
		configPaths, err := effectiveProjectConfigPaths()
		if err != nil {
			return err
		}
		releaseConfigPaths := effectiveReleaseConfigPaths()
		configPath := configPaths[0]
		deployment, err := singleSelector("deployment", opts.Deployments)
		if err != nil {
			return err
		}
		partition, err := singleSelector("partition", opts.Partitions)
		if err != nil {
			return err
		}
		projectCtx, err := cmdutil.LoadProjectContext(cmdutil.ProjectOptions{
			ConfigPaths:        configPaths,
			ReleaseConfigPaths: releaseConfigPaths,
			ConfigPath:         configPath,
			Deployment:         deployment,
			Context:            opts.Context,
			Partition:          partition,
			Offline:            opts.Offline,
			Debug:              opts.Debug,
		}, false, false)
		if err != nil {
			return err
		}
		cfg := projectCtx.Config

		nodesInScope := cmdutil.ResolveDeploymentNodeSpecs(cfg)
		var updated []string
		var missing []string
		if len(nodesInScope) > 0 {
			client, err := projectCtx.SwarmClient()
			if err != nil {
				return err
			}

			nodes, err := client.ListNodes(context.Background())
			if err != nil {
				return err
			}
			index := make(map[string]swarm.Node, len(nodes)*2)
			for _, node := range nodes {
				if node.Name != "" {
					index[node.Name] = node
				}
				if node.Hostname != "" {
					index[node.Hostname] = node
				}
			}

			for name, node := range nodesInScope {
				actual, ok := index[name]
				if !ok {
					missing = append(missing, name)
					continue
				}
				desired := cmdutil.DesiredNodeLabels(cfg, node)
				if len(desired) == 0 {
					continue
				}
				next := actual.Spec
				if next.Labels == nil {
					next.Labels = make(map[string]string, len(desired))
				}
				changed := false
				for key, value := range desired {
					if next.Labels[key] != value {
						next.Labels[key] = value
						changed = true
					}
				}
				if changed {
					if err := client.UpdateNode(context.Background(), actual, next); err != nil {
						return err
					}
					updated = append(updated, name)
				}
			}
		}

		sort.Strings(updated)
		sort.Strings(missing)

		writeback, err := cmdutil.WriteAutoNodeLabels(cmdutil.AutoLabelWriteOptions{
			ConfigPath:      configPath,
			Config:          cfg,
			PartitionFilter: partition,
			Prune:           opts.PruneAutoLabels,
		})
		if err != nil {
			return err
		}

		out := cmd.OutOrStdout()
		_, _ = fmt.Fprintf(out, "bootstrap labels OK\nnodes updated: %d\nnodes missing: %d\n", len(updated), len(missing))
		if len(updated) > 0 {
			_, _ = fmt.Fprintln(out, "nodes updated:")
			for _, name := range updated {
				_, _ = fmt.Fprintf(out, "  - %s\n", name)
			}
		}
		if len(missing) > 0 {
			_, _ = fmt.Fprintln(out, "nodes missing:")
			for _, name := range missing {
				_, _ = fmt.Fprintf(out, "  - %s\n", name)
			}
		}
		if writeback.Skipped {
			_, _ = fmt.Fprintf(out, "config labels: skipped (%s)\n", writeback.SkipReason)
		} else {
			_, _ = fmt.Fprintf(out, "config labels: added=%d updated=%d pruned=%d\n", writeback.Added, writeback.Updated, writeback.Pruned)
		}
		if len(writeback.Notes) > 0 {
			_, _ = fmt.Fprintln(out, "label notes:")
			for _, note := range writeback.Notes {
				_, _ = fmt.Fprintf(out, "  - %s\n", note)
			}
		}
		return nil
	},
}

func init() {
	bootstrapCmd.AddCommand(bootstrapNetworksCmd)
	bootstrapCmd.AddCommand(bootstrapLabelsCmd)
	bootstrapLabelsCmd.Flags().BoolVar(&opts.PruneAutoLabels, "prune-auto-labels", false, "Remove auto volume labels that are no longer required by the current execution")
}
