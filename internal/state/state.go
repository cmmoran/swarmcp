package state

import (
	"encoding/json"
	"os"
)

const CurrentVersion = 1

type State struct {
	Version     int         `json:"version"`
	GeneratedAt string      `json:"generated_at"`
	Command     string      `json:"command"`
	ConfigPath  string      `json:"config_path"`
	Project     string      `json:"project"`
	Deployment  string      `json:"deployment,omitempty"`
	Partition   string      `json:"partition,omitempty"`
	Stack       string      `json:"stack,omitempty"`
	Plan        PlanSummary `json:"plan"`
}

type PlanSummary struct {
	NetworksCreated int      `json:"networks_created"`
	ConfigsCreated  int      `json:"configs_created"`
	SecretsCreated  int      `json:"secrets_created"`
	StacksDeployed  int      `json:"stacks_deployed"`
	StackNames      []string `json:"stack_names,omitempty"`
	ConfigsRemoved  int      `json:"configs_removed"`
	SecretsRemoved  int      `json:"secrets_removed"`
	ConfigsSkipped  int      `json:"configs_skipped"`
	SecretsSkipped  int      `json:"secrets_skipped"`
	ServicesCreated int      `json:"services_created"`
	ServicesUpdated int      `json:"services_updated"`
}

func Write(path string, state State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func Read(path string) (State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, err
	}
	return state, nil
}
