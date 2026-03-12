package cmd

import (
	"context"
	"fmt"
	"io"
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

var planProgressEnabled = true

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Compute desired state and show changes",
	RunE: func(cmd *cobra.Command, args []string) error {
		targets, err := prepareRuntimeTargets()
		if err != nil {
			return err
		}
		progress := newPlanProgressReporter(cmd.ErrOrStderr(), planProgressEnabled)
		out := cmd.OutOrStdout()

		for deploymentIndex, deployment := range targets.deployments {
			if len(targets.deployments) > 1 {
				if deploymentIndex > 0 {
					_, _ = fmt.Fprintln(out)
				}
				label := deployment
				if label == "" {
					label = "(default)"
				}
				_, _ = fmt.Fprintf(out, "target deployment: %s\n", label)
			}

			done := progress.start("load project context")
			projectCtx, err := loadValidatedProjectContext(targets, deployment, runtimeTargetOptions{includeValues: true, includeSecrets: true})
			done(err)
			if err != nil {
				return err
			}
			cfg := projectCtx.Config
			nodesInScope := cmdutil.ResolveDeploymentNodes(cfg)

			debugContentEnabled := opts.DebugContent
			if flag := cmd.Flags().Lookup("debug-content-max"); flag != nil && flag.Changed {
				debugContentEnabled = true
			}
			debugMax := opts.DebugContentMax
			debugDefsEnabled := opts.Debug || debugContentEnabled

			done = progress.start("detect template cycles")
			warnings, err := templates.DetectCycles(cfg, !opts.NoInfer)
			done(err)
			if err != nil {
				return err
			}
			warnings = filterInferredRefWarnings(warnings)
			warnings = append(warnings, cmdutil.VolumePlacementWarnings(cfg, targets.partitionFilters, targets.stackFilters, opts.Debug)...)

			done = progress.start("render desired configs/secrets")
			summary, err := render.RenderProject(cfg, projectCtx.Secrets, projectCtx.Values, targets.partitionFilters, targets.stackFilters, opts.AllowMissing, !opts.NoInfer)
			done(err)
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
				done = progress.start("load swarm nodes")
				client, err := getSwarmClient()
				if err != nil {
					done(err)
					return err
				}
				nodes, err := client.ListNodes(context.Background())
				done(err)
				if err != nil {
					return err
				}
				warnings = append(warnings, cmdutil.NodeLabelWarnings(cfg, nodeSpecs, nodes)...)
			}

			done = progress.start("compute desired plan")
			desired := apply.DesiredStateFromSummary(cfg, summary, targets.partitionFilters, targets.stackFilters)
			statePath, err := planStatePath(targets.configPath)
			done(err)
			if err != nil {
				return err
			}
			partitionState := ""
			if len(targets.partitionFilters) == 1 {
				partitionState = targets.partitionFilters[0]
			}
			stackState := ""
			if len(targets.stackFilters) == 1 {
				stackState = targets.stackFilters[0]
			}
			planSnapshot := state.State{
				Version:     state.CurrentVersion,
				GeneratedAt: time.Now().UTC().Format(time.RFC3339),
				Command:     "plan",
				ConfigPath:  targets.configPath,
				Project:     cfg.Project.Name,
				Deployment:  cfg.Project.Deployment,
				Partition:   partitionState,
				Stack:       stackState,
			}
			type networkStatus struct {
				Name   string
				Status string
			}
			var networkStatuses []networkStatus
			if len(desired.Networks) > 0 {
				done = progress.start("load swarm networks")
				existing := map[string]struct{}{}
				client, err := getSwarmClient()
				if err != nil {
					done(err)
					return err
				}
				networks, err := client.ListNetworks(context.Background())
				done(err)
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
			cmdutil.PrintWarnings(out, warnings)
			_, _ = fmt.Fprintf(out, "plan OK (dry-run)\nconfigs rendered: %d\nsecrets rendered: %d\n", summary.Configs, summary.Secrets)
			if len(summary.MissingSecrets) > 0 {
				sort.Strings(summary.MissingSecrets)
				_, _ = fmt.Fprintf(out, "missing secrets (placeholders): %d\n", len(summary.MissingSecrets))
				for _, item := range summary.MissingSecrets {
					_, _ = fmt.Fprintf(out, "  - %s\n", item)
				}
			}
			if len(nodesInScope) > 0 {
				_, _ = fmt.Fprintln(out, "nodes in scope:")
				for _, name := range nodesInScope {
					_, _ = fmt.Fprintf(out, "  - %s\n", name)
				}
			}
			if len(summary.ConfigsRendered) > 0 {
				_, _ = fmt.Fprintln(out, "configs:")
				printGroupedRenderedItems(out, summary.ConfigsRendered, splitRenderedDefItem)
			}
			if len(summary.SecretsRendered) > 0 {
				_, _ = fmt.Fprintln(out, "secrets:")
				printGroupedRenderedItems(out, summary.SecretsRendered, splitRenderedDefItem)
			}
			if len(networkStatuses) > 0 {
				_, _ = fmt.Fprintln(out, "networks:")
				for _, status := range networkStatuses {
					_, _ = fmt.Fprintf(out, "  - %s (%s)\n", status.Name, status.Status)
				}
			}
			if len(summary.Mounts) > 0 {
				_, _ = fmt.Fprintln(out, "service mounts:")
				printGroupedRenderedItems(out, summary.Mounts, splitMountItem)
			}
			done = progress.start("plan bind paths")
			bindPaths, err := apply.PlanBindPaths(cfg, projectCtx.Values, targets.partitionFilters, targets.stackFilters)
			done(err)
			if err != nil {
				return err
			}
			if len(bindPaths) > 0 {
				_, _ = fmt.Fprintln(out, "bind paths required:")
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
					_, _ = fmt.Fprintf(out, "  - %s\n", line)
				}
			}
			if debugDefsEnabled {
				done = progress.start("build stack compose output")
				stackDeploys, err := apply.BuildStackDeploys(cfg, desired, projectCtx.Values, targets.partitionFilters, targets.stackFilters, nil, nil, nil, !opts.NoInfer)
				done(err)
				if err != nil {
					return err
				}
				if len(stackDeploys) > 0 {
					_, _ = fmt.Fprintln(out, "stacks:")
					for _, deploy := range stackDeploys {
						_, _ = fmt.Fprintf(out, "  - %s\n", deploy.Name)
						for _, line := range strings.Split(strings.TrimRight(string(deploy.Compose), "\n"), "\n") {
							_, _ = fmt.Fprintf(out, "      %s\n", line)
						}
						planSnapshot.Plan.StackNames = append(planSnapshot.Plan.StackNames, deploy.Name)
					}
				}
			}
			if debugDefsEnabled && len(summary.Defs) > 0 {
				_, _ = fmt.Fprintln(out, "debug:")
				for _, item := range summary.Defs {
					physical, hash := render.PhysicalName(item.Name, item.Content)
					labels := render.FormatLabels(render.Labels(item.ScopeID, item.Name, hash))
					if opts.Debug {
						_, _ = fmt.Fprintf(out, "  - %s %q (%s) physical=%s labels=%s\n", item.Kind, item.Name, item.Scope, physical, labels)
					}
					if debugContentEnabled {
						content := cmdutil.TruncateContent(item.Content, debugMax)
						_, _ = fmt.Fprintf(out, "  - %s %q content:\n", item.Kind, item.Name)
						for _, line := range strings.Split(content, "\n") {
							_, _ = fmt.Fprintf(out, "      %s\n", line)
						}
					}
				}
				if opts.Debug {
					for _, item := range summary.RuntimeRefs {
						qualifier := ""
						if item.Missing {
							qualifier = " inferred"
						}
						_, _ = fmt.Fprintf(out, "  - runtime%s %s %q -> %s %q (from %s via %s)\n", qualifier, item.FromKind, item.FromName, item.ToKind, item.ToName, cmdutil.ScopeLabel(item.From), item.FuncName)
					}
				}
			}
			done = progress.start("write plan state")
			if err := state.Write(statePath, planSnapshot); err != nil {
				done(err)
				return err
			}
			done(nil)
		}
		return nil
	},
}

type planProgressReporter struct {
	out     io.Writer
	enabled bool
}

func newPlanProgressReporter(out io.Writer, enabled bool) planProgressReporter {
	return planProgressReporter{out: out, enabled: enabled}
}

func (p planProgressReporter) start(phase string) func(error) {
	if !p.enabled {
		return func(error) {}
	}
	started := time.Now()
	_, _ = fmt.Fprintf(p.out, "plan: %s...\n", phase)
	return func(err error) {
		elapsed := time.Since(started).Round(time.Millisecond)
		if err != nil {
			_, _ = fmt.Fprintf(p.out, "plan: %s failed (%s): %v\n", phase, elapsed, err)
			return
		}
		_, _ = fmt.Fprintf(p.out, "plan: %s done (%s)\n", phase, elapsed)
	}
}

func init() {
	planCmd.Flags().BoolVar(&planProgressEnabled, "progress", true, "Show phase progress while computing plan")
}

func singleSelectorValue(items []string) string {
	if len(items) != 1 {
		return ""
	}
	return items[0]
}

func filterInferredRefWarnings(warnings []string) []string {
	if len(warnings) == 0 {
		return warnings
	}
	filtered := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		// Inferred refs are expected behavior and are already shown in service mounts.
		if strings.Contains(warning, "(inferred)") && (strings.Contains(warning, "config_ref") || strings.Contains(warning, "secret_ref")) {
			continue
		}
		filtered = append(filtered, warning)
	}
	return filtered
}

func sortedStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := append([]string(nil), items...)
	sort.Strings(out)
	return out
}

type renderedGroup struct {
	scope string
	items []string
}

func printGroupedRenderedItems(out io.Writer, items []string, split func(string) (string, string)) {
	groups := groupRenderedItems(items, split)
	for _, group := range groups {
		_, _ = fmt.Fprintf(out, "  %s:\n", group.scope)
		for _, item := range group.items {
			_, _ = fmt.Fprintf(out, "    - %s\n", item)
		}
	}
}

func groupRenderedItems(items []string, split func(string) (string, string)) []renderedGroup {
	if len(items) == 0 {
		return nil
	}
	grouped := make(map[string][]string)
	for _, raw := range items {
		scope, item := split(raw)
		grouped[scope] = append(grouped[scope], item)
	}
	scopes := make([]string, 0, len(grouped))
	for scope := range grouped {
		scopes = append(scopes, scope)
	}
	sort.Strings(scopes)
	out := make([]renderedGroup, 0, len(scopes))
	for _, scope := range scopes {
		out = append(out, renderedGroup{
			scope: scope,
			items: sortedStrings(grouped[scope]),
		})
	}
	return out
}

func splitRenderedDefItem(item string) (string, string) {
	i := strings.LastIndex(item, ` "`)
	if i <= 0 || i+1 >= len(item) {
		return "unscoped", item
	}
	return item[:i], item[i+1:]
}

func splitMountItem(item string) (string, string) {
	start := strings.Index(item, "(stack ")
	if start < 0 {
		return "unscoped", item
	}
	end := strings.Index(item[start:], ")")
	if end < 0 {
		return "unscoped", item
	}
	scope := item[start+1 : start+end]
	head := strings.TrimSpace(item[:start])
	tail := strings.TrimSpace(item[start+end+1:])
	if tail != "" {
		return scope, strings.TrimSpace(head + " " + tail)
	}
	return scope, head
}
