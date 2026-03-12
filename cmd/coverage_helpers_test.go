package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/cmmoran/swarmcp/internal/apply"
	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/render"
	"github.com/cmmoran/swarmcp/internal/state"
	"github.com/cmmoran/swarmcp/internal/swarm"
	"github.com/docker/docker/api/types/mount"
)

func TestApplyPlanHelpers(t *testing.T) {
	t.Run("planStatePath validates and creates state dir", func(t *testing.T) {
		if _, err := planStatePath(""); err == nil {
			t.Fatalf("expected empty config path to fail")
		}

		dir := t.TempDir()
		configPath := filepath.Join(dir, "project.yaml")
		got, err := planStatePath(configPath)
		if err != nil {
			t.Fatalf("planStatePath: %v", err)
		}
		want := filepath.Join(dir, ".swarmcp", "project.state")
		if got != want {
			t.Fatalf("unexpected state path: want %q got %q", want, got)
		}
		if _, err := os.Stat(filepath.Dir(got)); err != nil {
			t.Fatalf("expected state dir to exist: %v", err)
		}
	})

	t.Run("plan summaries and deploy merging", func(t *testing.T) {
		deploys := []apply.StackDeploy{
			{Name: "beta", ServiceCreates: 1, ServiceUpdates: 2},
			{Name: "alpha", ServiceCreates: 3, ServiceUpdates: 4},
		}
		stacks, creates, updates := planDeploySummary(deploys)
		if !reflect.DeepEqual(stacks, []string{"beta", "alpha"}) {
			t.Fatalf("unexpected stacks: %#v", stacks)
		}
		if creates != 4 || updates != 6 {
			t.Fatalf("unexpected create/update counts: %d %d", creates, updates)
		}

		merged := mergeStackDeploys(
			[]apply.StackDeploy{{Name: "beta", ServiceCreates: 10}, {Name: "alpha", ServiceCreates: 20}},
			[]apply.StackDeploy{{Name: "gamma", ServiceCreates: 30}, {Name: "alpha", ServiceCreates: 99}},
		)
		if got := []string{merged[0].Name, merged[1].Name, merged[2].Name}; !reflect.DeepEqual(got, []string{"alpha", "beta", "gamma"}) {
			t.Fatalf("unexpected merged order: %#v", got)
		}
		if merged[0].ServiceCreates != 20 {
			t.Fatalf("expected primary deploy to win for duplicate stack, got %#v", merged[0])
		}

		summary := buildPlanSummary(apply.Plan{
			CreateNetworks: []swarm.NetworkSpec{{Name: "net1"}, {Name: "net2"}},
			CreateConfigs:  []swarm.ConfigSpec{{Name: "cfg"}},
			CreateSecrets:  []swarm.SecretSpec{{Name: "sec1"}, {Name: "sec2"}},
			StackDeploys:   []apply.StackDeploy{{Name: "core"}},
			DeleteConfigs:  []swarm.Config{{Name: "cfg1"}, {Name: "cfg2"}},
			DeleteSecrets:  []swarm.Secret{{Name: "sec1"}},
			SkippedDeletes: apply.SkippedDeletes{Configs: 3, Secrets: 4},
		})
		if summary.NetworksCreated != 2 || summary.ConfigsCreated != 1 || summary.SecretsCreated != 2 || summary.StacksDeployed != 1 {
			t.Fatalf("unexpected summary: %#v", summary)
		}
		if summary.ConfigsRemoved != 2 || summary.SecretsRemoved != 1 || summary.ConfigsSkipped != 3 || summary.SecretsSkipped != 4 {
			t.Fatalf("unexpected summary removals/skips: %#v", summary)
		}
		if planSummaryZero(summary) {
			t.Fatalf("expected non-zero summary")
		}
		if !planSummaryZero(state.PlanSummary{}) {
			t.Fatalf("expected zero summary to be considered empty")
		}
		if !planSummariesEqual(summary, summary) {
			t.Fatalf("expected identical summaries to compare equal")
		}
		if planSummariesEqual(summary, state.PlanSummary{}) {
			t.Fatalf("expected different summaries to compare unequal")
		}
	})
}

func TestLoadStateCacheFiltersByScope(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "project.yaml")
	cfg := &config.Config{Project: config.Project{Name: "demo", Deployment: "prod"}}

	statePath, err := planStatePath(configPath)
	if err != nil {
		t.Fatalf("planStatePath: %v", err)
	}
	cached := state.State{
		Command:    "apply",
		ConfigPath: configPath,
		Project:    "demo",
		Deployment: "prod",
		Partition:  "blue",
		Stack:      "core",
	}
	if err := state.Write(statePath, cached); err != nil {
		t.Fatalf("write state: %v", err)
	}

	if got, ok := loadStateCache(configPath, cfg, "blue", "core"); !ok || got.Project != "demo" {
		t.Fatalf("expected matching cache hit, got ok=%v state=%#v", ok, got)
	}
	if _, ok := loadStateCache(configPath, cfg, "green", "core"); ok {
		t.Fatalf("expected partition mismatch to reject cache")
	}
	if _, ok := loadStateCache(filepath.Join(dir, "other.yaml"), cfg, "blue", "core"); ok {
		t.Fatalf("expected config path mismatch to reject cache")
	}
	if _, ok := loadStateCache(configPath, &config.Config{Project: config.Project{Name: "other", Deployment: "prod"}}, "blue", "core"); ok {
		t.Fatalf("expected project mismatch to reject cache")
	}
}

func TestDiffHelpers(t *testing.T) {
	states := []apply.ServiceState{
		{Stack: "core", Service: "api", IntentMatch: false, IntentDiffs: []string{"image"}},
		{Stack: "core", Partition: "blue", Service: "web", Missing: true},
		{Stack: "core", Partition: "blue", Service: "jobs", IntentMatch: true, Unmanaged: []string{"labels"}},
		{Stack: "alpha", Service: "db", Unmanaged: []string{"mounts"}},
	}
	changed, missing := splitServiceStates(states)
	if len(changed) != 2 || len(missing) != 1 {
		t.Fatalf("unexpected split result: changed=%d missing=%d", len(changed), len(missing))
	}

	unmanaged := unmanagedServiceStates(states)
	if got := []string{
		unmanaged[0].Stack + "/" + unmanaged[0].Partition + "/" + unmanaged[0].Service,
		unmanaged[1].Stack + "/" + unmanaged[1].Partition + "/" + unmanaged[1].Service,
	}; !reflect.DeepEqual(got, []string{"alpha//db", "core/blue/jobs"}) {
		t.Fatalf("unexpected unmanaged order: %#v", got)
	}

	deltas := diffStateSummary(
		state.PlanSummary{ConfigsCreated: 1, ServicesUpdated: 2},
		state.PlanSummary{ConfigsCreated: 3, ServicesUpdated: 2, StacksDeployed: 1},
	)
	if len(deltas) != 2 {
		t.Fatalf("expected 2 deltas, got %#v", deltas)
	}
	if deltas[0].label != "configs to create" || deltas[1].label != "stacks to deploy" {
		t.Fatalf("unexpected delta labels: %#v", deltas)
	}

	if got := uniqueServiceStacks(
		[]apply.ServiceState{{Stack: "core", Partition: "blue"}, {Stack: "core", Partition: "blue"}},
		[]apply.ServiceState{{Stack: "core", Partition: "green"}, {Stack: "tools"}},
	); got != 3 {
		t.Fatalf("unexpected unique stack count: %d", got)
	}
}

func TestDiffDebugHelpers(t *testing.T) {
	t.Run("structured normalization preserves semantic equality", func(t *testing.T) {
		before := "b: 2\na: 1\n"
		after := "{\"a\":1,\"b\":2}"
		diff, err := semanticDiffLines(before, after)
		if err != nil {
			t.Fatalf("semanticDiffLines: %v", err)
		}
		if !reflect.DeepEqual(diff, []string{"(no content changes)"}) {
			t.Fatalf("expected no semantic diff, got %#v", diff)
		}
	})

	t.Run("structured formatting and service mount snapshot", func(t *testing.T) {
		formatted := formatStructuredValue(map[string]any{"b": 2, "a": 1})
		if !strings.Contains(formatted, "\"a\": 1") || !strings.Contains(formatted, "\"b\": 2") {
			t.Fatalf("unexpected formatted value: %s", formatted)
		}

		snapshot := serviceMountSnapshot(&apply.ServiceIntentSnapshot{
			Configs: []apply.ServiceMount{{Name: "cfg"}},
			Secrets: []apply.ServiceMount{{Name: "sec"}},
			Volumes: []mount.Mount{{Target: "/data"}},
		})
		if snapshot["configs"] == nil || snapshot["secrets"] == nil || snapshot["volumes"] == nil {
			t.Fatalf("unexpected mount snapshot: %#v", snapshot)
		}
		if serviceMountSnapshot(nil) != nil {
			t.Fatalf("expected nil snapshot for nil intent")
		}
	})

	t.Run("identity helpers are stable", func(t *testing.T) {
		keys := unionKeys(map[string]int{"b": 1, "a": 2}, map[string]int{"c": 3, "a": 4})
		if !reflect.DeepEqual(keys, []string{"a", "b", "c"}) {
			t.Fatalf("unexpected union keys: %#v", keys)
		}

		labels := map[string]string{
			render.LabelProject:   "demo",
			render.LabelStack:     "core",
			render.LabelPartition: "none",
			render.LabelService:   "api",
			render.LabelName:      "app-config",
		}
		identity := identityFromLabels(labels)
		if identity.Partition != "" {
			t.Fatalf("expected \"none\" partition label to normalize empty, got %#v", identity)
		}
		got := formatDefIdentity("config", "demo_core_api_app-config_v1", labels)
		if !strings.Contains(got, `config "app-config"`) || !strings.Contains(got, "stack core service api") {
			t.Fatalf("unexpected formatted identity: %s", got)
		}
	})

	t.Run("debug summary prints unmanaged and changed items", func(t *testing.T) {
		var out bytes.Buffer
		printDiffDebugSummary(&out, apply.StatusReport{
			MissingNetworks: []swarm.NetworkSpec{{Name: "core-net"}},
			MissingConfigs: []swarm.ConfigSpec{{
				Name: "demo_cfg_v1",
				Labels: map[string]string{
					render.LabelProject: "demo",
					render.LabelStack:   "core",
					render.LabelName:    "cfg",
				},
			}},
			Services: []apply.ServiceState{{Stack: "core", Service: "api", Unmanaged: []string{"labels"}}},
		}, []apply.ServiceState{{Stack: "core", Service: "api", IntentDiffs: []string{"image"}}}, []apply.ServiceState{{Stack: "core", Service: "web", Missing: true}})
		got := out.String()
		if !strings.Contains(got, "configs:") || !strings.Contains(got, "networks:") || !strings.Contains(got, "services:") {
			t.Fatalf("unexpected summary output: %s", got)
		}
		if !strings.Contains(got, "reason=unmanaged drift") || !strings.Contains(got, "reason=intent drift") || !strings.Contains(got, "reason=missing") {
			t.Fatalf("expected unmanaged/intent/missing markers in output: %s", got)
		}
	})
}
