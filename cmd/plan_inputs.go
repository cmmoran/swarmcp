package cmd

import (
	"github.com/cmmoran/swarmcp/internal/apply"
	"github.com/cmmoran/swarmcp/internal/cmdutil"
)

func buildPlanInputs(configPath string, configPaths []string, releaseConfigPaths []string, valuesFiles []string) ([]apply.PlanInput, error) {
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
	return inputs, nil
}
