package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

type configDefInput struct {
	Name   string `yaml:"name"`
	Source string `yaml:"source"`
	Target string `yaml:"target"`
	UID    string `yaml:"uid"`
	GID    string `yaml:"gid"`
	Mode   string `yaml:"mode"`
}

func (c *ConfigDefsOrRefs) UnmarshalYAML(value *yaml.Node) error {
	if value == nil {
		return nil
	}
	if value.Kind == yaml.ScalarNode && value.Tag == "!!null" {
		return nil
	}
	switch value.Kind {
	case yaml.MappingNode:
		var defs map[string]ConfigDef
		if err := value.Decode(&defs); err != nil {
			return err
		}
		c.Defs = defs
		return nil
	case yaml.SequenceNode:
		out := make(map[string]ConfigDef, len(value.Content))
		for _, item := range value.Content {
			switch item.Kind {
			case yaml.ScalarNode:
				name := item.Value
				if name == "" {
					return fmt.Errorf("config name is required")
				}
				if _, exists := out[name]; exists {
					return fmt.Errorf("duplicate config name %q", name)
				}
				out[name] = ConfigDef{}
			case yaml.MappingNode:
				var ref configDefInput
				if err := item.Decode(&ref); err != nil {
					return err
				}
				if ref.Name == "" {
					return fmt.Errorf("config name is required")
				}
				if _, exists := out[ref.Name]; exists {
					return fmt.Errorf("duplicate config name %q", ref.Name)
				}
				out[ref.Name] = ConfigDef{
					Source: ref.Source,
					Target: ref.Target,
					UID:    ref.UID,
					GID:    ref.GID,
					Mode:   ref.Mode,
				}
			default:
				return fmt.Errorf("invalid configs entry")
			}
		}
		c.Defs = out
		return nil
	default:
		return fmt.Errorf("invalid configs definition")
	}
}
