package config

import (
	"fmt"
	"strings"
	"time"
)

var updateFailureActions = map[string]struct{}{
	"pause":    {},
	"continue": {},
	"rollback": {},
}

var updateOrders = map[string]struct{}{
	"stop-first":  {},
	"start-first": {},
}

func NormalizeUpdatePolicyFailureAction(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return "", fmt.Errorf("update_config.failure_action: must not be empty")
	}
	if _, ok := updateFailureActions[value]; !ok {
		return "", fmt.Errorf("update_config.failure_action: invalid value %q", raw)
	}
	return value, nil
}

func NormalizeUpdatePolicyOrder(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return "", fmt.Errorf("update_config.order: must not be empty")
	}
	if _, ok := updateOrders[value]; !ok {
		return "", fmt.Errorf("update_config.order: invalid value %q", raw)
	}
	return value, nil
}

func ParseUpdatePolicyDuration(field string, raw string) (time.Duration, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, fmt.Errorf("update_config.%s: must not be empty", field)
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("update_config.%s: invalid duration %q", field, raw)
	}
	return duration, nil
}

func MergeUpdatePolicies(policies ...*UpdatePolicy) *UpdatePolicy {
	var out UpdatePolicy
	set := false
	for _, policy := range policies {
		if policy == nil {
			continue
		}
		if policy.Parallelism != nil {
			value := *policy.Parallelism
			out.Parallelism = &value
			set = true
		}
		if policy.Delay != nil {
			value := strings.TrimSpace(*policy.Delay)
			if value != "" {
				out.Delay = &value
				set = true
			}
		}
		if policy.FailureAction != nil {
			value := strings.TrimSpace(*policy.FailureAction)
			if value != "" {
				out.FailureAction = &value
				set = true
			}
		}
		if policy.Monitor != nil {
			value := strings.TrimSpace(*policy.Monitor)
			if value != "" {
				out.Monitor = &value
				set = true
			}
		}
		if policy.MaxFailureRatio != nil {
			value := *policy.MaxFailureRatio
			out.MaxFailureRatio = &value
			set = true
		}
		if policy.Order != nil {
			value := strings.TrimSpace(*policy.Order)
			if value != "" {
				out.Order = &value
				set = true
			}
		}
	}
	if !set {
		return nil
	}
	return &out
}

func StackPartitionUpdateConfig(stack Stack, partition string) *UpdatePolicy {
	if partition == "" || len(stack.Partitions) == 0 {
		return nil
	}
	if part, ok := stack.Partitions[partition]; ok {
		return part.UpdateConfig
	}
	return nil
}

func StackPartitionRollbackConfig(stack Stack, partition string) *UpdatePolicy {
	if partition == "" || len(stack.Partitions) == 0 {
		return nil
	}
	if part, ok := stack.Partitions[partition]; ok {
		return part.RollbackConfig
	}
	return nil
}

func validateUpdatePolicy(scope string, policy *UpdatePolicy) []string {
	if policy == nil {
		return nil
	}
	var errs []string
	if policy.Parallelism != nil && *policy.Parallelism < 0 {
		errs = append(errs, fmt.Sprintf("%s.parallelism: must be >= 0", scope))
	}
	if policy.Delay != nil {
		value := strings.TrimSpace(*policy.Delay)
		if value == "" {
			errs = append(errs, fmt.Sprintf("%s.delay: must not be empty", scope))
		} else if _, err := time.ParseDuration(value); err != nil {
			errs = append(errs, fmt.Sprintf("%s.delay: invalid duration %q", scope, *policy.Delay))
		}
	}
	if policy.FailureAction != nil {
		value := strings.ToLower(strings.TrimSpace(*policy.FailureAction))
		if value == "" {
			errs = append(errs, fmt.Sprintf("%s.failure_action: must not be empty", scope))
		} else if _, ok := updateFailureActions[value]; !ok {
			errs = append(errs, fmt.Sprintf("%s.failure_action: invalid value %q", scope, *policy.FailureAction))
		}
	}
	if policy.Monitor != nil {
		value := strings.TrimSpace(*policy.Monitor)
		if value == "" {
			errs = append(errs, fmt.Sprintf("%s.monitor: must not be empty", scope))
		} else if _, err := time.ParseDuration(value); err != nil {
			errs = append(errs, fmt.Sprintf("%s.monitor: invalid duration %q", scope, *policy.Monitor))
		}
	}
	if policy.MaxFailureRatio != nil {
		if *policy.MaxFailureRatio < 0 || *policy.MaxFailureRatio > 1 {
			errs = append(errs, fmt.Sprintf("%s.max_failure_ratio: must be between 0 and 1", scope))
		}
	}
	if policy.Order != nil {
		value := strings.ToLower(strings.TrimSpace(*policy.Order))
		if value == "" {
			errs = append(errs, fmt.Sprintf("%s.order: must not be empty", scope))
		} else if _, ok := updateOrders[value]; !ok {
			errs = append(errs, fmt.Sprintf("%s.order: invalid value %q", scope, *policy.Order))
		}
	}
	return errs
}
