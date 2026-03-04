package apply

import (
	"fmt"
	"strings"

	"github.com/cmmoran/swarmcp/internal/config"
	dockerapi "github.com/docker/docker/api/types/swarm"
)

type composeUpdateConfig struct {
	Parallelism     *uint64  `yaml:"parallelism,omitempty"`
	Delay           *string  `yaml:"delay,omitempty"`
	FailureAction   string   `yaml:"failure_action,omitempty"`
	Monitor         *string  `yaml:"monitor,omitempty"`
	MaxFailureRatio *float64 `yaml:"max_failure_ratio,omitempty"`
	Order           string   `yaml:"order,omitempty"`
}

func swarmUpdateConfig(policy *config.UpdatePolicy) (*dockerapi.UpdateConfig, error) {
	if policy == nil {
		return nil, nil
	}
	out := &dockerapi.UpdateConfig{}
	if policy.Parallelism != nil {
		if *policy.Parallelism < 0 {
			return nil, fmt.Errorf("update_config.parallelism: must be >= 0")
		}
		out.Parallelism = uint64(*policy.Parallelism)
	}
	if policy.Delay != nil {
		delay, err := config.ParseUpdatePolicyDuration("delay", *policy.Delay)
		if err != nil {
			return nil, err
		}
		out.Delay = delay
	}
	if policy.FailureAction != nil {
		value, err := config.NormalizeUpdatePolicyFailureAction(*policy.FailureAction)
		if err != nil {
			return nil, err
		}
		out.FailureAction = value
	}
	if policy.Monitor != nil {
		monitor, err := config.ParseUpdatePolicyDuration("monitor", *policy.Monitor)
		if err != nil {
			return nil, err
		}
		out.Monitor = monitor
	}
	if policy.MaxFailureRatio != nil {
		if *policy.MaxFailureRatio < 0 || *policy.MaxFailureRatio > 1 {
			return nil, fmt.Errorf("update_config.max_failure_ratio: must be between 0 and 1")
		}
		out.MaxFailureRatio = float32(*policy.MaxFailureRatio)
	}
	if policy.Order != nil {
		value, err := config.NormalizeUpdatePolicyOrder(*policy.Order)
		if err != nil {
			return nil, err
		}
		out.Order = value
	}
	if isZeroUpdateConfig(out) {
		return nil, nil
	}
	return out, nil
}

func composeUpdateConfigSpec(policy *config.UpdatePolicy, name string) (*composeUpdateConfig, error) {
	if policy == nil {
		return nil, nil
	}
	out := &composeUpdateConfig{}
	if policy.Parallelism != nil {
		if *policy.Parallelism < 0 {
			return nil, fmt.Errorf("%s.parallelism: must be >= 0", name)
		}
		out.Parallelism = new(uint64(*policy.Parallelism))
	}
	if policy.Delay != nil {
		value := strings.TrimSpace(*policy.Delay)
		if _, err := config.ParseUpdatePolicyDuration("delay", value); err != nil {
			return nil, fmt.Errorf("%s.delay: %s", name, strings.TrimPrefix(err.Error(), "update_config.delay: "))
		}
		out.Delay = &value
	}
	if policy.FailureAction != nil {
		value, err := config.NormalizeUpdatePolicyFailureAction(*policy.FailureAction)
		if err != nil {
			return nil, fmt.Errorf("%s.failure_action: %s", name, strings.TrimPrefix(err.Error(), "update_config.failure_action: "))
		}
		out.FailureAction = value
	}
	if policy.Monitor != nil {
		value := strings.TrimSpace(*policy.Monitor)
		if _, err := config.ParseUpdatePolicyDuration("monitor", value); err != nil {
			return nil, fmt.Errorf("%s.monitor: %s", name, strings.TrimPrefix(err.Error(), "update_config.monitor: "))
		}
		out.Monitor = &value
	}
	if policy.MaxFailureRatio != nil {
		if *policy.MaxFailureRatio < 0 || *policy.MaxFailureRatio > 1 {
			return nil, fmt.Errorf("%s.max_failure_ratio: must be between 0 and 1", name)
		}
		out.MaxFailureRatio = new(*policy.MaxFailureRatio)
	}
	if policy.Order != nil {
		value, err := config.NormalizeUpdatePolicyOrder(*policy.Order)
		if err != nil {
			return nil, fmt.Errorf("%s.order: %s", name, strings.TrimPrefix(err.Error(), "update_config.order: "))
		}
		out.Order = value
	}
	if isZeroComposeUpdateConfig(out) {
		return nil, nil
	}
	return out, nil
}

func isZeroUpdateConfig(cfg *dockerapi.UpdateConfig) bool {
	return cfg.Parallelism == 0 &&
		cfg.Delay == 0 &&
		cfg.FailureAction == "" &&
		cfg.Monitor == 0 &&
		cfg.MaxFailureRatio == 0 &&
		cfg.Order == ""
}

func isZeroComposeUpdateConfig(cfg *composeUpdateConfig) bool {
	return cfg.Parallelism == nil &&
		cfg.Delay == nil &&
		cfg.FailureAction == "" &&
		cfg.Monitor == nil &&
		cfg.MaxFailureRatio == nil &&
		cfg.Order == ""
}

func cloneUpdateConfig(cfg *dockerapi.UpdateConfig) *dockerapi.UpdateConfig {
	if cfg == nil {
		return nil
	}
	return new(*cfg)
}

func updateConfigsEqual(left, right *dockerapi.UpdateConfig) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	return left.Parallelism == right.Parallelism &&
		left.Delay == right.Delay &&
		left.FailureAction == right.FailureAction &&
		left.Monitor == right.Monitor &&
		left.MaxFailureRatio == right.MaxFailureRatio &&
		left.Order == right.Order
}

func formatUpdateConfig(cfg *dockerapi.UpdateConfig) string {
	if cfg == nil {
		return "{}"
	}
	parts := make([]string, 0, 6)
	if cfg.Parallelism != 0 {
		parts = append(parts, fmt.Sprintf("parallelism=%d", cfg.Parallelism))
	}
	if cfg.Delay != 0 {
		parts = append(parts, "delay="+cfg.Delay.String())
	}
	if cfg.FailureAction != "" {
		parts = append(parts, "failure_action="+cfg.FailureAction)
	}
	if cfg.Monitor != 0 {
		parts = append(parts, "monitor="+cfg.Monitor.String())
	}
	if cfg.MaxFailureRatio != 0 {
		parts = append(parts, fmt.Sprintf("max_failure_ratio=%g", cfg.MaxFailureRatio))
	}
	if cfg.Order != "" {
		parts = append(parts, "order="+cfg.Order)
	}
	if len(parts) == 0 {
		return "{}"
	}
	return "{" + strings.Join(parts, ", ") + "}"
}
