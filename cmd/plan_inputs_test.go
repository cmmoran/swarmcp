package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildPlanInputsIncludesInferredValues(t *testing.T) {
	dir := t.TempDir()
	project := filepath.Join(dir, "project.yaml")
	release := filepath.Join(dir, "release.yaml")
	valuesDir := filepath.Join(dir, "values")
	values := filepath.Join(valuesDir, "values.yaml")
	if err := os.MkdirAll(valuesDir, 0o755); err != nil {
		t.Fatalf("mkdir values: %v", err)
	}
	for path, content := range map[string]string{
		project: "project: {}\n",
		release: "stacks: {}\n",
		values:  "global: {}\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	inputs, err := buildPlanInputs(project, []string{project}, []string{release}, nil)
	if err != nil {
		t.Fatalf("buildPlanInputs: %v", err)
	}
	seen := map[string]bool{}
	for _, input := range inputs {
		seen[input.Kind+":"+input.Path] = true
	}
	for _, want := range []string{
		"project:" + project,
		"release:" + release,
		"values:" + values,
	} {
		if !seen[want] {
			t.Fatalf("expected input %q in %#v", want, inputs)
		}
	}
}
