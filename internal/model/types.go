package model

// FileTarget matches the Docker Swarm file target shape used for configs and
// secrets while allowing callers to retain templating metadata.
type FileTarget struct {
	Target string
	UID    string
	GID    string
	Mode   *uint32
	Source string // optional source path for traceability
}

// ConfigSpec represents a Swarm config with optional templating metadata.
type ConfigSpec struct {
	Name     string
	Labels   map[string]string
	Template string
	Target   FileTarget
	Source   string
}

// SecretSpec represents a Swarm secret with optional templating metadata.
type SecretSpec struct {
	Name     string
	Labels   map[string]string
	Template string
	Target   FileTarget
	Source   string
}

// CPUMem captures string-based CPU/memory declarations before conversion into
// Docker-native units.
type CPUMem struct {
	CPUs   string
	Memory string
}

// Resources mirrors Swarm resource specs while keeping raw string inputs.
type Resources struct {
	Limits       CPUMem
	Reservations CPUMem
}

// DeploymentSpec mirrors Swarm deployment options relevant to Swarm services.
type DeploymentSpec struct {
	Replicas    int
	Constraints []string
	Resources   Resources
}

// ServiceSpec describes a Swarm service coupled with config/secret mounts.
type ServiceSpec struct {
	Name       string
	Image      string
	Env        map[string]string
	Networks   []string
	Labels     map[string]string
	Configs    []ConfigSpec
	Secrets    []SecretSpec
	Deployment DeploymentSpec
}

// RenderedConfig pairs a ConfigSpec with its rendered bytes.
type RenderedConfig struct {
	Spec ConfigSpec
	Data []byte
}

// RenderedSecret pairs a SecretSpec with its rendered bytes.
type RenderedSecret struct {
	Spec SecretSpec
	Data []byte
}

// RenderedService bundles the service spec with rendered configs/secrets.
type RenderedService struct {
	Spec    ServiceSpec
	Configs []RenderedConfig
	Secrets []RenderedSecret
}
