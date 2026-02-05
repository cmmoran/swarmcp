package config

import (
	"fmt"
	"strings"
	"time"
)

var restartPolicyConditions = map[string]struct{}{
	"none":       {},
	"on-failure": {},
	"any":        {},
}

func NormalizeRestartPolicyCondition(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return "", fmt.Errorf("restart_policy.condition: must not be empty")
	}
	if _, ok := restartPolicyConditions[value]; !ok {
		return "", fmt.Errorf("restart_policy.condition: invalid value %q", raw)
	}
	return value, nil
}

func ParseRestartPolicyDuration(field string, raw string) (time.Duration, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, fmt.Errorf("restart_policy.%s: must not be empty", field)
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("restart_policy.%s: invalid duration %q", field, raw)
	}
	return duration, nil
}

func MergeRestartPolicies(policies ...*RestartPolicy) *RestartPolicy {
	var out RestartPolicy
	set := false
	for _, policy := range policies {
		if policy == nil {
			continue
		}
		if policy.Condition != nil {
			value := strings.TrimSpace(*policy.Condition)
			if value != "" {
				out.Condition = &value
				set = true
			}
		}
		if policy.Delay != nil {
			value := strings.TrimSpace(*policy.Delay)
			if value != "" {
				out.Delay = &value
				set = true
			}
		}
		if policy.MaxAttempts != nil {
			value := *policy.MaxAttempts
			out.MaxAttempts = &value
			set = true
		}
		if policy.Window != nil {
			value := strings.TrimSpace(*policy.Window)
			if value != "" {
				out.Window = &value
				set = true
			}
		}
	}
	if !set {
		return nil
	}
	return &out
}

func StackPartitionRestartPolicy(stack Stack, partition string) *RestartPolicy {
	if partition == "" || len(stack.Partitions) == 0 {
		return nil
	}
	if part, ok := stack.Partitions[partition]; ok {
		return part.RestartPolicy
	}
	return nil
}

func validateRestartPolicy(scope string, policy *RestartPolicy) []string {
	if policy == nil {
		return nil
	}
	var errs []string
	if policy.Condition != nil {
		value := strings.ToLower(strings.TrimSpace(*policy.Condition))
		if value == "" {
			errs = append(errs, fmt.Sprintf("%s.condition: must not be empty", scope))
		} else if _, ok := restartPolicyConditions[value]; !ok {
			errs = append(errs, fmt.Sprintf("%s.condition: invalid value %q", scope, *policy.Condition))
		}
	}
	if policy.Delay != nil {
		value := strings.TrimSpace(*policy.Delay)
		if value == "" {
			errs = append(errs, fmt.Sprintf("%s.delay: must not be empty", scope))
		} else if _, err := time.ParseDuration(value); err != nil {
			errs = append(errs, fmt.Sprintf("%s.delay: invalid duration %q", scope, *policy.Delay))
		}
	}
	if policy.MaxAttempts != nil && *policy.MaxAttempts < 0 {
		errs = append(errs, fmt.Sprintf("%s.max_attempts: must be >= 0", scope))
	}
	if policy.Window != nil {
		value := strings.TrimSpace(*policy.Window)
		if value == "" {
			errs = append(errs, fmt.Sprintf("%s.window: must not be empty", scope))
		} else if _, err := time.ParseDuration(value); err != nil {
			errs = append(errs, fmt.Sprintf("%s.window: invalid duration %q", scope, *policy.Window))
		}
	}
	return errs
}
