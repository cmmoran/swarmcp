package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cmmoran/swarmcp/internal/apply"
	"github.com/cmmoran/swarmcp/internal/cmdutil"
	"github.com/cmmoran/swarmcp/internal/render"
	"github.com/cmmoran/swarmcp/internal/state"
	"github.com/cmmoran/swarmcp/internal/swarm"
	"github.com/cmmoran/swarmcp/internal/templates"
	"github.com/spf13/cobra"
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Compute desired state and show changes",
	RunE: func(cmd *cobra.Command, args []string) error {
		projectCtx, err := cmdutil.LoadProjectContext(cmdutil.ProjectOptions{
			ConfigPath:  opts.ConfigPath,
			SecretsFile: opts.SecretsFile,
			ValuesFiles: opts.ValuesFiles,
			Deployment:  opts.Deployment,
			Context:     opts.Context,
			Partition:   opts.Partition,
			Offline:     opts.Offline,
			Debug:       opts.Debug,
		}, true, true)
		if err != nil {
			return err
		}
		cfg := projectCtx.Config
		partitionFilter := projectCtx.Partition
		nodesInScope := cmdutil.ResolveDeploymentNodes(cfg)

		debugContentEnabled := opts.DebugContent
		if flag := cmd.Flags().Lookup("debug-content-max"); flag != nil && flag.Changed {
			debugContentEnabled = true
		}
		debugMax := opts.DebugContentMax
		debugDefsEnabled := opts.Debug || debugContentEnabled

		warnings, err := templates.DetectCycles(cfg, !opts.NoInfer)
		if err != nil {
			return err
		}
		warnings = append(warnings, cmdutil.VolumePlacementWarnings(cfg, partitionFilter, opts.Debug)...)

		summary, err := render.RenderProject(cfg, projectCtx.Secrets, projectCtx.Values, partitionFilter, opts.AllowMissing, !opts.NoInfer)
		if err != nil {
			return err
		}
		if summary.RuntimeGraph != nil {
			if err := summary.RuntimeGraph.DetectCycles(); err != nil {
				return fmt.Errorf("runtime cycle detection failed: %w", err)
			}
		}

		var swarmClient swarm.Client
		getSwarmClient := func() (swarm.Client, error) {
			if swarmClient != nil {
				return swarmClient, nil
			}
			client, err := projectCtx.SwarmClient()
			if err != nil {
				return nil, err
			}
			swarmClient = client
			return swarmClient, nil
		}

		nodeSpecs := cmdutil.ResolveDeploymentNodeSpecs(cfg)
		if len(nodeSpecs) > 0 {
			client, err := getSwarmClient()
			if err != nil {
				return err
			}
			nodes, err := client.ListNodes(context.Background())
			if err != nil {
				return err
			}
			warnings = append(warnings, cmdutil.NodeLabelWarnings(cfg, nodeSpecs, nodes)...)
		}

		desired := apply.DesiredStateFromSummary(cfg, summary, partitionFilter)
		statePath, err := planStatePath(opts.ConfigPath)
		if err != nil {
			return err
		}
		planSnapshot := state.State{
			Version:     state.CurrentVersion,
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			Command:     "plan",
			ConfigPath:  opts.ConfigPath,
			Project:     cfg.Project.Name,
			Deployment:  cfg.Project.Deployment,
			Partition:   partitionFilter,
		}
		type networkStatus struct {
			Name   string
			Status string
		}
		var networkStatuses []networkStatus
		if len(desired.Networks) > 0 {
			existing := map[string]struct{}{}
			client, err := getSwarmClient()
			if err != nil {
				return err
			}
			networks, err := client.ListNetworks(context.Background())
			if err != nil {
				return err
			}
			for _, network := range networks {
				existing[network.Name] = struct{}{}
			}
			for _, network := range desired.Networks {
				status := "create"
				if _, ok := existing[network.Name]; ok {
					status = "exists"
				}
				networkStatuses = append(networkStatuses, networkStatus{Name: network.Name, Status: status})
				if status == "create" {
					warnings = append(warnings, fmt.Sprintf("bootstrap recommended: missing network %q", network.Name))
					planSnapshot.Plan.NetworksCreated++
				}
			}
		}
		out := cmd.OutOrStdout()
		cmdutil.PrintWarnings(out, warnings)
		fmt.Fprintf(out, "plan OK (dry-run)\nconfigs rendered: %d\nsecrets rendered: %d\n", summary.Configs, summary.Secrets)
		if len(summary.MissingSecrets) > 0 {
			sort.Strings(summary.MissingSecrets)
			fmt.Fprintf(out, "missing secrets (placeholders): %d\n", len(summary.MissingSecrets))
			for _, item := range summary.MissingSecrets {
				fmt.Fprintf(out, "  - %s\n", item)
			}
		}
		if len(nodesInScope) > 0 {
			fmt.Fprintln(out, "nodes in scope:")
			for _, name := range nodesInScope {
				fmt.Fprintf(out, "  - %s\n", name)
			}
		}
		if len(summary.ConfigsRendered) > 0 {
			fmt.Fprintln(out, "configs:")
			for _, item := range summary.ConfigsRendered {
				fmt.Fprintf(out, "  - %s\n", item)
			}
		}
		if len(summary.SecretsRendered) > 0 {
			fmt.Fprintln(out, "secrets:")
			for _, item := range summary.SecretsRendered {
				fmt.Fprintf(out, "  - %s\n", item)
			}
		}
		if len(networkStatuses) > 0 {
			fmt.Fprintln(out, "networks:")
			for _, status := range networkStatuses {
				fmt.Fprintf(out, "  - %s (%s)\n", status.Name, status.Status)
			}
		}
		if len(summary.Mounts) > 0 {
			fmt.Fprintln(out, "service mounts:")
			for _, item := range summary.Mounts {
				fmt.Fprintf(out, "  - %s\n", item)
			}
		}
		bindPaths, err := apply.PlanBindPaths(cfg, projectCtx.Values, partitionFilter)
		if err != nil {
			return err
		}
		if len(bindPaths) > 0 {
			fmt.Fprintln(out, "bind paths required:")
			for _, item := range bindPaths {
				line := apply.FormatBindPathLine(item)
				if len(nodeSpecs) > 0 {
					nodes, unknown := cmdutil.NodesForConstraints(nodeSpecs, item.Constraints)
					if unknown {
						line += " nodes: unknown"
					} else if len(nodes) == 0 {
						line += " nodes: none"
					} else {
						line += " nodes: " + strings.Join(nodes, ", ")
					}
				}
				fmt.Fprintf(out, "  - %s\n", line)
			}
		}
		if debugDefsEnabled {
			stackDeploys, err := apply.BuildStackDeploys(cfg, desired, projectCtx.Values, partitionFilter, nil, nil, nil, !opts.NoInfer)
			if err != nil {
				return err
			}
			if len(stackDeploys) > 0 {
				fmt.Fprintln(out, "stacks:")
				for _, deploy := range stackDeploys {
					fmt.Fprintf(out, "  - %s\n", deploy.Name)
					for _, line := range strings.Split(strings.TrimRight(string(deploy.Compose), "\n"), "\n") {
						fmt.Fprintf(out, "      %s\n", line)
					}
					planSnapshot.Plan.StackNames = append(planSnapshot.Plan.StackNames, deploy.Name)
				}
			}
		}
		if debugDefsEnabled && len(summary.Defs) > 0 {
			fmt.Fprintln(out, "debug:")
			for _, item := range summary.Defs {
				physical, hash := render.PhysicalName(item.Name, item.Content)
				labels := render.FormatLabels(render.Labels(item.ScopeID, item.Name, hash))
				if opts.Debug {
					fmt.Fprintf(out, "  - %s %q (%s) physical=%s labels=%s\n", item.Kind, item.Name, item.Scope, physical, labels)
				}
				if debugContentEnabled {
					content := cmdutil.TruncateContent(item.Content, debugMax)
					fmt.Fprintf(out, "  - %s %q content:\n", item.Kind, item.Name)
					for _, line := range strings.Split(content, "\n") {
						fmt.Fprintf(out, "      %s\n", line)
					}
				}
			}
			if opts.Debug {
				for _, item := range summary.RuntimeRefs {
					qualifier := ""
					if item.Missing {
						qualifier = " inferred"
					}
					fmt.Fprintf(out, "  - runtime%s %s %q -> %s %q (from %s via %s)\n", qualifier, item.FromKind, item.FromName, item.ToKind, item.ToName, cmdutil.ScopeLabel(item.From), item.FuncName)
				}
			}
		}
		if err := state.Write(statePath, planSnapshot); err != nil {
			return err
		}
		return nil
	},
}
