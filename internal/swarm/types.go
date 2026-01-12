package swarm

import (
	"time"

	dockerapi "github.com/docker/docker/api/types/swarm"
)

type ConfigSpec struct {
	Name   string
	Labels map[string]string
	Data   []byte
}

type SecretSpec struct {
	Name   string
	Labels map[string]string
	Data   []byte
}

type Config struct {
	ID        string
	Name      string
	Labels    map[string]string
	CreatedAt time.Time
}

type Secret struct {
	ID        string
	Name      string
	Labels    map[string]string
	CreatedAt time.Time
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
	Name       string
	Driver     string
	Attachable bool
	Internal   bool
	Labels     map[string]string
}

type Node struct {
	ID       string
	Name     string
	Hostname string
	Labels   map[string]string
	Spec     dockerapi.NodeSpec
	Version  uint64
}
