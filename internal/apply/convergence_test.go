package apply

import (
	"reflect"
	"testing"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/render"
	dockerapi "github.com/docker/docker/api/types/swarm"
	"go.yaml.in/yaml/v4"
)

func TestConvergenceMinimalService(t *testing.T) {
	cfg := minimalConfig()
	stack := cfg.Stacks["app"]
	service := stack.Services["web"]
	build, err := buildServiceIntent(cfg, "app", stack, "", "web", service, nil, false, map[defKey]string{})
	if err != nil {
		t.Fatalf("buildServiceIntent: %v", err)
	}
	desiredIntent := build.Intent
	spec := applyIntentToSpec(dockerapi.ServiceSpec{Annotations: dockerapi.Annotations{Name: "app_web", Labels: build.Labels}}, desiredIntent)
	currentIntent := intentFromSpec(spec, map[string]string{})
	if !intentEqual(desiredIntent, currentIntent) {
		t.Fatalf("apply vs status intent mismatch")
	}

	composeSvc := mustComposeService(t, cfg, "")
	if composeSvc.Image != desiredIntent.Image {
		t.Fatalf("compose image mismatch: %q != %q", composeSvc.Image, desiredIntent.Image)
	}
	if composeSvc.Deploy == nil || !reflect.DeepEqual(composeSvc.Deploy.Labels, desiredIntent.Labels) {
		t.Fatalf("compose labels mismatch")
	}
}

func TestConvergencePolicyFixture(t *testing.T) {
	cfg := policyConfig()
	stack := cfg.Stacks["app"]
	service := stack.Services["web"]
	build, err := buildServiceIntent(cfg, "app", stack, "", "web", service, nil, false, map[defKey]string{})
	if err != nil {
		t.Fatalf("buildServiceIntent: %v", err)
	}
	desiredIntent := build.Intent
	spec := applyIntentToSpec(dockerapi.ServiceSpec{Annotations: dockerapi.Annotations{Name: "app_web", Labels: build.Labels}}, desiredIntent)
	currentIntent := intentFromSpec(spec, map[string]string{})
	if !restartPoliciesEqual(desiredIntent.RestartPolicy, currentIntent.RestartPolicy) {
		t.Fatalf("restart policy mismatch")
	}
	if !updateConfigsEqual(desiredIntent.UpdateConfig, currentIntent.UpdateConfig) {
		t.Fatalf("update policy mismatch")
	}
	if !updateConfigsEqual(desiredIntent.RollbackConfig, currentIntent.RollbackConfig) {
		t.Fatalf("rollback policy mismatch")
	}

	composeSvc := mustComposeService(t, cfg, "")
	if composeSvc.Deploy == nil {
		t.Fatalf("missing deploy spec")
	}
	assertComposeRestartPolicy(t, composeSvc.Deploy.RestartPolicy, desiredIntent.RestartPolicy, true, true)
	assertComposeUpdatePolicy(t, "update", composeSvc.Deploy.UpdateConfig, desiredIntent.UpdateConfig, true, true, true, true, false)
	assertComposeUpdatePolicy(t, "rollback", composeSvc.Deploy.RollbackConfig, desiredIntent.RollbackConfig, true, true, true, true, false)
}

func TestConvergenceLabelIdentityFixture(t *testing.T) {
	cfg := labelConfig()
	stack := cfg.Stacks["app"]
	service := stack.Services["web"]
	build, err := buildServiceIntent(cfg, "app", stack, "dev", "web", service, nil, false, map[defKey]string{})
	if err != nil {
		t.Fatalf("buildServiceIntent: %v", err)
	}
	desiredIntent := build.Intent
	composeSvc := mustComposeService(t, cfg, "dev")
	if composeSvc.Deploy == nil || !reflect.DeepEqual(composeSvc.Deploy.Labels, desiredIntent.Labels) {
		t.Fatalf("compose labels mismatch")
	}

	bad := labelConfig()
	bad.Stacks["app"].Services["web"] = config.Service{
		Image:  "nginx:latest",
		Labels: map[string]string{"swarmcp.io/bad": "x"},
	}
	_, err = buildServiceIntent(bad, "app", bad.Stacks["app"], "dev", "web", bad.Stacks["app"].Services["web"], nil, false, map[defKey]string{})
	if err == nil {
		t.Fatalf("expected reserved-prefix label error")
	}

	spec := applyIntentToSpec(dockerapi.ServiceSpec{Annotations: dockerapi.Annotations{Name: "app_web", Labels: desiredIntent.Labels}}, desiredIntent)
	spec.Annotations.Labels[render.LabelProject] = "other"
	currentIntent := intentFromSpec(spec, map[string]string{})
	diffs := intentDiffs(currentIntent, desiredIntent)
	if !containsString(diffs, "labels") {
		t.Fatalf("expected label drift, got %v", diffs)
	}
}

func minimalConfig() *config.Config {
	return &config.Config{
		Project: config.Project{Name: "proj"},
		Stacks: map[string]config.Stack{
			"app": {
				Mode: "shared",
				Services: map[string]config.Service{
					"web": {Image: "nginx:latest"},
				},
			},
		},
	}
}

func policyConfig() *config.Config {
	updateDelay := "0s"
	failureAction := "pause"
	parallelism := 0
	maxFailureRatio := 0.0
	order := "start-first"

	return &config.Config{
		Project: config.Project{Name: "proj"},
		Stacks: map[string]config.Stack{
			"app": {
				Mode: "shared",
				Services: map[string]config.Service{
					"web": {
						Image: "nginx:latest",
						RestartPolicy: &config.RestartPolicy{
							Condition:   new("on-failure"),
							Delay:       new("0s"),
							MaxAttempts: new(0),
						},
						UpdateConfig: &config.UpdatePolicy{
							Parallelism:     &parallelism,
							Delay:           &updateDelay,
							FailureAction:   &failureAction,
							MaxFailureRatio: &maxFailureRatio,
							Order:           &order,
						},
						RollbackConfig: &config.UpdatePolicy{
							Parallelism:     &parallelism,
							Delay:           &updateDelay,
							FailureAction:   &failureAction,
							MaxFailureRatio: &maxFailureRatio,
							Order:           &order,
						},
					},
				},
			},
		},
	}
}

func labelConfig() *config.Config {
	return &config.Config{
		Project: config.Project{Name: "proj", Partitions: []string{"dev"}},
		Stacks: map[string]config.Stack{
			"app": {
				Mode: "partitioned",
				Services: map[string]config.Service{
					"web": {
						Image: "nginx:latest",
						Labels: map[string]string{
							"role.{partition}": "web",
						},
					},
				},
			},
		},
	}
}

func mustComposeService(t *testing.T, cfg *config.Config, partition string) composeService {
	desired := DesiredState{}
	var partitionFilters []string
	if partition != "" {
		partitionFilters = []string{partition}
	}
	deploys, err := BuildStackDeploys(cfg, desired, nil, partitionFilters, nil, nil, nil, nil, false)
	if err != nil {
		t.Fatalf("BuildStackDeploys: %v", err)
	}
	if len(deploys) == 0 {
		t.Fatalf("expected stack deploy")
	}
	var compose composeFile
	if err := yaml.Unmarshal(deploys[0].Compose, &compose); err != nil {
		t.Fatalf("compose unmarshal: %v", err)
	}
	svc, ok := compose.Services["web"]
	if !ok {
		t.Fatalf("missing compose service")
	}
	return svc
}

func assertComposeRestartPolicy(t *testing.T, got *composeRestartPolicy, want *dockerapi.RestartPolicy, delaySet bool, maxAttemptsSet bool) {
	if want == nil {
		if got != nil {
			t.Fatalf("expected nil restart policy")
		}
		return
	}
	if got == nil {
		t.Fatalf("missing restart policy")
	}
	if got.Condition != string(want.Condition) {
		t.Fatalf("restart condition mismatch: %q != %q", got.Condition, want.Condition)
	}
	if delaySet {
		if got.Delay == nil || *got.Delay != want.Delay.String() {
			t.Fatalf("restart delay mismatch: %v != %v", got.Delay, want.Delay)
		}
	}
	if maxAttemptsSet {
		if got.MaxAttempts == nil || *got.MaxAttempts != *want.MaxAttempts {
			t.Fatalf("restart max_attempts mismatch")
		}
	}
}

func assertComposeUpdatePolicy(t *testing.T, name string, got *composeUpdateConfig, want *dockerapi.UpdateConfig, parallelismSet bool, delaySet bool, failureActionSet bool, maxFailureRatioSet bool, monitorSet bool) {
	if want == nil {
		if got != nil {
			t.Fatalf("expected nil %s policy", name)
		}
		return
	}
	if got == nil {
		t.Fatalf("missing %s policy", name)
	}
	if parallelismSet {
		if got.Parallelism == nil || *got.Parallelism != want.Parallelism {
			t.Fatalf("%s parallelism mismatch", name)
		}
	}
	if delaySet {
		if got.Delay == nil || *got.Delay != want.Delay.String() {
			t.Fatalf("%s delay mismatch", name)
		}
	}
	if failureActionSet && got.FailureAction != want.FailureAction {
		t.Fatalf("%s failure_action mismatch", name)
	}
	if monitorSet {
		if got.Monitor == nil || *got.Monitor != want.Monitor.String() {
			t.Fatalf("%s monitor mismatch", name)
		}
	}
	if maxFailureRatioSet {
		if got.MaxFailureRatio == nil || *got.MaxFailureRatio != float64(want.MaxFailureRatio) {
			t.Fatalf("%s max_failure_ratio mismatch", name)
		}
	}
	if want.Order != "" && got.Order != want.Order {
		t.Fatalf("%s order mismatch", name)
	}
}

func containsString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}
