package apply

import (
	"testing"
	"time"

	"github.com/cmmoran/swarmcp/internal/config"
)

func TestSwarmUpdateConfig(t *testing.T) {
	policy := &config.UpdatePolicy{
		Parallelism:     new(2),
		Delay:           new("5s"),
		FailureAction:   new("pause"),
		Monitor:         new("30s"),
		MaxFailureRatio: new(0.25),
		Order:           new("start-first"),
	}
	converted, err := swarmUpdateConfig(policy)
	if err != nil {
		t.Fatalf("swarmUpdateConfig: %v", err)
	}
	if converted == nil {
		t.Fatalf("expected update config")
	}
	if converted.Parallelism != 2 {
		t.Fatalf("unexpected parallelism: %d", converted.Parallelism)
	}
	if converted.Delay != 5*time.Second {
		t.Fatalf("unexpected delay: %v", converted.Delay)
	}
	if converted.FailureAction != "pause" {
		t.Fatalf("unexpected failure_action: %q", converted.FailureAction)
	}
	if converted.Monitor != 30*time.Second {
		t.Fatalf("unexpected monitor: %v", converted.Monitor)
	}
	if converted.MaxFailureRatio != 0.25 {
		t.Fatalf("unexpected max_failure_ratio: %v", converted.MaxFailureRatio)
	}
	if converted.Order != "start-first" {
		t.Fatalf("unexpected order: %q", converted.Order)
	}
}
