package spec

import (
	"sort"
)

type Backend int

const (
	BackendAuto Backend = iota
	BackendBao
	BackendVault
)

type Project struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Spec       ProjSpec `yaml:"spec"`
	Root       string   `yaml:"-"`
}

type Metadata struct {
	Name string `yaml:"name"`
}

type ProjSpec struct {
	Defaults        ProjDefaults        `yaml:"defaults"`
	Vars            map[string]any      `yaml:"vars"`
	SecretsProvider SecretsProviderSpec `yaml:"secretsprovider"`
	Stacks          []StackRef          `yaml:"stacks"`
}

type ProjDefaults struct {
	Networks  map[string]NetworkDef `yaml:"networks"`
	Resources Resources             `yaml:"resources"`
}

type StackRef struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
	Path string `yaml:"path"`
}

type SecretsProviderSpec struct {
	Backend             Backend `yaml:"backend"`
	Addr                string  `yaml:"addr"`
	Namespace           string  `yaml:"namespace"`
	RoleIDPath          string  `yaml:"roleIdPath"`
	WrappedSecretIDPath string  `yaml:"wrappedSecretIdPath"`
}

type Stack struct {
	APIVersion string    `yaml:"apiVersion"`
	Kind       string    `yaml:"kind"`
	Metadata   Metadata  `yaml:"metadata"`
	Spec       StackSpec `yaml:"spec"`
	Dir        string    `yaml:"-"`
}

type StackSpec struct {
	Type      string        `yaml:"type"`
	Instances []InstanceRef `yaml:"instances"`
	Defaults  StackDefaults `yaml:"defaults"`
	Services  []ServiceRef  `yaml:"services"`
}

type InstanceRef struct {
	Name string         `yaml:"name"`
	Vars map[string]any `yaml:"vars"`
}

type StackDefaults struct {
	Networks map[string]NetworkDef `yaml:"networks"`
}

type ServiceRef struct {
	Name string `yaml:"name"`
	Path string `yaml:"path"`
}

type Service struct {
	APIVersion string      `yaml:"apiVersion"`
	Kind       string      `yaml:"kind"`
	Metadata   Metadata    `yaml:"metadata"`
	Spec       ServiceSpec `yaml:"spec"`
	Dir        string      `yaml:"-"`
}

type ServiceSpec struct {
	Image    ImageSpec         `yaml:"image"`
	Deploy   DeploySpec        `yaml:"deploy"`
	Networks []NetAttach       `yaml:"networks"`
	Env      []EnvVar          `yaml:"env"`
	Configs  []ConfigDecl      `yaml:"configs"`
	Secrets  []SecretDecl      `yaml:"secrets"`
	Mounts   []MountDecl       `yaml:"mounts"`
	Labels   map[string]string `yaml:"labels"`
}

type ImageSpec struct {
	Repo string `yaml:"repo"`
	Tag  string `yaml:"tag"`
}

type DeploySpec struct {
	Replicas  int       `yaml:"replicas"`
	Placement Placement `yaml:"placement"`
	Resources Resources `yaml:"resources"`
}

type Placement struct {
	Constraints []string `yaml:"constraints"`
}

type Resources struct {
	Reservations CPUAndMem `yaml:"reservations"`
	Limits       CPUAndMem `yaml:"limits"`
}

type CPUAndMem struct {
	CPUs   string `yaml:"cpus"`
	Memory string `yaml:"memory"`
}

type NetworkDef struct {
	Driver   string `yaml:"driver"`
	Internal bool   `yaml:"internal"`
}

type NetAttach struct {
	Name string `yaml:"name"`
}

type EnvVar struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

// ConfigDecl now supports an optional file target/perms.
type ConfigDecl struct {
	Name     string               `yaml:"name"`
	Template string               `yaml:"template,omitempty"`
	File     *ReferenceFileTarget `yaml:"file,omitempty"` // default "/<name>"
}

// SecretDecl supports either a SecretsProvider source or a template (rendered in-memory),
// and mounts as a file (no env secret targets).
type SecretDecl struct {
	Name      string               `yaml:"name"`
	FromVault string               `yaml:"fromVault,omitempty"` // optional direct SecretsProvider source
	Template  string               `yaml:"template,omitempty"`  // optional secret template (in-memory only)
	File      *ReferenceFileTarget `yaml:"file,omitempty"`      // default "/run/secrets/<name>"
}

// Effective (fully-resolved) items carry their data and final file targets.
// Mode is guaranteed non-nil after normalization at resolve time.

type EffectiveConfig struct {
	Name string
	Data []byte
	File ReferenceFileTarget
}

type EffectiveSecret struct {
	Name string
	Data []byte
	File ReferenceFileTarget // file-only; no env secrets
}

type MountDecl struct {
	Type    string   `yaml:"type"`
	Target  string   `yaml:"target"`
	Options []string `yaml:"options"`
}

type EffectiveProject struct {
	Project *Project
	Stacks  []EffectiveStack
}

type EffectiveStack struct {
	Stack    *Stack
	Instance *InstanceRef
	Services []EffectiveService
}

type EffectiveService struct {
	Service *Service
	Name    string

	// Cohesive, ordered items (bytes + file target together)
	Configs []EffectiveConfig
	Secrets []EffectiveSecret

	// Non-secret environment (from the spec only; no env secrets)
	Env map[string]string

	// Swarm networks
	Networks []string
}

func (x EffectiveService) EnvDecl() []string {
	out := make([]string, 0, len(x.Env))
	for k, v := range x.Env {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)

	return out
}

// ReferenceFileTarget decorates Docker's file target (maps to Swarm File.Name/UID/GID/Mode).
type ReferenceFileTarget struct {
	Target string  `yaml:"target,omitempty"` // in-container path (→ Swarm File.Name)
	UID    string  `yaml:"uid,omitempty"`    // default "0"
	GID    string  `yaml:"gid,omitempty"`    // default "0"
	Mode   *uint32 `yaml:"mode,omitempty"`   // default 0440 when nil
}

const (
	defaultSecretMode = 0400
	defaultConfigMode = 0444
	defaultUID        = "0"
	defaultGID        = "0"
)

func ResolveFileTarget(name string, in *ReferenceFileTarget, isSecret bool) ReferenceFileTarget {
	out := ReferenceFileTarget{
		UID:    defaultUID,
		GID:    defaultGID,
		Mode:   defaultMode(isSecret),
		Target: defaultTarget(isSecret, name),
	}
	if in == nil {
		return out
	}
	out.Target = ifThen(in.Target != "", in.Target, out.Target)
	out.UID = ifThen(in.UID != "", in.UID, out.UID)
	out.GID = ifThen(in.GID != "", in.GID, out.GID)
	out.Mode = ifThen(in.Mode != nil, in.Mode, out.Mode)
	return out
}

func defaultMode(isSecret bool) *uint32 {
	sdefMode := uint32(defaultSecretMode)
	cdefMode := uint32(defaultConfigMode)
	return ifThen(isSecret, &sdefMode, &cdefMode)
}

func defaultTarget(isSecret bool, name string) string {
	sdefTarget := "/run/secrets/" + name
	cdefTarget := "/" + name
	return ifThen(isSecret, sdefTarget, cdefTarget)
}

func ifThen[T any](cond bool, t, f T) T {
	if cond {
		return t
	}
	return f
}
