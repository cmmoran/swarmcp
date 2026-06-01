package apply

import (
	"os"
	"time"

	"go.yaml.in/yaml/v4"
)

const PlanFileAPIVersion = "swarmcp.plan.v1"

type PlanFile struct {
	APIVersion    string `yaml:"api_version"`
	GeneratedAt   string `yaml:"generated_at"`
	ToolVersion   string `yaml:"tool_version,omitempty"`
	Project       string `yaml:"project"`
	Deployment    string `yaml:"deployment,omitempty"`
	Partition     string `yaml:"partition,omitempty"`
	Stack         string `yaml:"stack,omitempty"`
	Context       string `yaml:"context,omitempty"`
	PruneServices bool   `yaml:"prune_services,omitempty"`
	Plan          Plan   `yaml:"plan"`
}

func NewPlanFile(toolVersion string, project string, deployment string, partition string, stack string, contextName string, pruneServices bool, plan Plan) PlanFile {
	return PlanFile{
		APIVersion:    PlanFileAPIVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		ToolVersion:   toolVersion,
		Project:       project,
		Deployment:    deployment,
		Partition:     partition,
		Stack:         stack,
		Context:       contextName,
		PruneServices: pruneServices,
		Plan:          plan,
	}
}

func WritePlanFile(path string, plan PlanFile) error {
	if plan.APIVersion == "" {
		plan.APIVersion = PlanFileAPIVersion
	}
	if plan.GeneratedAt == "" {
		plan.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}
	data, err := yaml.Marshal(plan)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func ReadPlanFile(path string) (PlanFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PlanFile{}, err
	}
	var plan PlanFile
	if err := yaml.Unmarshal(data, &plan); err != nil {
		return PlanFile{}, err
	}
	return plan, nil
}
