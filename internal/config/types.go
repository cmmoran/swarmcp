package config

type Config struct {
	Project  Project          `yaml:"project"`
	Stacks   map[string]Stack `yaml:"stacks"`
	Overlays Overlays         `yaml:"overlays"`
	BaseDir  string           `yaml:"-"`
	CacheDir string           `yaml:"-"`
	Offline  bool             `yaml:"-"`
	Debug    bool             `yaml:"-"`
}

type Project struct {
	Name                    string               `yaml:"name"`
	Partitions              []string             `yaml:"partitions"`
	Deployments             []string             `yaml:"deployments"`
	Deployment              string               `yaml:"deployment"`
	Contexts                map[string]string    `yaml:"contexts"`
	Targets                 DeploymentTargets    `yaml:"deployment_targets"`
	Defaults                ProjectDefaults      `yaml:"defaults"`
	PreserveUnusedResources *int                 `yaml:"preserve_unused_resources"`
	Nodes                   map[string]Node      `yaml:"nodes"`
	Sources                 Sources              `yaml:"sources"`
	Configs                 map[string]ConfigDef `yaml:"configs"`
	Secrets                 map[string]SecretDef `yaml:"secrets"`
	SecretsEngine           *SecretsEngine       `yaml:"secrets_engine"`
}

type ProjectDefaults struct {
	Networks NetworkDefaults `yaml:"networks"`
	Volumes  VolumeDefaults  `yaml:"volumes"`
}

type NetworkDefaults struct {
	Internal   string   `yaml:"internal"`
	Egress     string   `yaml:"egress"`
	Attachable bool     `yaml:"attachable"`
	Shared     []string `yaml:"shared"`
}

type VolumeDefaults struct {
	Driver          string                   `yaml:"driver"`
	BasePath        string                   `yaml:"base_path"`
	Layout          string                   `yaml:"layout"`
	NodeLabelKey    string                   `yaml:"node_label_key"`
	ServiceStandard string                   `yaml:"service_standard"`
	ServiceTarget   string                   `yaml:"service_target"`
	Standards       map[string]StandardMount `yaml:"standards"`
}

type Node struct {
	Roles    []string          `yaml:"roles"`
	Labels   map[string]string `yaml:"labels"`
	Volumes  []string          `yaml:"volumes"`
	Platform NodePlatform      `yaml:"platform"`
}

type DeploymentTargets map[string]DeploymentTarget

type DeploymentTarget struct {
	Include   NodeSelector        `yaml:"include"`
	Exclude   NodeSelector        `yaml:"exclude"`
	Overrides map[string]NodeSpec `yaml:"overrides"`
}

type NodeSelector struct {
	Names  []string          `yaml:"names"`
	Labels map[string]string `yaml:"labels"`
}

type NodeSpec struct {
	Roles    []string          `yaml:"roles"`
	Labels   map[string]string `yaml:"labels"`
	Volumes  []string          `yaml:"volumes"`
	Platform NodePlatform      `yaml:"platform"`
}

type NodePlatform struct {
	OS   string `yaml:"os"`
	Arch string `yaml:"arch"`
}

type Stack struct {
	Source     *SourceRef                `yaml:"source"`
	Overrides  map[string]any            `yaml:"overrides"`
	Mode       string                    `yaml:"mode"`
	Partitions map[string]StackPartition `yaml:"partitions"`
	Sources    Sources                   `yaml:"sources"`
	Configs    ConfigDefsOrRefs          `yaml:"configs"`
	Secrets    SecretDefsOrRefs          `yaml:"secrets"`
	Volumes    map[string]VolumeDef      `yaml:"volumes"`
	Services   map[string]Service        `yaml:"services"`
	BaseDir    string                    `yaml:"-"`
}

type StackPartition struct {
	Sources Sources          `yaml:"sources"`
	Configs ConfigDefsOrRefs `yaml:"configs"`
	Secrets SecretDefsOrRefs `yaml:"secrets"`
}

type Service struct {
	Source           *SourceRef               `yaml:"source"`
	Overrides        map[string]any           `yaml:"overrides"`
	Image            string                   `yaml:"image"`
	Command          []string                 `yaml:"command"`
	Args             []string                 `yaml:"args"`
	Workdir          string                   `yaml:"workdir"`
	Env              map[string]string        `yaml:"env"`
	Ports            []Port                   `yaml:"ports"`
	Mode             string                   `yaml:"mode"`
	Replicas         int                      `yaml:"replicas"`
	Labels           map[string]string        `yaml:"labels"`
	Placement        Placement                `yaml:"placement"`
	Healthcheck      map[string]any           `yaml:"healthcheck"`
	DependsOn        []string                 `yaml:"depends_on"`
	Egress           bool                     `yaml:"egress"`
	Networks         []string                 `yaml:"networks"`
	NetworkEphemeral *ServiceNetworkEphemeral `yaml:"network_ephemeral"`
	Configs          []ConfigRef              `yaml:"configs"`
	Secrets          []SecretRef              `yaml:"secrets"`
	Volumes          []VolumeRef              `yaml:"volumes"`
	Sources          Sources                  `yaml:"sources"`
	BaseDir          string                   `yaml:"-"`
}

type ServiceNetworkEphemeral struct {
	Internal   *bool `yaml:"internal"`
	Attachable *bool `yaml:"attachable"`
}

type Placement struct {
	Constraints []string `yaml:"constraints"`
}

type ConfigRef struct {
	Name   string `yaml:"name"`
	Source string `yaml:"source"`
	Target string `yaml:"target"`
	UID    string `yaml:"uid"`
	GID    string `yaml:"gid"`
	Mode   string `yaml:"mode"`
}

type SecretRef struct {
	Name   string `yaml:"name"`
	Source string `yaml:"source"`
	Target string `yaml:"target"`
	UID    string `yaml:"uid"`
	GID    string `yaml:"gid"`
	Mode   string `yaml:"mode"`
}

type ConfigDef struct {
	Source string `yaml:"source"`
	Target string `yaml:"target"`
	UID    string `yaml:"uid"`
	GID    string `yaml:"gid"`
	Mode   string `yaml:"mode"`
}

type SecretDef struct {
	Source string `yaml:"source"`
	Target string `yaml:"target"`
	UID    string `yaml:"uid"`
	GID    string `yaml:"gid"`
	Mode   string `yaml:"mode"`
}

type ConfigDefsOrRefs struct {
	Defs map[string]ConfigDef
}

type SecretDefsOrRefs struct {
	Defs map[string]SecretDef
}

type VolumeDef struct {
	Target  string `yaml:"target"`
	Subpath string `yaml:"subpath"`
}

type VolumeRef struct {
	Name     string `yaml:"name"`
	Standard string `yaml:"standard"`
	Source   string `yaml:"source"`
	Target   string `yaml:"target"`
	Subpath  string `yaml:"subpath"`
	ReadOnly bool   `yaml:"readonly"`
	Category string `yaml:"category"`
}

type StandardMount struct {
	Source   string              `yaml:"source"`
	Target   string              `yaml:"target"`
	ReadOnly bool                `yaml:"readonly"`
	Requires StandardMountPolicy `yaml:"requires"`
}

type StandardMountPolicy struct {
	Roles []string `yaml:"roles"`
}

type Port struct {
	Target    int    `yaml:"target"`
	Published int    `yaml:"published"`
	Protocol  string `yaml:"protocol"`
	Mode      string `yaml:"mode"`
}

type Sources struct {
	URL  string `yaml:"url"`
	Ref  string `yaml:"ref"`
	Path string `yaml:"path"`
	Base string `yaml:"-"`
}

type SourceRef struct {
	URL           string `yaml:"url"`
	Ref           string `yaml:"ref"`
	Path          string `yaml:"path"`
	OverridesPath string `yaml:"overrides_path"`
}

type SecretsEngine struct {
	Provider string     `yaml:"provider"`
	Addr     string     `yaml:"addr"`
	Auth     AuthConfig `yaml:"auth"`
	Vault    *VaultKV   `yaml:"vault"`
}

type AuthConfig struct {
	Method   string `yaml:"method"`
	Path     string `yaml:"path"`
	Role     string `yaml:"role"`
	Audience string `yaml:"audience"`
}

type VaultKV struct {
	Mount        string `yaml:"mount"`
	PathTemplate string `yaml:"path_template"`
}
