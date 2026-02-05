package apply

import (
	"github.com/cmmoran/swarmcp/internal/render"
)

func isManagedProject(labels map[string]string, projectName string) bool {
	if labels == nil {
		return false
	}
	if labels[render.LabelManaged] != "true" {
		return false
	}
	return labels[render.LabelProject] == projectName
}

func configLabelDrift(expected, actual map[string]string, projectName string) string {
	if len(actual) == 0 {
		return "labels missing"
	}
	if !isManagedProject(actual, projectName) {
		return "unmanaged resource with matching name"
	}
	if expected == nil {
		return ""
	}
	if expected[render.LabelName] != "" && actual[render.LabelName] != expected[render.LabelName] {
		return "logical name label mismatch"
	}
	if expected[render.LabelProject] != "" && actual[render.LabelProject] != expected[render.LabelProject] {
		return "project label mismatch"
	}
	if expected[render.LabelHash] != "" && actual[render.LabelHash] != expected[render.LabelHash] {
		return "hash label mismatch"
	}
	return ""
}
