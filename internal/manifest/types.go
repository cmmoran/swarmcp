package manifest

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
	Defaults ProjDefaults   `yaml:"defaults"`
	Vars     map[string]any `yaml:"vars"`
	Vault    VaultSpec      `yaml:"vault"`
	Stacks   []StackRef     `yaml:"stacks"`
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

type VaultSpec struct {
	Addr                string `yaml:"addr"`
	RoleIDPath          string `yaml:"roleIdPath"`
	WrappedSecretIDPath string `yaml:"wrappedSecretIdPath"`
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

type ConfigDecl struct {
	Name     string `yaml:"name"`
	Template string `yaml:"template"`
	Mode     uint32 `yaml:"mode"`
	Target   string `yaml:"target"`
}

type SecretDecl struct {
	Name      string `yaml:"name"`
	FromVault string `yaml:"fromVault"`
	Target    string `yaml:"target"`
	Mode      uint32 `yaml:"mode"`
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
	Service         *Service
	Name            string
	RenderedConfigs map[string][]byte
	ResolvedSecrets map[string][]byte
	EffectiveEnv    map[string]string
	EffectiveNets   []string
}
