package cmdutil

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/cmmoran/swarmcp/internal/config"
	"gopkg.in/yaml.v3"
)

type AutoLabelWriteOptions struct {
	ConfigPath      string
	Config          *config.Config
	PartitionFilter string
	Prune           bool
}

type AutoLabelWriteResult struct {
	Changed    bool
	Added      int
	Updated    int
	Pruned     int
	Notes      []string
	Skipped    bool
	SkipReason string
}

func WriteAutoNodeLabels(opts AutoLabelWriteOptions) (AutoLabelWriteResult, error) {
	var result AutoLabelWriteResult
	if opts.Config == nil || opts.ConfigPath == "" {
		result.Skipped = true
		result.SkipReason = "config not loaded"
		return result, nil
	}
	labelKey := strings.TrimSpace(opts.Config.Project.Defaults.Volumes.NodeLabelKey)
	if labelKey == "" {
		result.Skipped = true
		result.SkipReason = "project.defaults.volumes.node_label_key not set"
		return result, nil
	}

	data, err := os.ReadFile(opts.ConfigPath)
	if err != nil {
		return result, err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return result, err
	}
	if len(doc.Content) == 0 {
		return result, fmt.Errorf("config %q: empty document", opts.ConfigPath)
	}

	required := RequiredVolumes(opts.Config, opts.PartitionFilter)
	execKnown := len(required) > 0

	var notes []string
	for name, node := range opts.Config.Project.Nodes {
		scope := fmt.Sprintf("project.nodes.%s", name)
		notes = append(notes, autoLabelNotes(scope, node.Labels, node.Volumes, required, execKnown, labelKey)...)
	}
	for targetName, target := range opts.Config.Project.Targets {
		for nodeName, override := range target.Overrides {
			base, ok := opts.Config.Project.Nodes[nodeName]
			volumes := override.Volumes
			if len(volumes) == 0 && ok {
				volumes = base.Volumes
			}
			scope := fmt.Sprintf("project.deployment_targets.%s.overrides.%s", targetName, nodeName)
			notes = append(notes, autoLabelNotes(scope, override.Labels, volumes, required, execKnown, labelKey)...)
		}
	}
	sort.Strings(notes)
	result.Notes = notes

	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return result, fmt.Errorf("config %q: root is not a mapping", opts.ConfigPath)
	}

	for name, node := range opts.Config.Project.Nodes {
		desiredVolumes := filterVolumes(node.Volumes, required, opts.Prune)
		desired := autoLabelsForVolumes(labelKey, desiredVolumes)
		nodeMap := ensureMapping(root, "project", "nodes", name)
		if nodeMap == nil {
			continue
		}
		changed, added, updated, pruned := updateLabelsMap(nodeMap, labelKey, desired, opts.Prune)
		result.Added += added
		result.Updated += updated
		result.Pruned += pruned
		if changed {
			result.Changed = true
		}
	}

	for targetName, target := range opts.Config.Project.Targets {
		for nodeName, override := range target.Overrides {
			base, ok := opts.Config.Project.Nodes[nodeName]
			volumes := override.Volumes
			if len(volumes) == 0 && ok {
				volumes = base.Volumes
			}
			desiredVolumes := filterVolumes(volumes, required, opts.Prune)
			desired := autoLabelsForVolumes(labelKey, desiredVolumes)
			nodeMap := ensureMapping(root, "project", "deployment_targets", targetName, "overrides", nodeName)
			if nodeMap == nil {
				continue
			}
			changed, added, updated, pruned := updateLabelsMap(nodeMap, labelKey, desired, opts.Prune)
			result.Added += added
			result.Updated += updated
			result.Pruned += pruned
			if changed {
				result.Changed = true
			}
		}
	}

	if !result.Changed {
		return result, nil
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return result, err
	}
	if err := enc.Close(); err != nil {
		return result, err
	}

	mode := os.FileMode(0o644)
	if info, err := os.Stat(opts.ConfigPath); err == nil {
		mode = info.Mode()
	}
	if err := os.WriteFile(opts.ConfigPath, buf.Bytes(), mode); err != nil {
		return result, err
	}

	return result, nil
}

func autoLabelNotes(scope string, labels map[string]string, volumes []string, required map[string]struct{}, execKnown bool, labelKey string) []string {
	if len(labels) == 0 {
		return nil
	}
	volumeSet := make(map[string]struct{}, len(volumes))
	for _, name := range volumes {
		if name == "" {
			continue
		}
		volumeSet[name] = struct{}{}
	}
	prefix := labelKey + "."
	var notes []string
	for key, value := range labels {
		if !strings.HasPrefix(key, prefix) || value != "true" {
			continue
		}
		volume := strings.TrimPrefix(key, prefix)
		if volume == "" {
			continue
		}
		var reasons []string
		if _, ok := volumeSet[volume]; !ok {
			reasons = append(reasons, "not in node volumes list")
		}
		if execKnown {
			if _, ok := required[volume]; !ok {
				reasons = append(reasons, "not required by current execution")
			}
		}
		if len(reasons) > 0 {
			notes = append(notes, fmt.Sprintf("label note: %s label %s=true no longer relevant (%s)", scope, key, strings.Join(reasons, "; ")))
		}
	}
	return notes
}

func filterVolumes(volumes []string, required map[string]struct{}, prune bool) []string {
	if !prune || len(required) == 0 {
		return volumes
	}
	out := make([]string, 0, len(volumes))
	for _, name := range volumes {
		if name == "" {
			continue
		}
		if _, ok := required[name]; ok {
			out = append(out, name)
		}
	}
	return out
}

func autoLabelsForVolumes(labelKey string, volumes []string) map[string]string {
	if len(volumes) == 0 {
		return nil
	}
	labels := make(map[string]string, len(volumes))
	for _, volume := range volumes {
		volume = strings.TrimSpace(volume)
		if volume == "" {
			continue
		}
		labels[labelKey+"."+volume] = "true"
	}
	if len(labels) == 0 {
		return nil
	}
	return labels
}

func ensureMapping(root *yaml.Node, path ...string) *yaml.Node {
	current := root
	for _, key := range path {
		if current.Kind != yaml.MappingNode {
			return nil
		}
		child := mappingValue(current, key)
		if child == nil {
			keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
			child = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			current.Content = append(current.Content, keyNode, child)
		}
		current = child
	}
	return current
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func updateLabelsMap(nodeMap *yaml.Node, labelKey string, desired map[string]string, prune bool) (bool, int, int, int) {
	var added, updated, pruned int
	labelsNode := mappingValue(nodeMap, "labels")
	if labelsNode == nil {
		if len(desired) == 0 {
			return false, 0, 0, 0
		}
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "labels"}
		labelsNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		nodeMap.Content = append(nodeMap.Content, keyNode, labelsNode)
	}

	existing := make(map[string]*yaml.Node, len(labelsNode.Content)/2)
	for i := 0; i+1 < len(labelsNode.Content); i += 2 {
		keyNode := labelsNode.Content[i]
		valNode := labelsNode.Content[i+1]
		existing[keyNode.Value] = valNode
	}

	if len(desired) > 0 {
		keys := make([]string, 0, len(desired))
		for key := range desired {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			value := desired[key]
			if node, ok := existing[key]; ok {
				if node.Value != value {
					node.Value = value
					updated++
				}
				continue
			}
			labelsNode.Content = append(labelsNode.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
			)
			added++
		}
	}

	if prune && labelsNode != nil {
		prefix := labelKey + "."
		out := labelsNode.Content[:0]
		for i := 0; i+1 < len(labelsNode.Content); i += 2 {
			keyNode := labelsNode.Content[i]
			valNode := labelsNode.Content[i+1]
			if strings.HasPrefix(keyNode.Value, prefix) {
				if _, ok := desired[keyNode.Value]; !ok {
					pruned++
					continue
				}
			}
			out = append(out, keyNode, valNode)
		}
		labelsNode.Content = out
	}

	if labelsNode != nil && len(labelsNode.Content) == 0 {
		deleteMappingKey(nodeMap, "labels")
	}

	changed := added > 0 || updated > 0 || pruned > 0
	return changed, added, updated, pruned
}

func deleteMappingKey(node *yaml.Node, key string) bool {
	if node == nil || node.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			node.Content = append(node.Content[:i], node.Content[i+2:]...)
			return true
		}
	}
	return false
}
