package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/cmmoran/swarmcp/internal/apply"
	"github.com/cmmoran/swarmcp/internal/cmdutil"
	"github.com/cmmoran/swarmcp/internal/render"
	"github.com/cmmoran/swarmcp/internal/swarm"
	"github.com/cmmoran/swarmcp/internal/templates"
	"github.com/cmmoran/swarmcp/internal/yamlutil"
	"go.yaml.in/yaml/v4"
)

type diffDebugOptions struct {
	Debug           bool
	DebugContent    bool
	DebugContentMax int
}

type defIdentity struct {
	Project   string
	Stack     string
	Partition string
	Service   string
	Name      string
}

func printDiffDebug(out io.Writer, ctx context.Context, client swarm.Client, report apply.StatusReport, changedServices, missingServices []apply.ServiceState, opts diffDebugOptions) error {
	_, _ = fmt.Fprintln(out, "debug:")
	printDiffDebugSummary(out, report, changedServices, missingServices)
	if !opts.DebugContent {
		return nil
	}
	return printDiffDebugContent(out, ctx, client, report, changedServices, opts.DebugContentMax)
}

func printDiffDebugSummary(out io.Writer, report apply.StatusReport, changedServices, missingServices []apply.ServiceState) {
	hasItems := len(report.MissingConfigs) > 0 ||
		len(report.MissingSecrets) > 0 ||
		len(report.MissingNetworks) > 0 ||
		len(report.StaleConfigs) > 0 ||
		len(report.StaleSecrets) > 0 ||
		len(report.DriftConfigs) > 0 ||
		len(report.DriftSecrets) > 0 ||
		len(changedServices) > 0 ||
		len(missingServices) > 0 ||
		len(unmanagedServiceStates(report.Services)) > 0
	if !hasItems {
		_, _ = fmt.Fprintln(out, "  (no diff items)")
		return
	}
	if len(report.MissingConfigs) > 0 || len(report.StaleConfigs) > 0 || len(report.DriftConfigs) > 0 {
		_, _ = fmt.Fprintln(out, "  configs:")
		for _, cfg := range report.MissingConfigs {
			_, _ = fmt.Fprintf(out, "    - %s reason=missing fields=presence\n", formatDefIdentity("config", cfg.Name, cfg.Labels))
		}
		for _, cfg := range report.StaleConfigs {
			_, _ = fmt.Fprintf(out, "    - %s reason=stale fields=presence\n", formatDefIdentity("config", cfg.Name, cfg.Labels))
		}
		for _, item := range report.DriftConfigs {
			_, _ = fmt.Fprintf(out, "    - %s reason=label drift fields=labels detail=%s\n", formatDefIdentity("config", item.Name, item.Labels), item.Reason)
		}
	}
	if len(report.MissingSecrets) > 0 || len(report.StaleSecrets) > 0 || len(report.DriftSecrets) > 0 {
		_, _ = fmt.Fprintln(out, "  secrets:")
		for _, sec := range report.MissingSecrets {
			_, _ = fmt.Fprintf(out, "    - %s reason=missing fields=presence\n", formatDefIdentity("secret", sec.Name, sec.Labels))
		}
		for _, sec := range report.StaleSecrets {
			_, _ = fmt.Fprintf(out, "    - %s reason=stale fields=presence\n", formatDefIdentity("secret", sec.Name, sec.Labels))
		}
		for _, item := range report.DriftSecrets {
			_, _ = fmt.Fprintf(out, "    - %s reason=label drift fields=labels detail=%s\n", formatDefIdentity("secret", item.Name, item.Labels), item.Reason)
		}
	}
	if len(report.MissingNetworks) > 0 {
		names := make([]string, 0, len(report.MissingNetworks))
		for _, net := range report.MissingNetworks {
			names = append(names, net.Name)
		}
		sort.Strings(names)
		_, _ = fmt.Fprintln(out, "  networks:")
		for _, name := range names {
			_, _ = fmt.Fprintf(out, "    - %s reason=missing fields=presence\n", name)
		}
	}
	if len(changedServices) > 0 || len(missingServices) > 0 || len(unmanagedServiceStates(report.Services)) > 0 {
		_, _ = fmt.Fprintln(out, "  services:")
		for _, state := range changedServices {
			scope := cmdutil.ServiceScopeLabel(state.Stack, state.Partition, state.Service)
			fields := "none"
			if len(state.IntentDiffs) > 0 {
				fields = strings.Join(state.IntentDiffs, ", ")
			}
			_, _ = fmt.Fprintf(out, "    - %s reason=intent drift fields=%s\n", scope, fields)
		}
		for _, state := range missingServices {
			scope := cmdutil.ServiceScopeLabel(state.Stack, state.Partition, state.Service)
			_, _ = fmt.Fprintf(out, "    - %s reason=missing fields=presence\n", scope)
		}
		for _, state := range unmanagedServiceStates(report.Services) {
			scope := cmdutil.ServiceScopeLabel(state.Stack, state.Partition, state.Service)
			fields := "none"
			if len(state.Unmanaged) > 0 {
				fields = strings.Join(state.Unmanaged, ", ")
			}
			_, _ = fmt.Fprintf(out, "    - %s reason=unmanaged drift fields=%s\n", scope, fields)
		}
	}
}

func printDiffDebugContent(out io.Writer, ctx context.Context, client swarm.Client, report apply.StatusReport, changedServices []apply.ServiceState, maxContent int) error {
	_, _ = fmt.Fprintln(out, "debug content:")
	if err := printConfigContentDiffs(out, ctx, client, report.MissingConfigs, report.StaleConfigs, maxContent); err != nil {
		return err
	}
	printSecretContentChanges(out, report.MissingSecrets, report.StaleSecrets)
	printServiceContentDiffs(out, changedServices)
	return nil
}

type contentItem struct {
	Physical string
	Labels   map[string]string
	Content  string
	Err      error
}

func printConfigContentDiffs(out io.Writer, ctx context.Context, client swarm.Client, missing []swarm.ConfigSpec, stale []swarm.Config, maxContent int) error {
	desiredByKey := make(map[string]contentItem, len(missing))
	for _, cfg := range missing {
		id := identityFromLabels(cfg.Labels)
		desiredByKey[id.Key()] = contentItem{
			Physical: cfg.Name,
			Labels:   cfg.Labels,
			Content:  string(cfg.Data),
		}
	}
	staleByKey := make(map[string]contentItem, len(stale))
	for _, cfg := range stale {
		id := identityFromLabels(cfg.Labels)
		content, err := client.ConfigContent(ctx, cfg.ID)
		if err != nil {
			staleByKey[id.Key()] = contentItem{Physical: cfg.Name, Labels: cfg.Labels, Err: err}
			continue
		}
		staleByKey[id.Key()] = contentItem{
			Physical: cfg.Name,
			Labels:   cfg.Labels,
			Content:  string(content),
		}
	}
	keys := unionKeys(desiredByKey, staleByKey)
	if len(keys) == 0 {
		return nil
	}
	_, _ = fmt.Fprintln(out, "  configs:")
	for _, key := range keys {
		desiredItem, hasDesired := desiredByKey[key]
		staleItem, hasStale := staleByKey[key]
		label := desiredItem
		if !hasDesired {
			label = staleItem
		}
		reason := "content changed"
		switch {
		case hasDesired && !hasStale:
			reason = "new config"
		case hasStale && !hasDesired:
			reason = "removed config"
		}
		_, _ = fmt.Fprintf(out, "    - %s reason=%s\n", formatDefIdentity("config", label.Physical, label.Labels), reason)
		if hasStale && staleItem.Err != nil {
			_, _ = fmt.Fprintf(out, "      diff error: %v\n", staleItem.Err)
			continue
		}
		before := ""
		if hasStale {
			before = staleItem.Content
		}
		after := ""
		if hasDesired {
			after = desiredItem.Content
		}
		before = cmdutil.TruncateContent(before, maxContent)
		after = cmdutil.TruncateContent(after, maxContent)
		diff, err := semanticDiffLines(before, after)
		if err != nil {
			_, _ = fmt.Fprintf(out, "      diff error: %v\n", err)
			continue
		}
		for _, line := range diff {
			_, _ = fmt.Fprintf(out, "      %s\n", line)
		}
	}
	return nil
}

func printSecretContentChanges(out io.Writer, missing []swarm.SecretSpec, stale []swarm.Secret) {
	desiredByKey := make(map[string]contentItem, len(missing))
	for _, sec := range missing {
		id := identityFromLabels(sec.Labels)
		desiredByKey[id.Key()] = contentItem{Physical: sec.Name, Labels: sec.Labels}
	}
	staleByKey := make(map[string]contentItem, len(stale))
	for _, sec := range stale {
		id := identityFromLabels(sec.Labels)
		staleByKey[id.Key()] = contentItem{Physical: sec.Name, Labels: sec.Labels}
	}
	keys := unionKeys(desiredByKey, staleByKey)
	if len(keys) == 0 {
		return
	}
	_, _ = fmt.Fprintln(out, "  secrets:")
	for _, key := range keys {
		desiredItem, hasDesired := desiredByKey[key]
		staleItem, hasStale := staleByKey[key]
		label := desiredItem
		if !hasDesired {
			label = staleItem
		}
		reason := "content changed"
		switch {
		case hasDesired && !hasStale:
			reason = "new secret"
		case hasStale && !hasDesired:
			reason = "removed secret"
		}
		_, _ = fmt.Fprintf(out, "    - %s reason=%s content=hidden\n", formatDefIdentity("secret", label.Physical, label.Labels), reason)
	}
}

func printServiceContentDiffs(out io.Writer, states []apply.ServiceState) {
	if len(states) == 0 {
		return
	}
	_, _ = fmt.Fprintln(out, "  services:")
	for _, state := range states {
		if state.IntentCurrent == nil || state.IntentDesired == nil {
			continue
		}
		scope := cmdutil.ServiceScopeLabel(state.Stack, state.Partition, state.Service)
		_, _ = fmt.Fprintf(out, "    - %s\n", scope)
		diffSet := make(map[string]struct{}, len(state.IntentDiffs))
		for _, diff := range state.IntentDiffs {
			diffSet[diff] = struct{}{}
		}
		if _, ok := diffSet["labels"]; ok {
			printValueDiff(out, "labels", state.IntentCurrent.Labels, state.IntentDesired.Labels)
		}
		if _, ok := diffSet["env"]; ok {
			printValueDiff(out, "env", state.IntentCurrent.Env, state.IntentDesired.Env)
		}
		if _, ok := diffSet["args"]; ok {
			printValueDiff(out, "args", state.IntentCurrent.Args, state.IntentDesired.Args)
		}
		if _, ok := diffSet["configs"]; ok {
			printValueDiff(out, "configs", state.IntentCurrent.Configs, state.IntentDesired.Configs)
		}
		if !state.MountsMatch {
			printValueDiff(out, "mounts", serviceMountSnapshot(state.IntentCurrent), serviceMountSnapshot(state.IntentDesired))
		}
	}
}

func printValueDiff(out io.Writer, label string, before, after any) {
	beforeText := formatStructuredValue(before)
	afterText := formatStructuredValue(after)
	diff, err := diffLinesText(beforeText, afterText)
	if err != nil {
		_, _ = fmt.Fprintf(out, "      %s diff error: %v\n", label, err)
		return
	}
	_, _ = fmt.Fprintf(out, "      %s:\n", label)
	for _, line := range diff {
		_, _ = fmt.Fprintf(out, "        %s\n", line)
	}
}

func formatStructuredValue(value any) string {
	encoded, err := marshalIndentNoHTMLEscape(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(encoded)
}

func serviceMountSnapshot(snapshot *apply.ServiceIntentSnapshot) map[string]any {
	if snapshot == nil {
		return nil
	}
	return map[string]any{
		"configs": snapshot.Configs,
		"secrets": snapshot.Secrets,
		"volumes": snapshot.Volumes,
	}
}

func semanticDiffLines(before, after string) ([]string, error) {
	return semanticDiffLinesWithContext(before, after, 0)
}

func semanticDiffLinesWithContext(before, after string, contextLines int) ([]string, error) {
	if normalized, ok := normalizeStructuredContent(before); ok {
		if normalizedAfter, okAfter := normalizeStructuredContent(after); okAfter {
			return diffLinesTextWithContext(normalized, normalizedAfter, contextLines)
		}
	}
	return diffLinesTextWithContext(before, after, contextLines)
}

func normalizeStructuredContent(input string) (string, bool) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return input, false
	}
	var parsed any
	if err := json.Unmarshal([]byte(input), &parsed); err == nil {
		encoded, err := marshalIndentNoHTMLEscape(parsed)
		if err != nil {
			return input, false
		}
		return string(encoded), true
	}
	if err := yaml.Unmarshal([]byte(input), &parsed); err == nil {
		normalized := yamlutil.NormalizeValue(parsed)
		encoded, err := marshalIndentNoHTMLEscape(normalized)
		if err != nil {
			return input, false
		}
		return string(encoded), true
	}
	return input, false
}

func marshalIndentNoHTMLEscape(value any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	encoded := bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
	return encoded, nil
}

func unionKeys[T any](left, right map[string]T) []string {
	keys := make([]string, 0, len(left)+len(right))
	seen := make(map[string]struct{}, len(left)+len(right))
	for key := range left {
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	for key := range right {
		if _, ok := seen[key]; ok {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func formatDefIdentity(kind, physical string, labels map[string]string) string {
	id := identityFromLabels(labels)
	logical := id.Name
	if logical == "" {
		logical = physical
	}
	scope := cmdutil.ScopeLabel(id.Scope())
	if scope == "" {
		scope = "unknown scope"
	}
	if physical == "" || physical == logical {
		return fmt.Sprintf("%s %q (%s)", kind, logical, scope)
	}
	return fmt.Sprintf("%s %q (%s) physical=%s", kind, logical, scope, physical)
}

func identityFromLabels(labels map[string]string) defIdentity {
	id := defIdentity{
		Project:   normalizeLabel(labels[render.LabelProject]),
		Stack:     normalizeLabel(labels[render.LabelStack]),
		Partition: normalizeLabel(labels[render.LabelPartition]),
		Service:   normalizeLabel(labels[render.LabelService]),
		Name:      labels[render.LabelName],
	}
	return id
}

func (id defIdentity) Scope() templates.Scope {
	return templates.Scope{
		Project:   id.Project,
		Stack:     id.Stack,
		Partition: id.Partition,
		Service:   id.Service,
	}
}

func (id defIdentity) Key() string {
	return strings.Join([]string{id.Project, id.Stack, id.Partition, id.Service, id.Name}, "|")
}

func normalizeLabel(value string) string {
	switch strings.TrimSpace(value) {
	case "", "none":
		return ""
	default:
		return value
	}
}
