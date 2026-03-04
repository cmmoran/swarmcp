package apply

import (
	"testing"
	"time"

	"github.com/cmmoran/swarmcp/internal/config"
	dockerapi "github.com/docker/docker/api/types/swarm"
)

func TestSwarmRestartPolicy(t *testing.T) {
	policy := &config.RestartPolicy{
		Condition:   new("on-failure"),
		Delay:       new("5s"),
		MaxAttempts: new(1),
		Window:      new("2m"),
	}
	converted, err := swarmRestartPolicy(policy)
	if err != nil {
		t.Fatalf("swarmRestartPolicy: %v", err)
	}
	if converted == nil {
		t.Fatalf("expected restart policy")
	}
	if converted.Condition != dockerapi.RestartPolicyConditionOnFailure {
		t.Fatalf("expected condition on-failure, got %q", converted.Condition)
	}
	if converted.Delay == nil || *converted.Delay != 5*time.Second {
		t.Fatalf("unexpected delay: %v", converted.Delay)
	}
	if converted.Window == nil || *converted.Window != 2*time.Minute {
		t.Fatalf("unexpected window: %v", converted.Window)
	}
	if converted.MaxAttempts == nil || *converted.MaxAttempts != 1 {
		t.Fatalf("unexpected max_attempts: %v", converted.MaxAttempts)
	}
}

func TestIntentDiffsRestartPolicy(t *testing.T) {
	current := serviceIntent{
		RestartPolicy: &dockerapi.RestartPolicy{Condition: dockerapi.RestartPolicyConditionAny},
	}
	desired := serviceIntent{
		RestartPolicy: &dockerapi.RestartPolicy{Condition: dockerapi.RestartPolicyConditionOnFailure},
	}
	diffs := intentDiffs(current, desired)
	if !stringSliceContains(diffs, "restart_policy") {
		t.Fatalf("expected restart_policy diff, got %v", diffs)
	}
}

func stringSliceContains(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}
