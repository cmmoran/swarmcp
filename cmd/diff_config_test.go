package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/cmmoran/swarmcp/internal/swarm"
)

func TestSelectConfigVersionByIndex(t *testing.T) {
	now := time.Now()
	records := []configVersionRecord{
		{config: swarm.Config{Name: "cfg_new", CreatedAt: now}},
		{config: swarm.Config{Name: "cfg_old", CreatedAt: now.Add(-time.Hour)}},
	}
	selected, err := selectConfigVersion(records, "@1")
	if err != nil {
		t.Fatalf("select by index failed: %v", err)
	}
	if selected.config.Name != "cfg_old" {
		t.Fatalf("unexpected selected config: %q", selected.config.Name)
	}
}

func TestSelectConfigVersionByName(t *testing.T) {
	records := []configVersionRecord{
		{config: swarm.Config{Name: "cfg_alpha"}},
		{config: swarm.Config{Name: "cfg_beta"}},
	}
	selected, err := selectConfigVersion(records, "cfg_beta")
	if err != nil {
		t.Fatalf("select by name failed: %v", err)
	}
	if selected.config.Name != "cfg_beta" {
		t.Fatalf("unexpected selected config: %q", selected.config.Name)
	}
}

func TestGroupConfigVersions(t *testing.T) {
	now := time.Now()
	records := []configVersionRecord{
		{
			config: swarm.Config{Name: "cfg_b_old", CreatedAt: now.Add(-2 * time.Hour)},
			id:     defIdentity{Project: "p", Stack: "core", Service: "ingress", Name: "traefik.yml"},
		},
		{
			config: swarm.Config{Name: "cfg_b_new", CreatedAt: now.Add(-time.Hour)},
			id:     defIdentity{Project: "p", Stack: "core", Service: "ingress", Name: "traefik.yml"},
		},
		{
			config: swarm.Config{Name: "cfg_a", CreatedAt: now},
			id:     defIdentity{Project: "p", Stack: "tools", Name: "rpc.toml"},
		},
	}
	grouped := groupConfigVersions(records)
	if len(grouped) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(grouped))
	}
	if grouped[0].name != "traefik.yml" || grouped[0].rows[0].config.Name != "cfg_b_new" {
		t.Fatalf("unexpected first group ordering: %#v", grouped[0])
	}
}

func TestRenderDiffLineColor(t *testing.T) {
	green := renderDiffLine("+ add", true)
	if !strings.Contains(green, "\x1b[32m") {
		t.Fatalf("expected green ANSI code, got %q", green)
	}
	plain := renderDiffLine("+ add", false)
	if plain != "+ add" {
		t.Fatalf("expected plain line when color disabled, got %q", plain)
	}
}

func TestRelativeAgeLabels(t *testing.T) {
	now := time.Now()
	left, right := relativeAgeLabels(now, now.Add(-time.Minute))
	if left != "(newer)" || right != "(older)" {
		t.Fatalf("unexpected labels for newer left: left=%s right=%s", left, right)
	}
	left, right = relativeAgeLabels(now.Add(-time.Minute), now)
	if left != "(older)" || right != "(newer)" {
		t.Fatalf("unexpected labels for newer right: left=%s right=%s", left, right)
	}
	left, right = relativeAgeLabels(now, now)
	if left != "(same age)" || right != "(same age)" {
		t.Fatalf("unexpected labels for equal time: left=%s right=%s", left, right)
	}
}

func TestParseConfigSelectorWithQualifiers(t *testing.T) {
	parsed, err := parseConfigSelector("@0#partition=dev#stack=core#service=ingress")
	if err != nil {
		t.Fatalf("parseConfigSelector failed: %v", err)
	}
	if parsed.Base != "@0" || parsed.Partition != "dev" || parsed.Stack != "core" || parsed.Service != "ingress" {
		t.Fatalf("unexpected parse result: %#v", parsed)
	}
}

func TestParseConfigSelectorUnknownQualifier(t *testing.T) {
	_, err := parseConfigSelector("@0#region=us")
	if err == nil {
		t.Fatalf("expected parse error for unknown qualifier")
	}
}

func TestSelectConfigVersionByQualifiedIndex(t *testing.T) {
	now := time.Now()
	records := []configVersionRecord{
		{
			config: swarm.Config{Name: "cfg_dev_new", CreatedAt: now},
			id:     defIdentity{Stack: "core", Partition: "dev", Service: "ingress", Name: "traefik.yml"},
		},
		{
			config: swarm.Config{Name: "cfg_qa_new", CreatedAt: now.Add(-time.Minute)},
			id:     defIdentity{Stack: "core", Partition: "qa", Service: "ingress", Name: "traefik.yml"},
		},
		{
			config: swarm.Config{Name: "cfg_dev_old", CreatedAt: now.Add(-time.Hour)},
			id:     defIdentity{Stack: "core", Partition: "dev", Service: "ingress", Name: "traefik.yml"},
		},
	}
	selected, err := selectConfigVersion(records, "@0#partition=dev")
	if err != nil {
		t.Fatalf("select by qualified index failed: %v", err)
	}
	if selected.config.Name != "cfg_dev_new" {
		t.Fatalf("unexpected selected config: %q", selected.config.Name)
	}
}
