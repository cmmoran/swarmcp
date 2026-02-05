package apply

import (
	"fmt"
	"strings"
	"time"

	"github.com/cmmoran/swarmcp/internal/config"
	dockerapi "github.com/docker/docker/api/types/swarm"
)

type composeRestartPolicy struct {
	Condition   string  `yaml:"condition,omitempty"`
	Delay       *string `yaml:"delay,omitempty"`
	MaxAttempts *uint64 `yaml:"max_attempts,omitempty"`
	Window      *string `yaml:"window,omitempty"`
}

func swarmRestartPolicy(policy *config.RestartPolicy) (*dockerapi.RestartPolicy, error) {
	if policy == nil {
		return nil, nil
	}
	out := &dockerapi.RestartPolicy{}
	if policy.Condition != nil {
		value, err := config.NormalizeRestartPolicyCondition(*policy.Condition)
		if err != nil {
			return nil, err
		}
		out.Condition = dockerapi.RestartPolicyCondition(value)
	}
	if policy.Delay != nil {
		delay, err := config.ParseRestartPolicyDuration("delay", *policy.Delay)
		if err != nil {
			return nil, err
		}
		out.Delay = &delay
	}
	if policy.MaxAttempts != nil {
		if *policy.MaxAttempts < 0 {
			return nil, fmt.Errorf("restart_policy.max_attempts: must be >= 0")
		}
		value := uint64(*policy.MaxAttempts)
		out.MaxAttempts = &value
	}
	if policy.Window != nil {
		window, err := config.ParseRestartPolicyDuration("window", *policy.Window)
		if err != nil {
			return nil, err
		}
		out.Window = &window
	}
	if out.Condition == "" && out.Delay == nil && out.MaxAttempts == nil && out.Window == nil {
		return nil, nil
	}
	return out, nil
}

func composeRestartPolicySpec(policy *config.RestartPolicy) (*composeRestartPolicy, error) {
	if policy == nil {
		return nil, nil
	}
	out := &composeRestartPolicy{}
	if policy.Condition != nil {
		value, err := config.NormalizeRestartPolicyCondition(*policy.Condition)
		if err != nil {
			return nil, err
		}
		out.Condition = value
	}
	if policy.Delay != nil {
		value := strings.TrimSpace(*policy.Delay)
		if _, err := config.ParseRestartPolicyDuration("delay", value); err != nil {
			return nil, err
		}
		out.Delay = &value
	}
	if policy.MaxAttempts != nil {
		if *policy.MaxAttempts < 0 {
			return nil, fmt.Errorf("restart_policy.max_attempts: must be >= 0")
		}
		value := uint64(*policy.MaxAttempts)
		out.MaxAttempts = &value
	}
	if policy.Window != nil {
		value := strings.TrimSpace(*policy.Window)
		if _, err := config.ParseRestartPolicyDuration("window", value); err != nil {
			return nil, err
		}
		out.Window = &value
	}
	if out.Condition == "" && out.Delay == nil && out.MaxAttempts == nil && out.Window == nil {
		return nil, nil
	}
	return out, nil
}

func cloneRestartPolicy(policy *dockerapi.RestartPolicy) *dockerapi.RestartPolicy {
	if policy == nil {
		return nil
	}
	out := *policy
	if policy.Delay != nil {
		delay := *policy.Delay
		out.Delay = &delay
	}
	if policy.MaxAttempts != nil {
		value := *policy.MaxAttempts
		out.MaxAttempts = &value
	}
	if policy.Window != nil {
		window := *policy.Window
		out.Window = &window
	}
	return &out
}

func restartPoliciesEqual(left, right *dockerapi.RestartPolicy) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	if left.Condition != right.Condition {
		return false
	}
	if !durationPtrEqual(left.Delay, right.Delay) {
		return false
	}
	if !durationPtrEqual(left.Window, right.Window) {
		return false
	}
	if !uint64PtrEqual(left.MaxAttempts, right.MaxAttempts) {
		return false
	}
	return true
}

func durationPtrEqual(left, right *time.Duration) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	return *left == *right
}

func uint64PtrEqual(left, right *uint64) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	return *left == *right
}

func formatRestartPolicy(policy *dockerapi.RestartPolicy) string {
	if policy == nil {
		return "{}"
	}
	parts := make([]string, 0, 4)
	if policy.Condition != "" {
		parts = append(parts, "condition="+string(policy.Condition))
	}
	if policy.Delay != nil {
		parts = append(parts, "delay="+policy.Delay.String())
	}
	if policy.MaxAttempts != nil {
		parts = append(parts, fmt.Sprintf("max_attempts=%d", *policy.MaxAttempts))
	}
	if policy.Window != nil {
		parts = append(parts, "window="+policy.Window.String())
	}
	if len(parts) == 0 {
		return "{}"
	}
	return "{" + strings.Join(parts, ", ") + "}"
}
