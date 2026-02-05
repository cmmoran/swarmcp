package config

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/dlclark/regexp2"
	"gopkg.in/yaml.v3"
)

func normalizeTemplateScalars(input string) string {
	lines := strings.Split(input, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if prefix, value, suffix, ok := splitTemplateScalar(line); ok {
			lines[i] = prefix + "'" + escapeSingleQuotes(value) + "'" + suffix
		}
	}
	return strings.Join(lines, "\n")
}

func splitTemplateScalar(line string) (string, string, string, bool) {
	if prefix, value, suffix, ok := splitMappingTemplate(line); ok {
		return prefix, value, suffix, true
	}
	if prefix, value, suffix, ok := splitListTemplate(line); ok {
		return prefix, value, suffix, true
	}
	return "", "", "", false
}

func splitMappingTemplate(line string) (string, string, string, bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", "", false
	}
	prefix := line[:idx+1]
	rest := strings.TrimSpace(line[idx+1:])
	value, suffix := splitInlineComment(rest)
	if isBareTemplateScalar(value) {
		return prefix + " ", value, suffix, true
	}
	return "", "", "", false
}

func splitListTemplate(line string) (string, string, string, bool) {
	trimmed := strings.TrimLeft(line, " \t")
	if !strings.HasPrefix(trimmed, "- ") {
		return "", "", "", false
	}
	prefixLen := len(line) - len(trimmed)
	prefix := line[:prefixLen+2]
	rest := strings.TrimSpace(trimmed[2:])
	value, suffix := splitInlineComment(rest)
	if isBareTemplateScalar(value) {
		return prefix, value, suffix, true
	}
	return "", "", "", false
}

func splitInlineComment(value string) (string, string) {
	var inSingle bool
	var inDouble bool
	for i, r := range value {
		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble {
				return strings.TrimSpace(value[:i]), value[i:]
			}
		}
	}
	return strings.TrimSpace(value), ""
}

func isBareTemplateScalar(value string) bool {
	if value == "" {
		return false
	}
	if strings.HasPrefix(value, "'") || strings.HasPrefix(value, "\"") {
		return false
	}
	return strings.HasPrefix(value, "{{") && strings.HasSuffix(value, "}}")
}

func escapeSingleQuotes(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

type LoadOptions struct {
	Offline  bool
	CacheDir string
	Debug    bool
}

func Load(path string) (*Config, error) {
	return LoadWithOptions(path, LoadOptions{})
}

func LoadWithOptions(path string, opts LoadOptions) (*Config, error) {
	configPath := path
	if abs, err := filepath.Abs(path); err == nil {
		configPath = abs
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	normalized := normalizeTemplateScalars(string(data))
	var cfg Config
	if err := yaml.Unmarshal([]byte(normalized), &cfg); err != nil {
		return nil, err
	}

	SetBaseDir(&cfg, configPath)
	opts = normalizeLoadOptions(opts, cfg.BaseDir)
	cfg.CacheDir = opts.CacheDir
	cfg.Offline = opts.Offline
	cfg.Debug = opts.Debug
	if err := ResolveImports(&cfg, opts); err != nil {
		return nil, err
	}
	SetSourcesBaseDir(&cfg)
	if err := ApplySourceBaseDir(&cfg, opts); err != nil {
		return nil, err
	}
	if err := Validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func normalizeLoadOptions(opts LoadOptions, baseDir string) LoadOptions {
	if opts.CacheDir == "" && baseDir != "" {
		opts.CacheDir = filepath.Join(baseDir, ".swarmcp", "sources")
	}
	return opts
}

func SetBaseDir(cfg *Config, configPath string) {
	if cfg == nil || configPath == "" {
		return
	}
	absPath := configPath
	if abs, err := filepath.Abs(configPath); err == nil {
		absPath = abs
	}
	cfg.BaseDir = filepath.Dir(absPath)
}

func Validate(cfg *Config) error {
	var errs []string

	if cfg.Project.Name == "" {
		errs = append(errs, "project.name is required")
	}
	if cfg.Project.PreserveUnusedResources != nil && *cfg.Project.PreserveUnusedResources < 0 {
		errs = append(errs, "project.preserve_unused_resources must be >= 0")
	}

	for _, partition := range cfg.Project.Partitions {
		if partition == "_" {
			errs = append(errs, "partition name '_' is reserved")
		}
		if err := validateLogicalName("partition "+partition, partition); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if err := ValidateDeployment(cfg); err != nil {
		errs = append(errs, err.Error())
	}

	if err := validateConfigDefs("project.configs", cfg.Project.Configs); err != nil {
		errs = append(errs, err.Error())
	}
	if err := validateSecretDefs("project.secrets", cfg.Project.Secrets); err != nil {
		errs = append(errs, err.Error())
	}
	errs = append(errs, validateRestartPolicy("project.restart_policy", cfg.Project.RestartPolicy)...)
	errs = append(errs, validateUpdatePolicy("project.update_config", cfg.Project.UpdateConfig)...)
	errs = append(errs, validateUpdatePolicy("project.rollback_config", cfg.Project.RollbackConfig)...)
	if err := validateSecretsEngine(cfg.Project.SecretsEngine); err != nil {
		errs = append(errs, err.Error())
	}
	if err := validateStandardMounts("project.defaults.volumes.standards", cfg.Project.Defaults.Volumes.Standards); err != nil {
		errs = append(errs, err.Error())
	}
	serviceStandard := ServiceStandardName(cfg)
	if err := validateLogicalName("project.defaults.volumes.service_standard", serviceStandard); err != nil {
		errs = append(errs, err.Error())
	}
	if _, ok := cfg.Project.Defaults.Volumes.Standards[serviceStandard]; ok {
		errs = append(errs, fmt.Sprintf("project.defaults.volumes.standards.%s: reserved for service default volume", serviceStandard))
	}

	for stackName, stack := range cfg.Stacks {
		if err := validateLogicalName("stack "+stackName, stackName); err != nil {
			errs = append(errs, err.Error())
		}
		if err := validateStack(cfg, stackName, stack, cfg.Project.Defaults.Volumes.Standards, serviceStandard); err != nil {
			errs = append(errs, err.Error())
		}
	}

	for nodeName, node := range cfg.Project.Nodes {
		if err := validateNode(nodeName, node); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if err := validateDeploymentTargets(cfg); err != nil {
		errs = append(errs, err.Error())
	}

	if err := validateOverlays(cfg); err != nil {
		errs = append(errs, err.Error())
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n- %s", joinErrors(errs))
	}

	return nil
}

func validateSecretsEngine(engine *SecretsEngine) error {
	if engine == nil {
		return nil
	}
	var errs []string
	if engine.Provider == "" {
		errs = append(errs, "secrets_engine.provider is required")
	} else if engine.Provider != "vault" && engine.Provider != "bao" && engine.Provider != "openbao" {
		errs = append(errs, fmt.Sprintf("secrets_engine.provider %q is not supported", engine.Provider))
	}
	if engine.Provider != "" {
		if engine.Addr == "" {
			errs = append(errs, "secrets_engine.addr is required")
		}
		if engine.Provider == "vault" || engine.Provider == "bao" || engine.Provider == "openbao" {
			if engine.Vault == nil {
				errs = append(errs, "secrets_engine.vault is required for vault provider")
			} else {
				if engine.Vault.Mount == "" {
					errs = append(errs, "secrets_engine.vault.mount is required")
				}
				if engine.Vault.PathTemplate == "" {
					errs = append(errs, "secrets_engine.vault.path_template is required")
				}
			}
			if engine.Auth.Method != "" {
				switch engine.Auth.Method {
				case "jwt", "kubernetes", "approle", "oidc", "tls":
				default:
					errs = append(errs, fmt.Sprintf("secrets_engine.auth.method %q is not supported", engine.Auth.Method))
				}
				if (engine.Auth.Method == "jwt" || engine.Auth.Method == "kubernetes" || engine.Auth.Method == "oidc") && engine.Auth.Role == "" {
					errs = append(errs, fmt.Sprintf("secrets_engine.auth.role is required for %s auth", engine.Auth.Method))
				}
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", joinErrors(errs))
	}
	return nil
}

func ValidateDeployment(cfg *Config) error {
	var errs []string
	if cfg.Project.Deployment != "" {
		if err := validateLogicalName("project.deployment", cfg.Project.Deployment); err != nil {
			errs = append(errs, err.Error())
		}
		if len(cfg.Project.Deployments) > 0 {
			found := false
			for _, name := range cfg.Project.Deployments {
				if name == cfg.Project.Deployment {
					found = true
					break
				}
			}
			if !found {
				errs = append(errs, fmt.Sprintf("project.deployment %q not found in project.deployments", cfg.Project.Deployment))
			}
		}
	}
	for _, name := range cfg.Project.Deployments {
		if err := validateLogicalName("deployment "+name, name); err != nil {
			errs = append(errs, err.Error())
		}
	}
	for name, context := range cfg.Project.Contexts {
		if err := validateLogicalName("deployment context "+name, name); err != nil {
			errs = append(errs, err.Error())
		}
		if len(cfg.Project.Deployments) > 0 && !deploymentInProject(cfg, name) {
			errs = append(errs, fmt.Sprintf("project.contexts %q not found in project.deployments", name))
		}
		if strings.TrimSpace(context) == "" {
			errs = append(errs, fmt.Sprintf("project.contexts.%s is required", name))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", joinErrors(errs))
	}
	return nil
}

func validateStack(cfg *Config, name string, stack Stack, standards map[string]StandardMount, serviceStandard string) error {
	var errs []string

	if stack.Mode != "" && stack.Mode != "shared" && stack.Mode != "partitioned" {
		errs = append(errs, fmt.Sprintf("stack %q: invalid mode %q", name, stack.Mode))
	}
	errs = append(errs, validateRestartPolicy("stack "+name+".restart_policy", stack.RestartPolicy)...)
	errs = append(errs, validateUpdatePolicy("stack "+name+".update_config", stack.UpdateConfig)...)
	errs = append(errs, validateUpdatePolicy("stack "+name+".rollback_config", stack.RollbackConfig)...)
	if name == "core" && stack.Mode == "partitioned" {
		errs = append(errs, "stack \"core\": reserved for shared stack mode")
	}

	if err := validateConfigDefs("stack "+name+".configs", stack.Configs.Defs); err != nil {
		errs = append(errs, err.Error())
	}
	if err := validateSecretDefs("stack "+name+".secrets", stack.Secrets.Defs); err != nil {
		errs = append(errs, err.Error())
	}

	for partitionName, partition := range stack.Partitions {
		if partitionName == "_" {
			errs = append(errs, fmt.Sprintf("stack %q partition '_' is reserved", name))
		}
		if err := validateLogicalName("stack "+name+" partition "+partitionName, partitionName); err != nil {
			errs = append(errs, err.Error())
		}
		errs = append(errs, validateRestartPolicy("stack "+name+".partitions."+partitionName+".restart_policy", partition.RestartPolicy)...)
		errs = append(errs, validateUpdatePolicy("stack "+name+".partitions."+partitionName+".update_config", partition.UpdateConfig)...)
		errs = append(errs, validateUpdatePolicy("stack "+name+".partitions."+partitionName+".rollback_config", partition.RollbackConfig)...)
		if err := validateConfigDefs("stack "+name+".partition "+partitionName+".configs", partition.Configs.Defs); err != nil {
			errs = append(errs, err.Error())
		}
		if err := validateSecretDefs("stack "+name+".partition "+partitionName+".secrets", partition.Secrets.Defs); err != nil {
			errs = append(errs, err.Error())
		}
	}

	for serviceName := range stack.Services {
		if err := validateLogicalName("stack "+name+" service "+serviceName, serviceName); err != nil {
			errs = append(errs, err.Error())
		}
	}
	for serviceName, service := range stack.Services {
		if err := validateService(name, serviceName, service, stack.Services, standards); err != nil {
			errs = append(errs, err.Error())
		}
		if err := validateServiceConfigs("stack "+name+".services."+serviceName+".configs", service.Configs); err != nil {
			errs = append(errs, err.Error())
		}
		if err := validateServiceSecrets("stack "+name+".services."+serviceName+".secrets", service.Secrets); err != nil {
			errs = append(errs, err.Error())
		}
		if err := validateServiceVolumes("stack "+name+".services."+serviceName+".volumes", service.Volumes, standards, serviceStandard); err != nil {
			errs = append(errs, err.Error())
		}
		if err := validateServiceOverlays(cfg, name, serviceName, service, stack.Services, standards, serviceStandard); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if err := validateStackVolumes("stack "+name+".volumes", stack.Volumes); err != nil {
		errs = append(errs, err.Error())
	}
	if err := validateStackOverlays(cfg, name, stack); err != nil {
		errs = append(errs, err.Error())
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s", joinErrors(errs))
	}

	return nil
}

func validateService(stackName string, serviceName string, service Service, services map[string]Service, standards map[string]StandardMount) error {
	var errs []string
	scope := "stack " + stackName + ".services." + serviceName

	if service.Image == "" {
		errs = append(errs, scope+".image is required")
	}
	if service.Mode != "" && service.Mode != "replicated" && service.Mode != "global" {
		errs = append(errs, fmt.Sprintf("%s.mode: invalid value %q", scope, service.Mode))
	}
	if service.Mode == "global" && service.Replicas > 0 {
		errs = append(errs, fmt.Sprintf("%s.replicas: must be omitted or 0 when mode is global", scope))
	}
	if service.Replicas < 0 {
		errs = append(errs, fmt.Sprintf("%s.replicas: must be >= 0", scope))
	}
	errs = append(errs, validateRestartPolicy(scope+".restart_policy", service.RestartPolicy)...)
	errs = append(errs, validateUpdatePolicy(scope+".update_config", service.UpdateConfig)...)
	errs = append(errs, validateUpdatePolicy(scope+".rollback_config", service.RollbackConfig)...)
	if len(service.Networks) > 0 {
		errs = append(errs, fmt.Sprintf("%s.networks: networks are derived; remove service-level networks", scope))
	}

	for key := range service.Env {
		if key == "" {
			errs = append(errs, fmt.Sprintf("%s.env: empty key is not allowed", scope))
		}
	}
	for key := range service.Labels {
		if key == "" {
			errs = append(errs, fmt.Sprintf("%s.labels: empty key is not allowed", scope))
			continue
		}
		if strings.HasPrefix(key, "swarmcp.io/") {
			errs = append(errs, fmt.Sprintf("%s.labels: %q uses reserved prefix swarmcp.io/", scope, key))
		}
	}
	for _, constraint := range service.Placement.Constraints {
		if strings.TrimSpace(constraint) == "" {
			errs = append(errs, fmt.Sprintf("%s.placement.constraints: empty constraint", scope))
		}
	}

	for _, dep := range service.DependsOn {
		if err := validateLogicalName(scope+".depends_on", dep); err != nil {
			errs = append(errs, err.Error())
			continue
		}
		if _, ok := services[dep]; !ok {
			errs = append(errs, fmt.Sprintf("%s.depends_on: service %q not found", scope, dep))
		}
	}

	for i, port := range service.Ports {
		if port.Target <= 0 {
			errs = append(errs, fmt.Sprintf("%s.ports[%d].target: must be > 0", scope, i))
		}
		if port.Protocol != "" && port.Protocol != "tcp" && port.Protocol != "udp" {
			errs = append(errs, fmt.Sprintf("%s.ports[%d].protocol: invalid value %q", scope, i, port.Protocol))
		}
		if port.Mode != "" && port.Mode != "ingress" && port.Mode != "host" {
			errs = append(errs, fmt.Sprintf("%s.ports[%d].mode: invalid value %q", scope, i, port.Mode))
		}
		if port.Published < 0 {
			errs = append(errs, fmt.Sprintf("%s.ports[%d].published: must be >= 0", scope, i))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s", joinErrors(errs))
	}

	return nil
}

func validateNode(name string, node Node) error {
	var errs []string

	for _, role := range node.Roles {
		if role != "manager" && role != "worker" {
			errs = append(errs, fmt.Sprintf("node %q: invalid role %q", name, role))
		}
	}
	if node.Platform.OS != "" && node.Platform.Arch == "" {
		errs = append(errs, fmt.Sprintf("node %q platform.arch is required when platform.os is set", name))
	}
	if node.Platform.Arch != "" && node.Platform.OS == "" {
		errs = append(errs, fmt.Sprintf("node %q platform.os is required when platform.arch is set", name))
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s", joinErrors(errs))
	}

	return nil
}

func validateDeploymentTargets(cfg *Config) error {
	var errs []string
	for name, target := range cfg.Project.Targets {
		if err := validateLogicalName("deployment target "+name, name); err != nil {
			errs = append(errs, err.Error())
		}
		if len(cfg.Project.Deployments) > 0 && !deploymentInProject(cfg, name) {
			errs = append(errs, fmt.Sprintf("deployment target %q not found in project.deployments", name))
		}
		errs = append(errs, validateNodeSelector("deployment target "+name+".include", target.Include)...)
		errs = append(errs, validateNodeSelector("deployment target "+name+".exclude", target.Exclude)...)
		for nodeName, override := range target.Overrides {
			if err := validateLogicalName("deployment target "+name+" override "+nodeName, nodeName); err != nil {
				errs = append(errs, err.Error())
			}
			if !nodeInProject(cfg, nodeName) {
				errs = append(errs, fmt.Sprintf("deployment target %q override node %q not found in project.nodes", name, nodeName))
				continue
			}
			if err := validateNodeSpec("deployment target "+name+".overrides."+nodeName, override); err != nil {
				errs = append(errs, err.Error())
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", joinErrors(errs))
	}
	return nil
}

func validateNodeSpec(scope string, node NodeSpec) error {
	var errs []string
	for _, role := range node.Roles {
		if role != "manager" && role != "worker" {
			errs = append(errs, fmt.Sprintf("%s: invalid role %q", scope, role))
		}
	}
	if node.Platform.OS != "" && node.Platform.Arch == "" {
		errs = append(errs, fmt.Sprintf("%s.platform.arch is required when platform.os is set", scope))
	}
	if node.Platform.Arch != "" && node.Platform.OS == "" {
		errs = append(errs, fmt.Sprintf("%s.platform.os is required when platform.arch is set", scope))
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", joinErrors(errs))
	}
	return nil
}

func validateNodeSelector(scope string, selector NodeSelector) []string {
	var errs []string
	for _, name := range selector.Names {
		if err := validateLogicalName(scope+" name "+name, name); err != nil {
			errs = append(errs, err.Error())
		}
	}
	for key, value := range selector.Labels {
		if key == "" {
			errs = append(errs, fmt.Sprintf("%s: label key is required", scope))
		}
		if value == "" {
			errs = append(errs, fmt.Sprintf("%s: label %q value is required", scope, key))
		}
	}
	return errs
}

func nodeInProject(cfg *Config, name string) bool {
	if cfg.Project.Nodes == nil {
		return false
	}
	_, ok := cfg.Project.Nodes[name]
	return ok
}

func deploymentInProject(cfg *Config, name string) bool {
	for _, deployment := range cfg.Project.Deployments {
		if deployment == name {
			return true
		}
	}
	return false
}

func validateConfigDefs(scope string, defs map[string]ConfigDef) error {
	var errs []string
	for name, def := range defs {
		if err := validateLogicalName(scope+" "+name, name); err != nil {
			errs = append(errs, err.Error())
		}
		if def.Source == "" {
			errs = append(errs, fmt.Sprintf("%s %q: source is required", scope, name))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s", joinErrors(errs))
	}

	return nil
}

func validateSecretDefs(scope string, defs map[string]SecretDef) error {
	var errs []string
	for name := range defs {
		if err := validateLogicalName(scope+" "+name, name); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s", joinErrors(errs))
	}

	return nil
}

const (
	maxDockerNameLen  = 63
	hashSuffixLen     = 12
	nameSeparatorLen  = 1
	maxLogicalNameLen = maxDockerNameLen - hashSuffixLen - nameSeparatorLen
)

func validateLogicalName(label string, value string) error {
	if value == "" {
		return fmt.Errorf("%s: name is required", label)
	}
	if len(value) > maxLogicalNameLen {
		return fmt.Errorf("%s: name length %d exceeds max %d", label, len(value), maxLogicalNameLen)
	}
	return nil
}

func joinErrors(errs []string) string {
	if len(errs) == 0 {
		return ""
	}
	out := errs[0]
	for i := 1; i < len(errs); i++ {
		out += "\n- " + errs[i]
	}
	return out
}

func validateServiceConfigs(scope string, refs []ConfigRef) error {
	var errs []string
	for _, ref := range refs {
		if ref.Name == "" {
			errs = append(errs, fmt.Sprintf("%s: name is required", scope))
			continue
		}
		if err := validateLogicalName(scope+" "+ref.Name, ref.Name); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", joinErrors(errs))
	}
	return nil
}

func validateServiceSecrets(scope string, refs []SecretRef) error {
	var errs []string
	for _, ref := range refs {
		if ref.Name == "" {
			errs = append(errs, fmt.Sprintf("%s: name is required", scope))
			continue
		}
		if err := validateLogicalName(scope+" "+ref.Name, ref.Name); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", joinErrors(errs))
	}
	return nil
}

func validateStackVolumes(scope string, volumes map[string]VolumeDef) error {
	if len(volumes) == 0 {
		return nil
	}
	var errs []string
	for name, def := range volumes {
		if err := validateLogicalName(scope+" "+name, name); err != nil {
			errs = append(errs, err.Error())
		}
		if strings.TrimSpace(def.Target) == "" {
			errs = append(errs, fmt.Sprintf("%s.%s.target: required", scope, name))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", joinErrors(errs))
	}
	return nil
}

func validateServiceVolumes(scope string, refs []VolumeRef, standards map[string]StandardMount, serviceStandard string) error {
	if len(refs) == 0 {
		return nil
	}
	var errs []string
	for _, ref := range refs {
		if ref.Name != "" {
			if strings.TrimSpace(ref.Category) != "" {
				errs = append(errs, fmt.Sprintf("%s %s.category: only allowed for standard volumes", scope, ref.Name))
			}
			if ref.Standard != "" {
				errs = append(errs, fmt.Sprintf("%s %s.standard: not allowed for named volumes", scope, ref.Name))
			}
			if strings.TrimSpace(ref.Source) != "" {
				errs = append(errs, fmt.Sprintf("%s %s.source: not allowed for named volumes", scope, ref.Name))
			}
			if err := validateLogicalName(scope+" "+ref.Name, ref.Name); err != nil {
				errs = append(errs, err.Error())
			}
			if strings.TrimSpace(ref.Target) == "" {
				errs = append(errs, fmt.Sprintf("%s %s.target: required", scope, ref.Name))
			}
			continue
		}
		if ref.Standard != "" {
			if ref.Standard == serviceStandard {
				if strings.TrimSpace(ref.Source) != "" {
					errs = append(errs, fmt.Sprintf("%s %s.source: not allowed for service standard volume", scope, ref.Standard))
				}
				continue
			}
			if strings.TrimSpace(ref.Source) != "" || strings.TrimSpace(ref.Target) != "" || strings.TrimSpace(ref.Subpath) != "" || ref.ReadOnly {
				errs = append(errs, fmt.Sprintf("%s %s: standard mounts cannot override source/target/subpath/readonly", scope, ref.Standard))
			}
			if standards == nil {
				errs = append(errs, fmt.Sprintf("%s %s: standard mount not found", scope, ref.Standard))
				continue
			}
			if _, ok := standards[ref.Standard]; !ok {
				errs = append(errs, fmt.Sprintf("%s %s: standard mount not found", scope, ref.Standard))
			}
			continue
		}
		if strings.TrimSpace(ref.Category) != "" {
			errs = append(errs, fmt.Sprintf("%s.category: only allowed for standard volumes", scope))
		}
		if strings.TrimSpace(ref.Source) == "" {
			errs = append(errs, fmt.Sprintf("%s.source: required for ad-hoc bind", scope))
		}
		if strings.TrimSpace(ref.Target) == "" {
			errs = append(errs, fmt.Sprintf("%s.target: required for ad-hoc bind", scope))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", joinErrors(errs))
	}
	return nil
}

func validateStandardMounts(scope string, standards map[string]StandardMount) error {
	if len(standards) == 0 {
		return nil
	}
	var errs []string
	for name, def := range standards {
		if err := validateLogicalName(scope+" "+name, name); err != nil {
			errs = append(errs, err.Error())
		}
		if strings.TrimSpace(def.Source) == "" {
			errs = append(errs, fmt.Sprintf("%s.%s.source: required", scope, name))
		}
		if strings.TrimSpace(def.Target) == "" {
			errs = append(errs, fmt.Sprintf("%s.%s.target: required", scope, name))
		}
		for _, role := range def.Requires.Roles {
			if role == "" {
				errs = append(errs, fmt.Sprintf("%s.%s.requires.roles: empty role", scope, name))
				continue
			}
			if role != "manager" && role != "worker" {
				errs = append(errs, fmt.Sprintf("%s.%s.requires.roles: invalid role %q", scope, name, role))
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", joinErrors(errs))
	}
	return nil
}

func validateOverlays(cfg *Config) error {
	var errs []string

	if err := validateDeploymentOverlays(cfg); err != nil {
		errs = append(errs, err.Error())
	}
	if err := validatePartitionOverlays(cfg); err != nil {
		errs = append(errs, err.Error())
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s", joinErrors(errs))
	}
	return nil
}

func validateDeploymentOverlays(cfg *Config) error {
	var errs []string

	for name, overlay := range cfg.Overlays.Deployments {
		if err := validateLogicalName("overlay deployment "+name, name); err != nil {
			errs = append(errs, err.Error())
		}
		if err := validateOverlayProject("overlays.deployments."+name+".project", overlay.Project); err != nil {
			errs = append(errs, err.Error())
		}
		for stackName, stack := range overlay.Stacks {
			if _, ok := cfg.Stacks[stackName]; !ok {
				errs = append(errs, fmt.Sprintf("overlays.deployments.%s.stacks: stack %q not found", name, stackName))
				continue
			}
			if err := validateOverlayStack("overlays.deployments."+name+".stacks."+stackName, stackName, stack, cfg); err != nil {
				errs = append(errs, err.Error())
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s", joinErrors(errs))
	}
	return nil
}

func validatePartitionOverlays(cfg *Config) error {
	var errs []string

	partitions := make(map[string]bool, len(cfg.Project.Partitions))
	for _, name := range cfg.Project.Partitions {
		partitions[name] = true
	}

	for idx, rule := range cfg.Overlays.Partitions.Rules {
		scope := fmt.Sprintf("overlays.partitions[%d]", idx)
		if rule.Name != "" {
			scope = fmt.Sprintf("%s(%s)", scope, rule.Name)
		}
		if rule.Name == "_" {
			errs = append(errs, "overlay partition '_' is reserved")
		}
		if rule.Name != "" {
			if err := validateLogicalName("overlay partition "+rule.Name, rule.Name); err != nil {
				errs = append(errs, err.Error())
			}
		}
		errs = append(errs, validatePartitionMatch(scope, rule.Match.Partition, partitions)...)
		if err := validateOverlayProject(scope+".project", rule.Project); err != nil {
			errs = append(errs, err.Error())
		}
		for stackName, stack := range rule.Stacks {
			if _, ok := cfg.Stacks[stackName]; !ok {
				errs = append(errs, fmt.Sprintf("%s.stacks: stack %q not found", scope, stackName))
				continue
			}
			if err := validateOverlayStack(scope+".stacks."+stackName, stackName, stack, cfg); err != nil {
				errs = append(errs, err.Error())
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s", joinErrors(errs))
	}
	return nil
}

func validateOverlayProject(scope string, project OverlayProject) error {
	var errs []string
	if err := validateConfigDefs(scope+".configs", project.Configs); err != nil {
		errs = append(errs, err.Error())
	}
	if err := validateSecretDefs(scope+".secrets", project.Secrets); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", joinErrors(errs))
	}
	return nil
}

func validateOverlayStack(scope string, stackName string, stack OverlayStack, cfg *Config) error {
	var errs []string
	if err := validateConfigDefs(scope+".configs", stack.Configs.Defs); err != nil {
		errs = append(errs, err.Error())
	}
	if err := validateSecretDefs(scope+".secrets", stack.Secrets.Defs); err != nil {
		errs = append(errs, err.Error())
	}
	baseServices := map[string]Service{}
	if baseStack, ok := cfg.Stacks[stackName]; ok {
		for name, service := range baseStack.Services {
			baseServices[name] = service
		}
	}
	mergedServices := map[string]Service{}
	for name, service := range baseServices {
		mergedServices[name] = service
	}
	for serviceName, service := range stack.Services {
		if _, ok := service.Fields["source"]; ok {
			errs = append(errs, fmt.Sprintf("%s.services.%s.source: overrides do not support source", scope, serviceName))
		}
		if _, ok := service.Fields["overrides"]; ok {
			errs = append(errs, fmt.Sprintf("%s.services.%s.overrides: overrides are not supported in overlays", scope, serviceName))
		}
		base := baseServices[serviceName]
		merged, err := mergeServiceOverlay(base, service)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s.services.%s: %v", scope, serviceName, err))
			continue
		}
		mergedServices[serviceName] = merged
	}
	standards := cfg.Project.Defaults.Volumes.Standards
	serviceStandard := cfg.Project.Defaults.Volumes.ServiceStandard
	for serviceName := range stack.Services {
		merged := mergedServices[serviceName]
		if err := validateService(stackName, serviceName, merged, mergedServices, standards); err != nil {
			errs = append(errs, err.Error())
		}
		if err := validateServiceConfigs(scope+".services."+serviceName+".configs", merged.Configs); err != nil {
			errs = append(errs, err.Error())
		}
		if err := validateServiceSecrets(scope+".services."+serviceName+".secrets", merged.Secrets); err != nil {
			errs = append(errs, err.Error())
		}
		if err := validateServiceVolumes(scope+".services."+serviceName+".volumes", merged.Volumes, standards, serviceStandard); err != nil {
			errs = append(errs, err.Error())
		}
	}
	for partitionName, partition := range stack.Partitions {
		if partitionName == "_" {
			errs = append(errs, fmt.Sprintf("%s.partitions: '_' is reserved", scope))
		}
		if err := validateLogicalName(scope+" partition "+partitionName, partitionName); err != nil {
			errs = append(errs, err.Error())
		}
		if !partitionInProject(cfg, partitionName) {
			errs = append(errs, fmt.Sprintf("%s.partitions.%s: partition not found in project.partitions", scope, partitionName))
		}
		if err := validateConfigDefs(scope+".partitions."+partitionName+".configs", partition.Configs.Defs); err != nil {
			errs = append(errs, err.Error())
		}
		if err := validateSecretDefs(scope+".partitions."+partitionName+".secrets", partition.Secrets.Defs); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", joinErrors(errs))
	}
	return nil
}

func validatePartitionMatch(scope string, match OverlayMatchPartition, partitions map[string]bool) []string {
	var errs []string
	pattern := strings.TrimSpace(match.Pattern)
	if pattern == "" {
		return nil
	}
	matchType := strings.ToLower(strings.TrimSpace(match.Type))
	if matchType == "" {
		if hasGlob(pattern) {
			matchType = "glob"
		} else {
			matchType = "exact"
		}
	}
	switch matchType {
	case "exact":
		if !partitions[pattern] {
			errs = append(errs, fmt.Sprintf("%s.match.partition: partition %q not found in project.partitions", scope, pattern))
		}
	case "glob":
		if _, err := path.Match(pattern, "partition"); err != nil {
			errs = append(errs, fmt.Sprintf("%s.match.partition: invalid glob pattern: %v", scope, err))
		}
	case "regexp":
		if _, err := regexp2.Compile(pattern, regexp2.RE2); err != nil {
			errs = append(errs, fmt.Sprintf("%s.match.partition: invalid regexp: %v", scope, err))
		}
	default:
		errs = append(errs, fmt.Sprintf("%s.match.partition: unsupported match type %q", scope, matchType))
	}
	return errs
}

func validateStackOverlays(cfg *Config, stackName string, stack Stack) error {
	var errs []string
	partitions := make(map[string]bool, len(cfg.Project.Partitions))
	for _, name := range cfg.Project.Partitions {
		partitions[name] = true
	}
	for name, overlay := range stack.Overlays.Deployments {
		if err := validateLogicalName("stack "+stackName+" overlay deployment "+name, name); err != nil {
			errs = append(errs, err.Error())
		}
		if err := validateOverlayStack("stack "+stackName+".overlays.deployments."+name, stackName, overlay, cfg); err != nil {
			errs = append(errs, err.Error())
		}
	}
	for idx, rule := range stack.Overlays.Partitions.Rules {
		scope := fmt.Sprintf("stack %s.overlays.partitions[%d]", stackName, idx)
		if rule.Name != "" {
			scope = fmt.Sprintf("%s(%s)", scope, rule.Name)
		}
		if rule.Name == "_" {
			errs = append(errs, fmt.Sprintf("stack %q overlay partition '_' is reserved", stackName))
		}
		if rule.Name != "" {
			if err := validateLogicalName("stack "+stackName+" overlay partition "+rule.Name, rule.Name); err != nil {
				errs = append(errs, err.Error())
			}
		}
		errs = append(errs, validatePartitionMatch(scope, rule.Match.Partition, partitions)...)
		if err := validateOverlayStack(scope, stackName, rule.OverlayStack, cfg); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", joinErrors(errs))
	}
	return nil
}

func validateServiceOverlays(cfg *Config, stackName string, serviceName string, service Service, services map[string]Service, standards map[string]StandardMount, serviceStandard string) error {
	var errs []string
	scope := "stack " + stackName + ".services." + serviceName + ".overlays"
	partitions := make(map[string]bool, len(cfg.Project.Partitions))
	for _, name := range cfg.Project.Partitions {
		partitions[name] = true
	}
	for name, overlay := range service.Overlays.Deployments {
		if err := validateLogicalName(scope+".deployments."+name, name); err != nil {
			errs = append(errs, err.Error())
		}
		if err := validateOverlayService(scope+".deployments."+name, stackName, serviceName, service, services, overlay, standards, serviceStandard); err != nil {
			errs = append(errs, err.Error())
		}
	}
	for idx, rule := range service.Overlays.Partitions.Rules {
		overlayScope := fmt.Sprintf("%s.partitions[%d]", scope, idx)
		if rule.Name != "" {
			overlayScope = fmt.Sprintf("%s(%s)", overlayScope, rule.Name)
		}
		if rule.Name == "_" {
			errs = append(errs, fmt.Sprintf("%s: overlay partition '_' is reserved", overlayScope))
		}
		if rule.Name != "" {
			if err := validateLogicalName(scope+" partition "+rule.Name, rule.Name); err != nil {
				errs = append(errs, err.Error())
			}
		}
		errs = append(errs, validatePartitionMatch(overlayScope, rule.Match.Partition, partitions)...)
		if err := validateOverlayService(overlayScope, stackName, serviceName, service, services, rule.Service, standards, serviceStandard); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", joinErrors(errs))
	}
	return nil
}

func validateOverlayService(scope string, stackName string, serviceName string, base Service, services map[string]Service, overlay OverlayService, standards map[string]StandardMount, serviceStandard string) error {
	var errs []string
	if _, ok := overlay.Fields["source"]; ok {
		errs = append(errs, fmt.Sprintf("%s: overlays do not support source", scope))
	}
	if _, ok := overlay.Fields["overrides"]; ok {
		errs = append(errs, fmt.Sprintf("%s: overlays do not support overrides", scope))
	}
	merged, err := mergeServiceOverlay(base, overlay)
	if err != nil {
		return fmt.Errorf("%s: %v", scope, err)
	}
	mergedServices := map[string]Service{}
	for name, svc := range services {
		mergedServices[name] = svc
	}
	mergedServices[serviceName] = merged
	if err := validateService(stackName, serviceName, merged, mergedServices, standards); err != nil {
		errs = append(errs, err.Error())
	}
	if err := validateServiceConfigs(scope+".configs", merged.Configs); err != nil {
		errs = append(errs, err.Error())
	}
	if err := validateServiceSecrets(scope+".secrets", merged.Secrets); err != nil {
		errs = append(errs, err.Error())
	}
	if err := validateServiceVolumes(scope+".volumes", merged.Volumes, standards, serviceStandard); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", joinErrors(errs))
	}
	return nil
}

func partitionInProject(cfg *Config, name string) bool {
	for _, partition := range cfg.Project.Partitions {
		if partition == name {
			return true
		}
	}
	return false
}
