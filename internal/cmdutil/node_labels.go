package cmdutil

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/swarm"
)

func DesiredNodeLabels(cfg *config.Config, node config.NodeSpec) map[string]string {
	desired := copyNodeLabels(node.Labels)
	labelKey := strings.TrimSpace(cfg.Project.Defaults.Volumes.NodeLabelKey)
	if labelKey == "" {
		return desired
	}
	if desired == nil {
		desired = make(map[string]string, len(node.Volumes))
	}
	for _, volume := range node.Volumes {
		desired[labelKey+"."+volume] = "true"
	}
	return desired
}

func NodeLabelWarnings(cfg *config.Config, nodes map[string]config.NodeSpec, swarmNodes []swarm.Node) []string {
	if len(nodes) == 0 {
		return nil
	}
	index := make(map[string]swarm.Node, len(swarmNodes)*2)
	for _, node := range swarmNodes {
		if node.Name != "" {
			index[node.Name] = node
		}
		if node.Hostname != "" {
			index[node.Hostname] = node
		}
	}
	var warnings []string
	for name, node := range nodes {
		desired := DesiredNodeLabels(cfg, node)
		if len(desired) == 0 {
			continue
		}
		actual, ok := index[name]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("label check: node %q not found in swarm (run: swarmcp bootstrap labels)", name))
			continue
		}
		for key, value := range desired {
			if actual.Labels == nil {
				warnings = append(warnings, fmt.Sprintf("label check: node %q missing label %s=%s (run: swarmcp bootstrap labels)", name, key, value))
				continue
			}
			if actualValue, ok := actual.Labels[key]; !ok {
				warnings = append(warnings, fmt.Sprintf("label check: node %q missing label %s=%s (run: swarmcp bootstrap labels)", name, key, value))
			} else if actualValue != value {
				warnings = append(warnings, fmt.Sprintf("label check: node %q label %s=%s (found %s) (run: swarmcp bootstrap labels)", name, key, value, actualValue))
			}
		}
	}
	sort.Strings(warnings)
	return warnings
}
