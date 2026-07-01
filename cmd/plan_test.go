package cmd

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestFilterInferredRefWarnings(t *testing.T) {
	input := []string{
		`stack "core" service "ingress" config "foo": config_ref "bar" not found (inferred)`,
		`stack "core" service "ingress" secret "foo": secret_ref "bar" not found (inferred)`,
		`stack "core" service "ingress" config "foo": config_ref has dynamic reference`,
	}

	filtered := filterInferredRefWarnings(input)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 warning after filtering, got %d", len(filtered))
	}
	if filtered[0] != input[2] {
		t.Fatalf("unexpected warning kept: %q", filtered[0])
	}
}

func TestSplitRenderedDefItem(t *testing.T) {
	scope, item := splitRenderedDefItem(`stack core service ingress "aws_shared_credentials"`)
	if scope != "stack core service ingress" {
		t.Fatalf("unexpected scope: %q", scope)
	}
	if item != `"aws_shared_credentials"` {
		t.Fatalf("unexpected item: %q", item)
	}
}

func TestPlanOutRejectsMultipleDeployments(t *testing.T) {
	oldOut := planOutPath
	oldDeployments := opts.Deployments
	oldConfigPaths := opts.ConfigPaths
	defer func() {
		planOutPath = oldOut
		opts.Deployments = oldDeployments
		opts.ConfigPaths = oldConfigPaths
	}()
	planOutPath = "plan.yaml"
	opts.Deployments = []string{"dev", "prod"}
	opts.ConfigPaths = []string{"project.yaml"}

	err := planCmd.RunE(planCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "plan --out requires a single deployment target") {
		t.Fatalf("expected single deployment error, got %v", err)
	}
}

func TestPlanOutRejectsMultiplePartitions(t *testing.T) {
	oldOut := planOutPath
	oldPartitions := opts.Partitions
	oldConfigPaths := opts.ConfigPaths
	defer func() {
		planOutPath = oldOut
		opts.Partitions = oldPartitions
		opts.ConfigPaths = oldConfigPaths
	}()
	planOutPath = "plan.yaml"
	opts.Partitions = []string{"blue", "green"}
	opts.ConfigPaths = []string{"project.yaml"}

	err := planCmd.RunE(planCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "plan --out requires at most one partition target") {
		t.Fatalf("expected single partition error, got %v", err)
	}
}

func TestPlanOutRejectsMultipleStacks(t *testing.T) {
	oldOut := planOutPath
	oldStacks := opts.Stacks
	oldConfigPaths := opts.ConfigPaths
	defer func() {
		planOutPath = oldOut
		opts.Stacks = oldStacks
		opts.ConfigPaths = oldConfigPaths
	}()
	planOutPath = "plan.yaml"
	opts.Stacks = []string{"core", "edge"}
	opts.ConfigPaths = []string{"project.yaml"}

	err := planCmd.RunE(planCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "plan --out requires at most one stack target") {
		t.Fatalf("expected single stack error, got %v", err)
	}
}

func TestSplitMountItem(t *testing.T) {
	scope, item := splitMountItem(`config "dynamic.yml" -> /etc/traefik/dynamic.yml (stack "core" service "ingress") (inferred)`)
	if scope != `stack "core" service "ingress"` {
		t.Fatalf("unexpected scope: %q", scope)
	}
	if item != `config "dynamic.yml" -> /etc/traefik/dynamic.yml (inferred)` {
		t.Fatalf("unexpected item: %q", item)
	}
}

func TestGroupRenderedItemsSorted(t *testing.T) {
	input := []string{
		`stack tools "vault_secret"`,
		`stack core service ingress "zeta"`,
		`stack core service ingress "alpha"`,
	}
	groups := groupRenderedItems(input, splitRenderedDefItem)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if groups[0].scope != "stack core service ingress" {
		t.Fatalf("unexpected first scope: %q", groups[0].scope)
	}
	if !reflect.DeepEqual(groups[0].items, []string{`"alpha"`, `"zeta"`}) {
		t.Fatalf("unexpected first group items: %#v", groups[0].items)
	}
	if groups[1].scope != "stack tools" {
		t.Fatalf("unexpected second scope: %q", groups[1].scope)
	}
	if !reflect.DeepEqual(groups[1].items, []string{`"vault_secret"`}) {
		t.Fatalf("unexpected second group items: %#v", groups[1].items)
	}
}

func TestPlanProgressReporterEnabled(t *testing.T) {
	var buf bytes.Buffer
	reporter := newPlanProgressReporter(&buf, true)
	done := reporter.start("phase one")
	done(nil)
	out := buf.String()
	if !strings.Contains(out, "plan: phase one...") {
		t.Fatalf("expected phase start, got %q", out)
	}
	if !strings.Contains(out, "plan: phase one done (") {
		t.Fatalf("expected phase completion, got %q", out)
	}
}

func TestPlanProgressReporterDisabled(t *testing.T) {
	var buf bytes.Buffer
	reporter := newPlanProgressReporter(&buf, false)
	done := reporter.start("phase one")
	done(nil)
	if buf.Len() != 0 {
		t.Fatalf("expected no output, got %q", buf.String())
	}
}
