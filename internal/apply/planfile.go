package apply

import (
	"os"
	"time"

	"go.yaml.in/yaml/v4"
)

const PlanFileAPIVersion = "swarmcp.plan.v1"

type PlanFile struct {
	APIVersion    string             `yaml:"api_version"`
	GeneratedAt   string             `yaml:"generated_at"`
	ToolVersion   string             `yaml:"tool_version,omitempty"`
	Project       string             `yaml:"project"`
	Deployment    string             `yaml:"deployment,omitempty"`
	Partition     string             `yaml:"partition,omitempty"`
	Stack         string             `yaml:"stack,omitempty"`
	Context       string             `yaml:"context,omitempty"`
	PruneServices bool               `yaml:"prune_services,omitempty"`
	SecretSources []PlanSecretSource `yaml:"secret_sources,omitempty"`
	Plan          Plan               `yaml:"plan"`
}

type PlanSecretSource struct {
	SecretName   string                 `yaml:"secret_name"`
	LogicalName  string                 `yaml:"logical_name"`
	Scope        PlanScope              `yaml:"scope"`
	Dependencies []PlanSecretDependency `yaml:"dependencies"`
}

type PlanSecretDependency struct {
	Name     string    `yaml:"name"`
	Scope    PlanScope `yaml:"scope"`
	Hash     string    `yaml:"hash"`
	Provider string    `yaml:"provider,omitempty"`
	Mount    string    `yaml:"mount,omitempty"`
	Path     string    `yaml:"path,omitempty"`
	Key      string    `yaml:"key,omitempty"`
	Version  *int      `yaml:"version,omitempty"`
}

type PlanScope struct {
	Project    string `yaml:"project,omitempty"`
	Deployment string `yaml:"deployment,omitempty"`
	Stack      string `yaml:"stack,omitempty"`
	Partition  string `yaml:"partition,omitempty"`
	Service    string `yaml:"service,omitempty"`
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
