package cmdutil

import (
	"sort"
	"strings"

	"github.com/cmmoran/swarmcp/internal/config"
)

func ResolveDeploymentNodeSpecs(cfg *config.Config) map[string]config.NodeSpec {
	if cfg == nil {
		return nil
	}
	deployment := cfg.Project.Deployment
	if deployment != "" {
		if target, ok := cfg.Project.Targets[deployment]; ok {
			return selectDeploymentNodes(cfg.Project.Nodes, target)
		}
	}
	if len(cfg.Project.Nodes) == 0 {
		return nil
	}
	out := make(map[string]config.NodeSpec, len(cfg.Project.Nodes))
	for name, node := range cfg.Project.Nodes {
		out[name] = config.NodeSpec{
			Roles:   append([]string(nil), node.Roles...),
			Labels:  copyNodeLabels(node.Labels),
			Volumes: append([]string(nil), node.Volumes...),
			Platform: config.NodePlatform{
				OS:   node.Platform.OS,
				Arch: node.Platform.Arch,
			},
		}
	}
	return out
}

func NodesForConstraints(nodes map[string]config.NodeSpec, constraints []string) ([]string, bool) {
	if len(nodes) == 0 {
		return nil, false
	}
	if len(constraints) == 0 {
		return sortedNodeNames(nodes), false
	}
	var evaluators []func(config.NodeSpec) bool
	for _, raw := range constraints {
		eval, ok := parseConstraint(raw)
		if !ok {
			return nil, true
		}
		evaluators = append(evaluators, eval)
	}
	var matches []string
	for name, node := range nodes {
		if nodeMatchesAll(node, evaluators) {
			matches = append(matches, name)
		}
	}
	sortStrings(matches)
	return matches, false
}

func parseConstraint(raw string) (func(config.NodeSpec) bool, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, false
	}
	if strings.HasPrefix(value, "node.role") {
		_, role, ok := splitConstraint(value, "node.role")
		if !ok {
			return nil, false
		}
		return func(node config.NodeSpec) bool {
			return nodeHasRole(node, role)
		}, true
	}
	if strings.HasPrefix(value, "node.labels.") {
		key, val, ok := splitConstraint(value, "node.labels.")
		if !ok {
			return nil, false
		}
		return func(node config.NodeSpec) bool {
			if node.Labels == nil {
				return false
			}
			return node.Labels[key] == val
		}, true
	}
	if strings.HasPrefix(value, "node.platform.os") {
		_, osValue, ok := splitConstraint(value, "node.platform.os")
		if !ok {
			return nil, false
		}
		osValue = strings.Trim(strings.TrimSpace(osValue), "\"")
		return func(node config.NodeSpec) bool {
			return node.Platform.OS == osValue
		}, true
	}
	if strings.HasPrefix(value, "node.platform.arch") {
		_, archValue, ok := splitConstraint(value, "node.platform.arch")
		if !ok {
			return nil, false
		}
		archValue = strings.Trim(strings.TrimSpace(archValue), "\"")
		return func(node config.NodeSpec) bool {
			return node.Platform.Arch == archValue
		}, true
	}
	return nil, false
}

func splitConstraint(value string, prefix string) (string, string, bool) {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, prefix) {
		return "", "", false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
	parts := strings.SplitN(rest, "==", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	left := strings.TrimSpace(parts[0])
	right := strings.TrimSpace(parts[1])
	if right == "" {
		return "", "", false
	}
	return left, right, true
}

func nodeHasRole(node config.NodeSpec, role string) bool {
	role = strings.TrimSpace(role)
	if role == "" {
		return false
	}
	if len(node.Roles) == 0 {
		return role == "worker"
	}
	for _, item := range node.Roles {
		if item == role {
			return true
		}
	}
	return false
}

func nodeMatchesAll(node config.NodeSpec, evals []func(config.NodeSpec) bool) bool {
	for _, eval := range evals {
		if !eval(node) {
			return false
		}
	}
	return true
}

func sortedNodeNames(nodes map[string]config.NodeSpec) []string {
	if len(nodes) == 0 {
		return nil
	}
	out := make([]string, 0, len(nodes))
	for name := range nodes {
		out = append(out, name)
	}
	sortStrings(out)
	return out
}

func sortStrings(values []string) {
	if len(values) < 2 {
		return
	}
	sort.Strings(values)
}
