package cmd

import (
	"github.com/cmmoran/swarmcp/internal/apply"
	"github.com/cmmoran/swarmcp/internal/cmdutil"
	"github.com/cmmoran/swarmcp/internal/config"
)

func buildPlanInputs(cfg *config.Config, configPath string, configPaths []string, releaseConfigPaths []string, valuesFiles []string, secretsFile string) ([]apply.PlanInput, error) {
	var inputs []apply.PlanInput
	configInputs, err := apply.FileInputs("project", configPaths)
	if err != nil {
		return nil, err
	}
	inputs = append(inputs, configInputs...)
	releaseInputs, err := apply.FileInputs("release", releaseConfigPaths)
	if err != nil {
		return nil, err
	}
	inputs = append(inputs, releaseInputs...)
	valuesInputs, err := apply.FileInputs("values", cmdutil.InferValuesFiles(configPath, valuesFiles))
	if err != nil {
		return nil, err
	}
	inputs = append(inputs, valuesInputs...)
	secretInputs, err := apply.FileInputs("secrets", []string{cmdutil.InferSecretsFile(cfg, configPath, secretsFile)})
	if err != nil {
		return nil, err
	}
	inputs = append(inputs, secretInputs...)
	return inputs, nil
}
