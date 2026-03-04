package cmd

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cmmoran/swarmcp/internal/cmdutil"
	"github.com/cmmoran/swarmcp/internal/render"
	"github.com/cmmoran/swarmcp/internal/swarm"
	"github.com/spf13/cobra"
)

type configVersionRecord struct {
	config swarm.Config
	id     defIdentity
}

type diffConfigFilter struct {
	Name      string
	Stack     string
	Partition string
	Service   string
}

type configSelector struct {
	Base      string
	Stack     string
	Partition string
	Service   string
}

func init() {
	diffCmd.AddCommand(newDiffConfigCmd())
}

func newDiffConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect and compare config versions in Swarm",
	}
	cmd.AddCommand(newDiffConfigListCmd())
	cmd.AddCommand(newDiffConfigCompareCmd())
	return cmd
}

func newDiffConfigListCmd() *cobra.Command {
	var name string
	var stack string
	var service string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List config versions grouped by scope",
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit < 0 {
				return fmt.Errorf("limit must be >= 0")
			}
			deployment, err := singleSelector("deployment", opts.Deployments)
			if err != nil {
				return err
			}
			partition, err := singleSelector("partition", opts.Partitions)
			if err != nil {
				return err
			}
			projectCtx, err := cmdutil.LoadProjectContext(cmdutil.ProjectOptions{
				ConfigPath: opts.ConfigPath,
				Deployment: deployment,
				Context:    opts.Context,
				Partition:  partition,
				Offline:    opts.Offline,
				Debug:      opts.Debug,
			}, false, false)
			if err != nil {
				return err
			}
			filter := diffConfigFilter{
				Name:      strings.TrimSpace(name),
				Stack:     normalizeLabel(stack),
				Partition: normalizeLabel(projectCtx.Partition),
				Service:   normalizeLabel(service),
			}
			client, err := projectCtx.SwarmClient()
			if err != nil {
				return err
			}
			records, err := findConfigVersions(context.Background(), client, projectCtx.Config.Project.Name, filter)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(records) == 0 {
				_, _ = fmt.Fprintln(out, "config versions OK\nmatches: 0")
				return nil
			}
			groups := groupConfigVersions(records)
			_, _ = fmt.Fprintf(out, "config versions OK\nmatches: %d groups: %d\n", len(records), len(groups))
			for _, group := range groups {
				_, _ = fmt.Fprintf(out, "%s logical=%q\n", group.scope, group.name)
				rows := group.rows
				if limit > 0 && len(rows) > limit {
					rows = rows[:limit]
				}
				for i, row := range rows {
					_, _ = fmt.Fprintf(out, "  @%d %s physical=%s id=%s\n", i, row.config.CreatedAt.UTC().Format(time.RFC3339), row.config.Name, row.config.ID)
				}
				if limit > 0 && len(group.rows) > limit {
					_, _ = fmt.Fprintf(out, "  ... %d more\n", len(group.rows)-limit)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Logical config name (swarmcp.io/name) filter")
	cmd.Flags().StringVar(&stack, "stack", "", "Stack scope filter")
	cmd.Flags().StringVar(&service, "service", "", "Service scope filter")
	cmd.Flags().IntVar(&limit, "limit", 20, "Max versions per group to print (0 for all)")
	return cmd
}

func newDiffConfigCompareCmd() *cobra.Command {
	var name string
	var stack string
	var service string
	var leftSel string
	var rightSel string
	var contextLines int
	var color bool
	cmd := &cobra.Command{
		Use:   "compare",
		Short: "Compare two config versions with pretty diff",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("--name is required")
			}
			leftSel = strings.TrimSpace(leftSel)
			rightSel = strings.TrimSpace(rightSel)
			if leftSel == "" || rightSel == "" {
				return fmt.Errorf("--left and --right are required")
			}
			if contextLines < 0 {
				return fmt.Errorf("--context must be >= 0")
			}
			deployment, err := singleSelector("deployment", opts.Deployments)
			if err != nil {
				return err
			}
			partition, err := singleSelector("partition", opts.Partitions)
			if err != nil {
				return err
			}
			projectCtx, err := cmdutil.LoadProjectContext(cmdutil.ProjectOptions{
				ConfigPath: opts.ConfigPath,
				Deployment: deployment,
				Context:    opts.Context,
				Partition:  partition,
				Offline:    opts.Offline,
				Debug:      opts.Debug,
			}, false, false)
			if err != nil {
				return err
			}
			filter := diffConfigFilter{
				Name:      strings.TrimSpace(name),
				Stack:     normalizeLabel(stack),
				Partition: normalizeLabel(projectCtx.Partition),
				Service:   normalizeLabel(service),
			}
			client, err := projectCtx.SwarmClient()
			if err != nil {
				return err
			}
			records, err := findConfigVersions(context.Background(), client, projectCtx.Config.Project.Name, filter)
			if err != nil {
				return err
			}
			if len(records) == 0 {
				return fmt.Errorf("no matching configs found")
			}
			left, err := selectConfigVersion(records, leftSel)
			if err != nil {
				return err
			}
			right, err := selectConfigVersion(records, rightSel)
			if err != nil {
				return err
			}
			ctx := context.Background()
			leftContent, err := client.ConfigContent(ctx, left.config.ID)
			if err != nil {
				return err
			}
			rightContent, err := client.ConfigContent(ctx, right.config.ID)
			if err != nil {
				return err
			}
			diff, err := semanticDiffLinesWithContext(string(leftContent), string(rightContent), contextLines)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintln(out, "config compare OK")
			if left.id.Key() == right.id.Key() {
				_, _ = fmt.Fprintf(out, "scope: %s\n", formatScopeLabel(left.id))
				_, _ = fmt.Fprintf(out, "name: %q\n", left.id.Name)
			} else {
				_, _ = fmt.Fprintf(out, "left scope: %s\n", formatScopeLabel(left.id))
				_, _ = fmt.Fprintf(out, "right scope: %s\n", formatScopeLabel(right.id))
				if left.id.Name == right.id.Name {
					_, _ = fmt.Fprintf(out, "name: %q\n", left.id.Name)
				} else {
					_, _ = fmt.Fprintf(out, "left name: %q\n", left.id.Name)
					_, _ = fmt.Fprintf(out, "right name: %q\n", right.id.Name)
				}
			}
			leftAge, rightAge := relativeAgeLabels(left.config.CreatedAt, right.config.CreatedAt)
			_, _ = fmt.Fprintf(out, "left: %s %s (%s) %s\n", left.config.Name, left.config.CreatedAt.UTC().Format(time.RFC3339), leftSel, leftAge)
			_, _ = fmt.Fprintf(out, "right: %s %s (%s) %s\n", right.config.Name, right.config.CreatedAt.UTC().Format(time.RFC3339), rightSel, rightAge)
			_, _ = fmt.Fprintln(out, "diff:")
			for _, line := range diff {
				_, _ = fmt.Fprintf(out, "  %s\n", renderDiffLine(line, color))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Logical config name (swarmcp.io/name)")
	cmd.Flags().StringVar(&stack, "stack", "", "Stack scope filter")
	cmd.Flags().StringVar(&service, "service", "", "Service scope filter")
	cmd.Flags().StringVar(&leftSel, "left", "", "Left version selector (@index or physical name)")
	cmd.Flags().StringVar(&rightSel, "right", "", "Right version selector (@index or physical name)")
	cmd.Flags().IntVar(&contextLines, "context", 0, "Show N unchanged context lines around changes")
	cmd.Flags().BoolVar(&color, "color", false, "Enable ANSI colorized diff output")
	return cmd
}

func findConfigVersions(ctx context.Context, client swarm.Client, projectName string, filter diffConfigFilter) ([]configVersionRecord, error) {
	configs, err := client.ListConfigs(ctx)
	if err != nil {
		return nil, err
	}
	records := make([]configVersionRecord, 0, len(configs))
	for _, cfg := range configs {
		labels := cfg.Labels
		if labels == nil || labels[render.LabelManaged] != "true" || labels[render.LabelProject] != projectName {
			continue
		}
		id := identityFromLabels(labels)
		if id.Name == "" {
			id.Name = cfg.Name
		}
		if filter.Name != "" && id.Name != filter.Name {
			continue
		}
		if filter.Stack != "" && id.Stack != filter.Stack {
			continue
		}
		if filter.Partition != "" && id.Partition != filter.Partition {
			continue
		}
		if filter.Service != "" && id.Service != filter.Service {
			continue
		}
		records = append(records, configVersionRecord{config: cfg, id: id})
	}
	sort.Slice(records, func(i, j int) bool {
		if !records[i].config.CreatedAt.Equal(records[j].config.CreatedAt) {
			return records[i].config.CreatedAt.After(records[j].config.CreatedAt)
		}
		return records[i].config.Name > records[j].config.Name
	})
	return records, nil
}

type groupedConfigVersions struct {
	scope string
	name  string
	rows  []configVersionRecord
}

func groupConfigVersions(records []configVersionRecord) []groupedConfigVersions {
	type key struct {
		scope string
		name  string
	}
	grouped := make(map[key][]configVersionRecord)
	for _, record := range records {
		k := key{scope: formatScopeLabel(record.id), name: record.id.Name}
		grouped[k] = append(grouped[k], record)
	}
	keys := make([]key, 0, len(grouped))
	for k := range grouped {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].scope != keys[j].scope {
			return keys[i].scope < keys[j].scope
		}
		return keys[i].name < keys[j].name
	})
	out := make([]groupedConfigVersions, 0, len(keys))
	for _, k := range keys {
		rows := grouped[k]
		sort.Slice(rows, func(i, j int) bool {
			if !rows[i].config.CreatedAt.Equal(rows[j].config.CreatedAt) {
				return rows[i].config.CreatedAt.After(rows[j].config.CreatedAt)
			}
			return rows[i].config.Name > rows[j].config.Name
		})
		out = append(out, groupedConfigVersions{scope: k.scope, name: k.name, rows: rows})
	}
	return out
}

func selectConfigVersion(records []configVersionRecord, selector string) (configVersionRecord, error) {
	if selector == "" {
		return configVersionRecord{}, fmt.Errorf("empty selector")
	}
	parsed, err := parseConfigSelector(selector)
	if err != nil {
		return configVersionRecord{}, err
	}
	candidates := filterConfigSelectorCandidates(records, parsed)
	if len(candidates) == 0 {
		return configVersionRecord{}, fmt.Errorf("selector %q matched no configs", selector)
	}
	if strings.HasPrefix(parsed.Base, "@") {
		index, err := strconv.Atoi(strings.TrimPrefix(parsed.Base, "@"))
		if err != nil || index < 0 {
			return configVersionRecord{}, fmt.Errorf("invalid selector %q: expected @<index>", selector)
		}
		if index >= len(candidates) {
			return configVersionRecord{}, fmt.Errorf("selector %q out of range (matches=%d)", selector, len(candidates))
		}
		return candidates[index], nil
	}
	nameMatches := make([]configVersionRecord, 0, 1)
	for _, record := range candidates {
		if record.config.Name == parsed.Base {
			nameMatches = append(nameMatches, record)
		}
	}
	if len(nameMatches) == 0 {
		return configVersionRecord{}, fmt.Errorf("selector %q not found in matched configs", selector)
	}
	if len(nameMatches) > 1 {
		return configVersionRecord{}, fmt.Errorf("selector %q matched multiple configs; narrow with #partition/#stack/#service", selector)
	}
	return nameMatches[0], nil
}

func formatScopeLabel(id defIdentity) string {
	scope := cmdutil.ScopeLabel(id.Scope())
	if scope == "" {
		return "project"
	}
	return scope
}

func renderDiffLine(line string, color bool) string {
	if !color {
		return line
	}
	switch {
	case strings.HasPrefix(line, "+ "):
		return "\x1b[32m" + line + "\x1b[0m"
	case strings.HasPrefix(line, "- "):
		return "\x1b[31m" + line + "\x1b[0m"
	case strings.HasPrefix(line, "  "):
		return "\x1b[2m" + line + "\x1b[0m"
	case line == "...":
		return "\x1b[36m" + line + "\x1b[0m"
	default:
		return line
	}
}

func relativeAgeLabels(left, right time.Time) (string, string) {
	if left.After(right) {
		return "(newer)", "(older)"
	}
	if right.After(left) {
		return "(older)", "(newer)"
	}
	return "(same age)", "(same age)"
}

func parseConfigSelector(raw string) (configSelector, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return configSelector{}, fmt.Errorf("empty selector")
	}
	parts := strings.Split(value, "#")
	base := strings.TrimSpace(parts[0])
	if base == "" {
		return configSelector{}, fmt.Errorf("invalid selector %q: missing base selector", raw)
	}
	out := configSelector{Base: base}
	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		if part == "" {
			return configSelector{}, fmt.Errorf("invalid selector %q: empty qualifier", raw)
		}
		key, val, ok := strings.Cut(part, "=")
		if !ok {
			return configSelector{}, fmt.Errorf("invalid selector %q: qualifier %q must be key=value", raw, part)
		}
		key = strings.ToLower(strings.TrimSpace(key))
		val = normalizeLabel(strings.TrimSpace(val))
		if val == "" {
			return configSelector{}, fmt.Errorf("invalid selector %q: qualifier %q has empty value", raw, key)
		}
		switch key {
		case "partition":
			out.Partition = val
		case "stack":
			out.Stack = val
		case "service":
			out.Service = val
		default:
			return configSelector{}, fmt.Errorf("invalid selector %q: unknown qualifier %q", raw, key)
		}
	}
	return out, nil
}

func filterConfigSelectorCandidates(records []configVersionRecord, selector configSelector) []configVersionRecord {
	filtered := make([]configVersionRecord, 0, len(records))
	for _, record := range records {
		if selector.Stack != "" && record.id.Stack != selector.Stack {
			continue
		}
		if selector.Partition != "" && record.id.Partition != selector.Partition {
			continue
		}
		if selector.Service != "" && record.id.Service != selector.Service {
			continue
		}
		filtered = append(filtered, record)
	}
	return filtered
}
