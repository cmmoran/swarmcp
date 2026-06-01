package swarm

import (
	"time"

	dockerapi "github.com/docker/docker/api/types/swarm"
)

type ConfigSpec struct {
	Name   string            `yaml:"name" json:"name"`
	Labels map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	Data   []byte            `yaml:"data,omitempty" json:"data,omitempty"`
}

type SecretSpec struct {
	Name   string            `yaml:"name" json:"name"`
	Labels map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	Data   []byte            `yaml:"data,omitempty" json:"data,omitempty"`
}

type Config struct {
	ID        string            `yaml:"id" json:"id"`
	Name      string            `yaml:"name" json:"name"`
	Labels    map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	CreatedAt time.Time         `yaml:"created_at,omitempty" json:"created_at,omitempty"`
}

type Secret struct {
	ID        string            `yaml:"id" json:"id"`
	Name      string            `yaml:"name" json:"name"`
	Labels    map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	CreatedAt time.Time         `yaml:"created_at,omitempty" json:"created_at,omitempty"`
}

type Service struct {
	ID      string
	Name    string
	Labels  map[string]string
	Spec    dockerapi.ServiceSpec
	Version uint64
	Status  *dockerapi.ServiceStatus
}

type Network struct {
	ID      string
	Name    string
	Driver  string
	Scope   string
	Subnets []string
}

type NetworkSpec struct {
	Name       string            `yaml:"name" json:"name"`
	Driver     string            `yaml:"driver,omitempty" json:"driver,omitempty"`
	Attachable bool              `yaml:"attachable,omitempty" json:"attachable,omitempty"`
	Internal   bool              `yaml:"internal,omitempty" json:"internal,omitempty"`
	Labels     map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
}

type Node struct {
	ID       string
	Name     string
	Hostname string
	Labels   map[string]string
	Spec     dockerapi.NodeSpec
	Version  uint64
}
