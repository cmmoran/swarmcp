package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/cmmoran/swarmcp/internal/apply"
	"github.com/cmmoran/swarmcp/internal/cmdutil"
	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/render"
	"github.com/cmmoran/swarmcp/internal/state"
	"github.com/cmmoran/swarmcp/internal/swarm"
	"github.com/cmmoran/swarmcp/internal/templates"
	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show planned changes vs current Swarm state",
	RunE: func(cmd *cobra.Command, args []string) error {
		targets, err := prepareRuntimeTargets()
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		return forEachRuntimeTarget(out, targets, runtimeTargetOptions{includeValues: true, includeSecrets: true}, func(target runtimeTarget) error {
			cfg := target.projectCtx.Config
			desired, err := apply.BuildDesiredState(cfg, target.projectCtx.Secrets, target.projectCtx.Values, target.partitionFilters, target.stackFilters, opts.AllowMissing, !opts.NoInfer)
			if err != nil {
				return err
			}

			client, err := target.projectCtx.SwarmClient()
			if err != nil {
				return err
			}
			ctx := context.Background()
			preserve, err := resolvePreserve(cmd, cfg, opts)
			if err != nil {
				return err
			}
			report, err := apply.BuildStatus(ctx, client, cfg, desired, target.projectCtx.Values, target.partitionFilters, target.stackFilters, !opts.NoInfer, preserve)
			if err != nil {
				return err
			}

			warnings := cmdutil.VolumePlacementWarnings(cfg, target.partitionFilters, target.stackFilters, opts.Debug)
			sortServiceStates(report.Services)
			sortConfigSpecs(report.MissingConfigs)
			sortSecretSpecs(report.MissingSecrets)
			sortConfigs(report.StaleConfigs)
			sortSecrets(report.StaleSecrets)
			sortDriftItems(report.DriftConfigs)
			sortDriftItems(report.DriftSecrets)

			changedServices, missingServices := splitServiceStates(report.Services)
			debugContentEnabled := opts.DebugContent
			if flag := cmd.Flags().Lookup("debug-content-max"); flag != nil && flag.Changed {
				debugContentEnabled = true
			}

			cmdutil.PrintWarnings(out, warnings)
			_, _ = fmt.Fprintf(out, "diff OK\nconfigs to create: %d\nsecrets to create: %d\nnetworks to create: %d\nconfigs to delete: %d\nsecrets to delete: %d\nconfigs preserved: %d\nsecrets preserved: %d\nconfigs skipped (in use): %d\nsecrets skipped (in use): %d\nservices to update: %d\nservices missing: %d\n", len(report.MissingConfigs), len(report.MissingSecrets), len(report.MissingNetworks), len(report.StaleConfigs), len(report.StaleSecrets), report.Preserved.ConfigsPreserved, report.Preserved.SecretsPreserved, report.SkippedDeletes.Configs, report.SkippedDeletes.Secrets, len(changedServices), len(missingServices))
			if len(report.MissingConfigs) > 0 {
				_, _ = fmt.Fprintln(out, "configs to create:")
				for _, cfg := range report.MissingConfigs {
					_, _ = fmt.Fprintf(out, "  - %s\n", cmdutil.FormatConfigItem(cfg.Name, cfg.Labels))
				}
			}
			if len(report.MissingSecrets) > 0 {
				_, _ = fmt.Fprintln(out, "secrets to create:")
				for _, sec := range report.MissingSecrets {
					_, _ = fmt.Fprintf(out, "  - %s\n", cmdutil.FormatConfigItem(sec.Name, sec.Labels))
				}
			}
			if len(report.StaleConfigs) > 0 {
				_, _ = fmt.Fprintln(out, "configs to delete:")
				for _, cfg := range report.StaleConfigs {
					_, _ = fmt.Fprintf(out, "  - %s\n", cmdutil.FormatConfigItem(cfg.Name, cfg.Labels))
				}
			}
			if len(report.StaleSecrets) > 0 {
				_, _ = fmt.Fprintln(out, "secrets to delete:")
				for _, sec := range report.StaleSecrets {
					_, _ = fmt.Fprintf(out, "  - %s\n", cmdutil.FormatConfigItem(sec.Name, sec.Labels))
				}
			}
			if len(report.DriftConfigs) > 0 {
				_, _ = fmt.Fprintln(out, "configs with label drift:")
				for _, item := range report.DriftConfigs {
					_, _ = fmt.Fprintf(out, "  - %s (%s)\n", cmdutil.FormatConfigItem(item.Name, item.Labels), item.Reason)
				}
			}
			if len(report.DriftSecrets) > 0 {
				_, _ = fmt.Fprintln(out, "secrets with label drift:")
				for _, item := range report.DriftSecrets {
					_, _ = fmt.Fprintf(out, "  - %s (%s)\n", cmdutil.FormatConfigItem(item.Name, item.Labels), item.Reason)
				}
			}
			if len(report.MissingNetworks) > 0 {
				_, _ = fmt.Fprintln(out, "networks to create:")
				for _, net := range report.MissingNetworks {
					_, _ = fmt.Fprintf(out, "  - %s\n", net.Name)
				}
			}
			if len(changedServices) > 0 {
				_, _ = fmt.Fprintln(out, "services to update:")
				for _, state := range changedServices {
					line := cmdutil.ServiceScopeLabel(state.Stack, state.Partition, state.Service)
					if len(state.IntentDiffs) > 0 {
						line += " (" + strings.Join(state.IntentDiffs, ", ") + ")"
					}
					_, _ = fmt.Fprintf(out, "  - %s\n", line)
				}
			}
			if unmanaged := unmanagedServiceStates(report.Services); len(unmanaged) > 0 {
				_, _ = fmt.Fprintln(out, "services with unmanaged drift:")
				for _, state := range unmanaged {
					line := cmdutil.ServiceScopeLabel(state.Stack, state.Partition, state.Service)
					line += " (" + strings.Join(state.Unmanaged, ", ") + ")"
					_, _ = fmt.Fprintf(out, "  - %s\n", line)
				}
			}
			if len(missingServices) > 0 {
				_, _ = fmt.Fprintln(out, "services missing:")
				for _, state := range missingServices {
					_, _ = fmt.Fprintf(out, "  - %s\n", cmdutil.ServiceScopeLabel(state.Stack, state.Partition, state.Service))
				}
			}
			if err := printStateDiff(out, target.configPath, report, changedServices, missingServices); err != nil {
				return err
			}
			if opts.Debug || debugContentEnabled {
				if err := printDiffDebug(out, context.Background(), client, report, changedServices, missingServices, diffDebugOptions{
					Debug:           opts.Debug,
					DebugContent:    debugContentEnabled,
					DebugContentMax: opts.DebugContentMax,
				}); err != nil {
					return err
				}
			}
			if opts.DiffSources {
				if err := printSourcesDiff(out, cfg, desired.Defs, opts.Offline, opts.Debug); err != nil {
					return err
				}
			}
			return nil
		})
	},
}

func init() {
	diffCmd.Flags().BoolVar(&opts.DiffSources, "sources", false, "Show external source changes per config/secret")
}

func sortConfigSpecs(items []swarm.ConfigSpec) {
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
}

func sortSecretSpecs(items []swarm.SecretSpec) {
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
}

func sortConfigs(items []swarm.Config) {
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
}

func sortSecrets(items []swarm.Secret) {
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
}

func splitServiceStates(states []apply.ServiceState) ([]apply.ServiceState, []apply.ServiceState) {
	var changed []apply.ServiceState
	var missing []apply.ServiceState
	for _, state := range states {
		if state.Missing {
			missing = append(missing, state)
			continue
		}
		if !state.IntentMatch {
			changed = append(changed, state)
		}
	}
	return changed, missing
}

func unmanagedServiceStates(states []apply.ServiceState) []apply.ServiceState {
	var unmanaged []apply.ServiceState
	for _, state := range states {
		if state.Missing {
			continue
		}
		if len(state.Unmanaged) > 0 {
			unmanaged = append(unmanaged, state)
		}
	}
	sort.Slice(unmanaged, func(i, j int) bool {
		if unmanaged[i].Stack != unmanaged[j].Stack {
			return unmanaged[i].Stack < unmanaged[j].Stack
		}
		if unmanaged[i].Partition != unmanaged[j].Partition {
			return unmanaged[i].Partition < unmanaged[j].Partition
		}
		return unmanaged[i].Service < unmanaged[j].Service
	})
	return unmanaged
}

func sortDriftItems(items []apply.DriftItem) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Name != items[j].Name {
			return items[i].Name < items[j].Name
		}
		return items[i].Reason < items[j].Reason
	})
}

func printStateDiff(out io.Writer, configPath string, report apply.StatusReport, changedServices, missingServices []apply.ServiceState) error {
	statePath, err := planStatePath(configPath)
	if err != nil {
		return err
	}
	cached, err := state.Read(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	_, _ = fmt.Fprintf(out, "state cache: %s %s\n", cached.Command, cached.GeneratedAt)
	if cached.Command != "apply" {
		_, _ = fmt.Fprintln(out, "state drift: skipped (last command was plan)")
		return nil
	}
	if cached.ConfigPath != "" && cached.ConfigPath != configPath {
		_, _ = fmt.Fprintf(out, "state drift: skipped (config path mismatch: %s)\n", cached.ConfigPath)
		return nil
	}
	current := state.PlanSummary{
		NetworksCreated: len(report.MissingNetworks),
		ConfigsCreated:  len(report.MissingConfigs),
		SecretsCreated:  len(report.MissingSecrets),
		ConfigsRemoved:  len(report.StaleConfigs),
		SecretsRemoved:  len(report.StaleSecrets),
		ConfigsSkipped:  report.SkippedDeletes.Configs,
		SecretsSkipped:  report.SkippedDeletes.Secrets,
		ServicesCreated: len(missingServices),
		ServicesUpdated: len(changedServices),
		StacksDeployed:  uniqueServiceStacks(changedServices, missingServices),
	}
	deltas := diffStateSummary(cached.Plan, current)
	if len(deltas) == 0 {
		_, _ = fmt.Fprintln(out, "state drift: none")
		return nil
	}
	_, _ = fmt.Fprintln(out, "state drift:")
	for _, delta := range deltas {
		_, _ = fmt.Fprintf(out, "  - %s: state=%d current=%d\n", delta.label, delta.stateValue, delta.currentValue)
	}
	return nil
}

type sourceDiffItem struct {
	kind   string
	name   string
	scope  templates.Scope
	source string
}

type sourceDiffResult struct {
	before   config.SourceMetadata
	beforeOK bool
	after    config.SourceMetadata
	err      error
}

func printSourcesDiff(out io.Writer, cfg *config.Config, defs []render.RenderedDef, offline bool, debug bool) error {
	if cfg == nil {
		return nil
	}
	if offline {
		return fmt.Errorf("offline mode is enabled; disable --offline to diff sources")
	}
	items := collectSourceDiffItems(cfg, defs)
	if len(items) == 0 {
		_, _ = fmt.Fprintln(out, "sources diff: none")
		return nil
	}
	loadOpts := config.LoadOptions{CacheDir: cfg.CacheDir, Offline: offline, Debug: debug}
	results := make(map[string]sourceDiffResult)
	_, _ = fmt.Fprintln(out, "sources diff:")
	for _, item := range items {
		label := fmt.Sprintf("%s %s (%s)", item.kind, item.name, cmdutil.ScopeLabel(item.scope))
		source := strings.TrimSpace(item.source)
		if source == "" {
			_, _ = fmt.Fprintf(out, "  - %s: missing source\n", label)
			continue
		}
		if strings.HasPrefix(source, "inline:") {
			_, _ = fmt.Fprintf(out, "  - %s: skipped (inline)\n", label)
			continue
		}
		if templates.IsValuesSource(source) {
			_, _ = fmt.Fprintf(out, "  - %s: skipped (values)\n", label)
			continue
		}
		base, fragment := templates.SplitSource(source)
		if base == "" {
			_, _ = fmt.Fprintf(out, "  - %s: skipped (empty source)\n", label)
			continue
		}
		if !config.IsGitSource(base) {
			if fragment != "" {
				_, _ = fmt.Fprintf(out, "  - %s: skipped (local source %s%s)\n", label, base, fragment)
				continue
			}
			_, _ = fmt.Fprintf(out, "  - %s: skipped (local source %s)\n", label, base)
			continue
		}
		parsed, ok, err := config.ParseGitSource(base)
		if err != nil || !ok {
			_, _ = fmt.Fprintf(out, "  - %s: invalid git source %s\n", label, base)
			continue
		}
		key := fmt.Sprintf("%s@%s#%s", parsed.URL, parsed.Ref, parsed.Path)
		result, ok := results[key]
		if !ok {
			before, beforeOK, err := config.ReadSourceMetadata(parsed.URL, parsed.Ref, parsed.Path, loadOpts)
			if err != nil {
				result.err = err
			} else {
				result.before = before
				result.beforeOK = beforeOK && before.Commit != ""
				result.after, result.err = config.FetchGitSource(parsed.URL, parsed.Ref, parsed.Path, loadOpts)
			}
			results[key] = result
		}
		if result.err != nil {
			_, _ = fmt.Fprintf(out, "  - %s: error %v\n", label, result.err)
			continue
		}
		if !result.beforeOK {
			_, _ = fmt.Fprintf(out, "  - %s: new commit=%s subtree=%s\n", label, result.after.Commit, result.after.Subtree)
			if err := printSourceFileDiffs(out, parsed, "", result.after.Commit, loadOpts); err != nil {
				_, _ = fmt.Fprintf(out, "    diff error: %v\n", err)
			}
			continue
		}
		if result.before.Commit == result.after.Commit && result.before.Subtree == result.after.Subtree {
			_, _ = fmt.Fprintf(out, "  - %s: no changes (commit=%s)\n", label, result.after.Commit)
			continue
		}
		_, _ = fmt.Fprintf(out, "  - %s: commit %s -> %s subtree %s -> %s\n", label, result.before.Commit, result.after.Commit, result.before.Subtree, result.after.Subtree)
		if err := printSourceFileDiffs(out, parsed, result.before.Commit, result.after.Commit, loadOpts); err != nil {
			_, _ = fmt.Fprintf(out, "    diff error: %v\n", err)
		}
	}
	return nil
}

func collectSourceDiffItems(cfg *config.Config, defs []render.RenderedDef) []sourceDiffItem {
	if cfg == nil || len(defs) == 0 {
		return nil
	}
	items := make([]sourceDiffItem, 0, len(defs))
	seen := make(map[string]struct{})
	for _, def := range defs {
		if def.Name == "" {
			continue
		}
		resolver := templates.NewScopeResolver(cfg, def.ScopeID, true, true, nil, nil, nil)
		switch def.Kind {
		case "config":
			cfgDef, scope, ok := resolver.ResolveConfigWithScope(def.Name)
			if !ok {
				continue
			}
			key := fmt.Sprintf("config:%s:%s", scopeKey(scope), def.Name)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			items = append(items, sourceDiffItem{
				kind:   "config",
				name:   def.Name,
				scope:  scope,
				source: cfgDef.Source,
			})
		case "secret":
			secDef, scope, ok := resolver.ResolveSecretWithScope(def.Name)
			if !ok {
				continue
			}
			key := fmt.Sprintf("secret:%s:%s", scopeKey(scope), def.Name)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			items = append(items, sourceDiffItem{
				kind:   "secret",
				name:   def.Name,
				scope:  scope,
				source: secDef.Source,
			})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].kind != items[j].kind {
			return items[i].kind < items[j].kind
		}
		if scopeKey(items[i].scope) != scopeKey(items[j].scope) {
			return scopeKey(items[i].scope) < scopeKey(items[j].scope)
		}
		return items[i].name < items[j].name
	})
	return items
}

func scopeKey(scope templates.Scope) string {
	return fmt.Sprintf("%s/%s/%s/%s", scope.Project, scope.Stack, scope.Partition, scope.Service)
}

func printSourceFileDiffs(out io.Writer, parsed config.GitSource, beforeCommit string, afterCommit string, opts config.LoadOptions) error {
	beforeFiles := map[string][]byte(nil)
	if strings.TrimSpace(beforeCommit) != "" {
		files, err := config.ReadGitSourceFiles(parsed.URL, beforeCommit, parsed.Path, opts)
		if err != nil {
			return err
		}
		beforeFiles = files
	}
	afterFiles, err := config.ReadGitSourceFiles(parsed.URL, afterCommit, parsed.Path, opts)
	if err != nil {
		return err
	}
	fileList := unionFileKeys(beforeFiles, afterFiles)
	if len(fileList) == 0 {
		_, _ = fmt.Fprintln(out, "    (no files)")
		return nil
	}
	anyDiff := false
	for _, name := range fileList {
		before := ""
		after := ""
		if data, ok := beforeFiles[name]; ok {
			before = string(data)
		}
		if data, ok := afterFiles[name]; ok {
			after = string(data)
		}
		if before == after {
			continue
		}
		anyDiff = true
		_, _ = fmt.Fprintf(out, "    file: %s\n", name)
		diff, err := diffLinesText(before, after)
		if err != nil {
			_, _ = fmt.Fprintf(out, "      diff error: %v\n", err)
			continue
		}
		for _, line := range diff {
			_, _ = fmt.Fprintf(out, "      %s\n", line)
		}
	}
	if !anyDiff {
		_, _ = fmt.Fprintln(out, "    (no file diffs)")
	}
	return nil
}

func unionFileKeys(before, after map[string][]byte) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for name := range before {
		seen[name] = struct{}{}
		out = append(out, name)
	}
	for name := range after {
		if _, ok := seen[name]; ok {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

type diffOp struct {
	kind byte
	line string
}

const (
	maxDiffLines     = 5000
	maxDiffMatrixOps = 4_000_000
)

func diffLinesText(before, after string) ([]string, error) {
	return diffLinesTextWithContext(before, after, 0)
}

func diffLinesTextWithContext(before, after string, contextLines int) ([]string, error) {
	if contextLines < 0 {
		contextLines = 0
	}
	beforeLines := splitLines(before)
	afterLines := splitLines(after)
	if len(beforeLines)*len(afterLines) > maxDiffMatrixOps {
		return []string{fmt.Sprintf("diff suppressed (line count %d -> %d)", len(beforeLines), len(afterLines))}, nil
	}
	ops := diffLines(beforeLines, afterLines)
	if len(ops) > maxDiffLines {
		return []string{fmt.Sprintf("diff suppressed (output lines %d)", len(ops))}, nil
	}
	if contextLines > 0 {
		return diffOpsWithContext(ops, contextLines), nil
	}
	out := make([]string, 0, len(ops))
	for _, op := range ops {
		switch op.kind {
		case '+':
			out = append(out, "+ "+op.line)
		case '-':
			out = append(out, "- "+op.line)
		}
	}
	if len(out) == 0 {
		return []string{"(no content changes)"}, nil
	}
	return out, nil
}

func diffOpsWithContext(ops []diffOp, contextLines int) []string {
	if len(ops) == 0 {
		return []string{"(no content changes)"}
	}
	keep := make([]bool, len(ops))
	hasChange := false
	for i, op := range ops {
		if op.kind == ' ' {
			continue
		}
		hasChange = true
		start := i - contextLines
		if start < 0 {
			start = 0
		}
		end := i + contextLines
		if end >= len(ops) {
			end = len(ops) - 1
		}
		for j := start; j <= end; j++ {
			keep[j] = true
		}
	}
	if !hasChange {
		return []string{"(no content changes)"}
	}
	out := make([]string, 0, len(ops))
	inGap := false
	for i, op := range ops {
		if !keep[i] {
			inGap = true
			continue
		}
		if inGap && len(out) > 0 {
			out = append(out, "...")
		}
		inGap = false
		switch op.kind {
		case '+':
			out = append(out, "+ "+op.line)
		case '-':
			out = append(out, "- "+op.line)
		default:
			out = append(out, "  "+op.line)
		}
	}
	if len(out) == 0 {
		return []string{"(no content changes)"}
	}
	if len(out) > maxDiffLines {
		return []string{fmt.Sprintf("diff suppressed (output lines %d)", len(out))}
	}
	return out
}

func splitLines(input string) []string {
	if input == "" {
		return []string{}
	}
	normalized := strings.ReplaceAll(input, "\r\n", "\n")
	return strings.Split(normalized, "\n")
}

func diffLines(before, after []string) []diffOp {
	n := len(before)
	m := len(after)
	if n == 0 && m == 0 {
		return nil
	}
	if n == 0 {
		ops := make([]diffOp, 0, m)
		for _, line := range after {
			ops = append(ops, diffOp{kind: '+', line: line})
		}
		return ops
	}
	if m == 0 {
		ops := make([]diffOp, 0, n)
		for _, line := range before {
			ops = append(ops, diffOp{kind: '-', line: line})
		}
		return ops
	}
	width := m + 1
	dp := make([]int, (n+1)*width)
	for i := n - 1; i >= 0; i-- {
		row := i * width
		next := (i + 1) * width
		for j := m - 1; j >= 0; j-- {
			if before[i] == after[j] {
				dp[row+j] = dp[next+j+1] + 1
			} else {
				a := dp[next+j]
				b := dp[row+j+1]
				if a >= b {
					dp[row+j] = a
				} else {
					dp[row+j] = b
				}
			}
		}
	}
	ops := make([]diffOp, 0, n+m)
	i, j := 0, 0
	for i < n || j < m {
		if i < n && j < m && before[i] == after[j] {
			ops = append(ops, diffOp{kind: ' ', line: before[i]})
			i++
			j++
			continue
		}
		if j < m && (i == n || dp[i*width+j+1] >= dp[(i+1)*width+j]) {
			ops = append(ops, diffOp{kind: '+', line: after[j]})
			j++
			continue
		}
		if i < n {
			ops = append(ops, diffOp{kind: '-', line: before[i]})
			i++
		}
	}
	return ops
}

type summaryDelta struct {
	label        string
	stateValue   int
	currentValue int
}

func diffStateSummary(previous, current state.PlanSummary) []summaryDelta {
	var deltas []summaryDelta
	appendDelta := func(label string, prev, cur int) {
		if prev != cur {
			deltas = append(deltas, summaryDelta{label: label, stateValue: prev, currentValue: cur})
		}
	}
	appendDelta("configs to create", previous.ConfigsCreated, current.ConfigsCreated)
	appendDelta("secrets to create", previous.SecretsCreated, current.SecretsCreated)
	appendDelta("networks to create", previous.NetworksCreated, current.NetworksCreated)
	appendDelta("configs to delete", previous.ConfigsRemoved, current.ConfigsRemoved)
	appendDelta("secrets to delete", previous.SecretsRemoved, current.SecretsRemoved)
	appendDelta("configs skipped", previous.ConfigsSkipped, current.ConfigsSkipped)
	appendDelta("secrets skipped", previous.SecretsSkipped, current.SecretsSkipped)
	appendDelta("services to create", previous.ServicesCreated, current.ServicesCreated)
	appendDelta("services to update", previous.ServicesUpdated, current.ServicesUpdated)
	appendDelta("stacks to deploy", previous.StacksDeployed, current.StacksDeployed)
	return deltas
}

func uniqueServiceStacks(changed, missing []apply.ServiceState) int {
	stacks := make(map[string]struct{})
	for _, state := range changed {
		stacks[state.Stack+"|"+state.Partition] = struct{}{}
	}
	for _, state := range missing {
		stacks[state.Stack+"|"+state.Partition] = struct{}{}
	}
	return len(stacks)
}
