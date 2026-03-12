package config

import (
	"fmt"

	"go.yaml.in/yaml/v4"
)

type secretDefInput struct {
	Name   string `yaml:"name"`
	Source string `yaml:"source"`
	Target string `yaml:"target"`
	UID    string `yaml:"uid"`
	GID    string `yaml:"gid"`
	Mode   string `yaml:"mode"`
}

func (s *SecretDefsOrRefs) UnmarshalYAML(value *yaml.Node) error {
	if value == nil {
		return nil
	}
	if value.Kind == yaml.ScalarNode && value.Tag == "!!null" {
		return nil
	}
	switch value.Kind {
	case yaml.MappingNode:
		var defs map[string]SecretDef
		if err := value.Decode(&defs); err != nil {
			return err
		}
		s.Defs = defs
		return nil
	case yaml.SequenceNode:
		out := make(map[string]SecretDef, len(value.Content))
		for _, item := range value.Content {
			switch item.Kind {
			case yaml.ScalarNode:
				name := item.Value
				if name == "" {
					return fmt.Errorf("secret name is required")
				}
				if _, exists := out[name]; exists {
					return fmt.Errorf("duplicate secret name %q", name)
				}
				out[name] = SecretDef{}
			case yaml.MappingNode:
				var ref secretDefInput
				if err := item.Decode(&ref); err != nil {
					return err
				}
				if ref.Name == "" {
					return fmt.Errorf("secret name is required")
				}
				if _, exists := out[ref.Name]; exists {
					return fmt.Errorf("duplicate secret name %q", ref.Name)
				}
				out[ref.Name] = SecretDef{
					Source: ref.Source,
					Target: ref.Target,
					UID:    ref.UID,
					GID:    ref.GID,
					Mode:   ref.Mode,
				}
			default:
				return fmt.Errorf("invalid secrets entry")
			}
		}
		s.Defs = out
		return nil
	default:
		return fmt.Errorf("invalid secrets definition")
	}
}
