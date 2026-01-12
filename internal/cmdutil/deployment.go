package cmdutil

import (
	"sort"

	"github.com/cmmoran/swarmcp/internal/config"
)

func ResolveDeploymentNodes(cfg *config.Config) []string {
	deployment := cfg.Project.Deployment
	if deployment == "" {
		return nil
	}
	target, ok := cfg.Project.Targets[deployment]
	if !ok {
		return nil
	}
	selected := selectDeploymentNodes(cfg.Project.Nodes, target)
	if len(selected) == 0 {
		return nil
	}
	out := make([]string, 0, len(selected))
	for name := range selected {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func matchesNodeSelector(name string, node config.Node, selector config.NodeSelector) bool {
	if len(selector.Names) == 0 && len(selector.Labels) == 0 {
		return true
	}
	for _, match := range selector.Names {
		if name == match {
			return true
		}
	}
	labelMatch := len(selector.Labels) > 0
	for key, value := range selector.Labels {
		if nodeValue, ok := node.Labels[key]; !ok || nodeValue != value {
			labelMatch = false
			break
		}
	}
	return labelMatch
}

func selectDeploymentNodes(nodes map[string]config.Node, target config.DeploymentTarget) map[string]config.NodeSpec {
	selected := make(map[string]config.NodeSpec)
	for name, node := range nodes {
		if !matchesNodeSelector(name, node, target.Include) {
			continue
		}
		selected[name] = config.NodeSpec{
			Roles:   append([]string(nil), node.Roles...),
			Labels:  copyNodeLabels(node.Labels),
			Volumes: append([]string(nil), node.Volumes...),
			Platform: config.NodePlatform{
				OS:   node.Platform.OS,
				Arch: node.Platform.Arch,
			},
		}
	}
	for name, node := range nodes {
		if _, ok := selected[name]; !ok {
			continue
		}
		if len(target.Exclude.Names) > 0 || len(target.Exclude.Labels) > 0 {
			if matchesNodeSelector(name, node, target.Exclude) {
				delete(selected, name)
			}
		}
	}
	for name, override := range target.Overrides {
		base, ok := selected[name]
		if !ok {
			continue
		}
		if len(override.Roles) > 0 {
			base.Roles = append([]string(nil), override.Roles...)
		}
		if override.Labels != nil {
			if base.Labels == nil {
				base.Labels = make(map[string]string, len(override.Labels))
			}
			for key, value := range override.Labels {
				base.Labels[key] = value
			}
		}
		if override.Platform.OS != "" {
			base.Platform.OS = override.Platform.OS
		}
		if override.Platform.Arch != "" {
			base.Platform.Arch = override.Platform.Arch
		}
		if len(override.Volumes) > 0 {
			base.Volumes = append([]string(nil), override.Volumes...)
		}
		selected[name] = base
	}
	return selected
}

func copyNodeLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	out := make(map[string]string, len(labels))
	for key, value := range labels {
		out[key] = value
	}
	return out
}
