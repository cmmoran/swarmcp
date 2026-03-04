package apply

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/render"
	"github.com/cmmoran/swarmcp/internal/sliceutil"
	"github.com/cmmoran/swarmcp/internal/swarm"
	"github.com/cmmoran/swarmcp/internal/templates"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	dockerapi "github.com/docker/docker/api/types/swarm"
	"gopkg.in/yaml.v3"
)

type ServiceMount struct {
	Name   string
	Target string
	UID    string
	GID    string
	Mode   os.FileMode
}

type ServiceUpdate struct {
	Stack     string
	Partition string
	Service   swarm.Service
	Spec      dockerapi.ServiceSpec
	Configs   []ServiceMount
	Secrets   []ServiceMount
}

type ServiceCreate struct {
	Stack     string
	Partition string
	Name      string
	Spec      dockerapi.ServiceSpec
	Configs   []ServiceMount
	Secrets   []ServiceMount
}

func buildServiceChanges(cfg *config.Config, desired DesiredState, values any, services []swarm.Service, networkTargets map[string]string, partitionFilters []string, stackFilters []string, infer bool) ([]ServiceCreate, []ServiceUpdate, error) {
	index := buildDefIndex(desired.Defs)
	serviceIndex := indexServices(services, cfg.Project.Name)

	var creates []ServiceCreate
	var updates []ServiceUpdate
	for stackName, stack := range cfg.Stacks {
		if len(stackFilters) > 0 && !selectorContains(stackFilters, stackName) {
			continue
		}
		partitions := []string{""}
		if stack.Mode == "partitioned" && len(cfg.Project.Partitions) > 0 {
			partitions = sliceutil.FilterPartitions(cfg.Project.Partitions, partitionFilters)
		}

		for _, partitionName := range partitions {
			services, err := cfg.StackServices(stackName, partitionName)
			if err != nil {
				return nil, nil, err
			}
			if len(services) == 0 {
				continue
			}
			for serviceName, service := range services {
				key := serviceKey{
					project:   cfg.Project.Name,
					stack:     stackName,
					partition: partitionName,
					service:   serviceName,
				}
				current, ok := serviceIndex[key.labelKey()]

				build, err := buildServiceIntent(cfg, stackName, stack, partitionName, serviceName, service, values, infer, index)
				if err != nil {
					return nil, nil, err
				}
				desiredIntent := build.Intent
				if !ok {
					spec := dockerapi.ServiceSpec{
						Annotations: dockerapi.Annotations{
							Name:   serviceFullName(cfg.Project.Name, stackName, partitionName, serviceName),
							Labels: build.Labels,
						},
					}
					spec = applyIntentToSpec(spec, desiredIntent)
					creates = append(creates, ServiceCreate{
						Stack:     stackName,
						Partition: partitionName,
						Name:      spec.Annotations.Name,
						Spec:      spec,
						Configs:   build.ConfigMounts,
						Secrets:   build.SecretMounts,
					})
					continue
				}

				currentIntent := intentFromSpec(current.Spec, networkTargets)
				if intentEqual(currentIntent, desiredIntent) {
					continue
				}

				updates = append(updates, ServiceUpdate{
					Stack:     stackName,
					Partition: partitionName,
					Service:   current,
					Spec:      applyIntentToSpec(current.Spec, desiredIntent),
					Configs:   build.ConfigMounts,
					Secrets:   build.SecretMounts,
				})
			}
		}
	}

	return creates, updates, nil
}

type serviceIntent struct {
	Image          string
	Command        []string
	Args           []string
	Workdir        string
	Env            []string
	Ports          []portIntent
	Mode           string
	Replicas       uint64
	Labels         map[string]string
	Constraints    []string
	Healthcheck    *container.HealthConfig
	RestartPolicy  *dockerapi.RestartPolicy
	UpdateConfig   *dockerapi.UpdateConfig
	RollbackConfig *dockerapi.UpdateConfig
	Configs        []ServiceMount
	Secrets        []ServiceMount
	Volumes        []mount.Mount
	Networks       []string
}

type portIntent struct {
	Target    uint32
	Published uint32
	Protocol  dockerapi.PortConfigProtocol
	Mode      dockerapi.PortConfigPublishMode
}

func intentFromConfig(service config.Service, labels map[string]string, constraints []string, configs []ServiceMount, secrets []ServiceMount, volumes []mount.Mount, networks []string, restartPolicy *config.RestartPolicy, updateConfig *config.UpdatePolicy, rollbackConfig *config.UpdatePolicy) (serviceIntent, error) {
	env := envSlice(service.Env)
	ports, err := portIntents(service.Ports)
	if err != nil {
		return serviceIntent{}, err
	}
	healthcheck, err := parseHealthcheck(service.Healthcheck)
	if err != nil {
		return serviceIntent{}, err
	}
	policy, err := swarmRestartPolicy(restartPolicy)
	if err != nil {
		return serviceIntent{}, err
	}
	updateSpec, err := swarmUpdateConfig(updateConfig)
	if err != nil {
		return serviceIntent{}, err
	}
	rollbackSpec, err := swarmUpdateConfig(rollbackConfig)
	if err != nil {
		return serviceIntent{}, err
	}
	mode := service.Mode
	if mode == "" {
		mode = "replicated"
	}
	replicas := uint64(service.Replicas)
	if mode == "global" {
		replicas = 0
	}
	return serviceIntent{
		Image:          service.Image,
		Command:        cloneStrings(service.Command),
		Args:           cloneStrings(service.Args),
		Workdir:        service.Workdir,
		Env:            env,
		Ports:          ports,
		Mode:           mode,
		Replicas:       replicas,
		Labels:         cloneLabels(labels),
		Constraints:    cloneStrings(constraints),
		Healthcheck:    healthcheck,
		RestartPolicy:  policy,
		UpdateConfig:   updateSpec,
		RollbackConfig: rollbackSpec,
		Configs:        configs,
		Secrets:        secrets,
		Volumes:        volumes,
		Networks:       networks,
	}, nil
}

func intentFromSpec(spec dockerapi.ServiceSpec, networkTargets map[string]string) serviceIntent {
	containerSpec := spec.TaskTemplate.ContainerSpec
	var env []string
	var command []string
	var args []string
	var workdir string
	var image string
	var labels map[string]string
	var constraints []string
	var healthcheck *container.HealthConfig
	var configs []ServiceMount
	var secrets []ServiceMount
	var mounts []mount.Mount
	if containerSpec != nil {
		env = cloneStrings(containerSpec.Env)
		command = cloneStrings(containerSpec.Command)
		args = cloneStrings(containerSpec.Args)
		workdir = containerSpec.Dir
		image = containerSpec.Image
		healthcheck = containerSpec.Healthcheck
		configs = configMountsFromRefs(containerSpec.Configs)
		secrets = secretMountsFromRefs(containerSpec.Secrets)
		mounts = cloneMounts(containerSpec.Mounts)
	}
	if spec.Annotations.Labels != nil {
		labels = cloneLabels(spec.Annotations.Labels)
	}
	image = normalizeServiceImage(image, labels)
	labels = filterServiceLabels(labels)
	if spec.TaskTemplate.Placement != nil {
		constraints = cloneStrings(spec.TaskTemplate.Placement.Constraints)
	}
	sort.Strings(env)
	ports := portIntentsFromSpec(spec.EndpointSpec)
	networks := networksFromSpec(spec.TaskTemplate.Networks, networkTargets)
	mode, replicas := modeFromSpec(spec.Mode)
	return serviceIntent{
		Image:          image,
		Command:        command,
		Args:           args,
		Workdir:        workdir,
		Env:            env,
		Ports:          ports,
		Mode:           mode,
		Replicas:       replicas,
		Labels:         labels,
		Constraints:    constraints,
		Healthcheck:    healthcheck,
		RestartPolicy:  cloneRestartPolicy(spec.TaskTemplate.RestartPolicy),
		UpdateConfig:   cloneUpdateConfig(spec.UpdateConfig),
		RollbackConfig: cloneUpdateConfig(spec.RollbackConfig),
		Configs:        configs,
		Secrets:        secrets,
		Volumes:        mounts,
		Networks:       networks,
	}
}

func applyIntentToSpec(spec dockerapi.ServiceSpec, intent serviceIntent) dockerapi.ServiceSpec {
	if spec.TaskTemplate.ContainerSpec == nil {
		spec.TaskTemplate.ContainerSpec = &dockerapi.ContainerSpec{}
	}
	spec.Annotations.Labels = cloneLabels(intent.Labels)
	spec.TaskTemplate.ContainerSpec.Image = intent.Image
	spec.TaskTemplate.ContainerSpec.Command = cloneStrings(intent.Command)
	spec.TaskTemplate.ContainerSpec.Args = cloneStrings(intent.Args)
	spec.TaskTemplate.ContainerSpec.Dir = intent.Workdir
	spec.TaskTemplate.ContainerSpec.Env = cloneStrings(intent.Env)
	spec.TaskTemplate.ContainerSpec.Healthcheck = intent.Healthcheck
	spec.TaskTemplate.ContainerSpec.Mounts = cloneMounts(intent.Volumes)
	spec.TaskTemplate.Placement = applyPlacement(spec.TaskTemplate.Placement, intent.Constraints)
	spec.TaskTemplate.RestartPolicy = cloneRestartPolicy(intent.RestartPolicy)
	spec.UpdateConfig = cloneUpdateConfig(intent.UpdateConfig)
	spec.RollbackConfig = cloneUpdateConfig(intent.RollbackConfig)
	spec.EndpointSpec = applyPorts(spec.EndpointSpec, intent.Ports)
	spec.TaskTemplate.Networks = applyNetworks(spec.TaskTemplate.Networks, intent.Networks)
	spec.Mode = applyMode(spec.Mode, intent.Mode, intent.Replicas)
	return spec
}

func intentEqual(current, desired serviceIntent) bool {
	current = canonicalizeIntentForCompare(current)
	desired = canonicalizeIntentForCompare(desired)
	if current.Image != desired.Image {
		return false
	}
	if current.Workdir != desired.Workdir {
		return false
	}
	if current.Mode != desired.Mode || current.Replicas != desired.Replicas {
		return false
	}
	if !labelsEqual(current.Labels, desired.Labels) {
		return false
	}
	if !stringSlicesEqual(current.Constraints, desired.Constraints) {
		return false
	}
	if !stringSlicesEqual(current.Command, desired.Command) {
		return false
	}
	if !stringSlicesEqual(current.Args, desired.Args) {
		return false
	}
	if !stringSlicesEqual(current.Env, desired.Env) {
		return false
	}
	if !portIntentsEqual(current.Ports, desired.Ports) {
		return false
	}
	if !healthcheckEqual(current.Healthcheck, desired.Healthcheck) {
		return false
	}
	if !restartPoliciesEqual(current.RestartPolicy, desired.RestartPolicy) {
		return false
	}
	if !updateConfigsEqual(current.UpdateConfig, desired.UpdateConfig) {
		return false
	}
	if !updateConfigsEqual(current.RollbackConfig, desired.RollbackConfig) {
		return false
	}
	if !mountSlicesEqual(current.Configs, desired.Configs) {
		return false
	}
	if !mountSlicesEqual(current.Secrets, desired.Secrets) {
		return false
	}
	if !volumeMountsEqual(current.Volumes, desired.Volumes) {
		return false
	}
	if !stringSlicesEqual(current.Networks, desired.Networks) {
		return false
	}
	return true
}

type defKey struct {
	kind  string
	scope templates.Scope
	name  string
}

func normalizeScopeKey(scope templates.Scope) templates.Scope {
	scope.NetworksShared = ""
	scope.NetworkEphemeral = ""
	return scope
}

func buildDefIndex(defs []render.RenderedDef) map[defKey]string {
	index := make(map[defKey]string, len(defs))
	for _, def := range defs {
		physical, _ := render.PhysicalName(def.Name, def.Content)
		index[defKey{kind: def.Kind, scope: normalizeScopeKey(def.ScopeID), name: def.Name}] = physical
	}
	return index
}

func mergeConfigRefs(existing []config.ConfigRef, inferred map[string]struct{}) []config.ConfigRef {
	if len(inferred) == 0 {
		return existing
	}
	out := make([]config.ConfigRef, 0, len(existing)+len(inferred))
	seen := make(map[string]struct{}, len(existing))
	for _, ref := range existing {
		if ref.Name == "" {
			continue
		}
		seen[ref.Name] = struct{}{}
		out = append(out, ref)
	}
	names := make([]string, 0, len(inferred))
	for name := range inferred {
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		out = append(out, config.ConfigRef{Name: name})
	}
	return out
}

func mergeSecretRefs(existing []config.SecretRef, inferred map[string]struct{}) []config.SecretRef {
	if len(inferred) == 0 {
		return existing
	}
	out := make([]config.SecretRef, 0, len(existing)+len(inferred))
	seen := make(map[string]struct{}, len(existing))
	for _, ref := range existing {
		if ref.Name == "" {
			continue
		}
		seen[ref.Name] = struct{}{}
		out = append(out, ref)
	}
	names := make([]string, 0, len(inferred))
	for name := range inferred {
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		out = append(out, config.SecretRef{Name: name})
	}
	return out
}

func desiredConfigMounts(resolver *templates.ScopeResolver, engine *templates.Engine, data render.TemplateData, index map[defKey]string, scope templates.Scope, refs []config.ConfigRef, infer bool) ([]ServiceMount, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	out := make([]ServiceMount, 0, len(refs))
	for _, ref := range refs {
		if ref.Name == "" {
			continue
		}
		def, resolvedScope, ok := resolver.ResolveConfigWithScope(ref.Name)
		if !ok {
			if !infer {
				return nil, fmt.Errorf("service %q config %q: definition not found", scope.Service, ref.Name)
			}
			resolvedScope = scope
		}
		physical, ok := index[defKey{kind: "config", scope: normalizeScopeKey(resolvedScope), name: ref.Name}]
		if !ok {
			return nil, fmt.Errorf("service %q config %q: rendered definition missing", scope.Service, ref.Name)
		}
		target := firstNonEmpty(ref.Target, def.Target)
		if target == "" {
			target = "/" + ref.Name
		} else {
			renderedTarget, err := render.RenderTemplateString(engine, data, fmt.Sprintf("configs.%s.target", ref.Name), target)
			if err != nil {
				return nil, fmt.Errorf("service %q config %q: %w", scope.Service, ref.Name, err)
			}
			target = templates.ExpandPathTokens(renderedTarget, scope)
		}
		modeValue, err := render.RenderTemplateString(engine, data, fmt.Sprintf("configs.%s.mode", ref.Name), firstNonEmpty(ref.Mode, def.Mode))
		if err != nil {
			return nil, fmt.Errorf("service %q config %q: %w", scope.Service, ref.Name, err)
		}
		mode, err := parseFileMode(modeValue)
		if err != nil {
			return nil, fmt.Errorf("service %q config %q: %w", scope.Service, ref.Name, err)
		}
		uid, err := render.RenderTemplateString(engine, data, fmt.Sprintf("configs.%s.uid", ref.Name), firstNonEmpty(ref.UID, def.UID))
		if err != nil {
			return nil, fmt.Errorf("service %q config %q: %w", scope.Service, ref.Name, err)
		}
		gid, err := render.RenderTemplateString(engine, data, fmt.Sprintf("configs.%s.gid", ref.Name), firstNonEmpty(ref.GID, def.GID))
		if err != nil {
			return nil, fmt.Errorf("service %q config %q: %w", scope.Service, ref.Name, err)
		}
		out = append(out, ServiceMount{
			Name:   physical,
			Target: target,
			UID:    uid,
			GID:    gid,
			Mode:   mode,
		})
	}
	return out, nil
}

func desiredSecretMounts(resolver *templates.ScopeResolver, engine *templates.Engine, data render.TemplateData, index map[defKey]string, scope templates.Scope, refs []config.SecretRef, infer bool) ([]ServiceMount, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	out := make([]ServiceMount, 0, len(refs))
	for _, ref := range refs {
		if ref.Name == "" {
			continue
		}
		def, resolvedScope, ok := resolver.ResolveSecretWithScope(ref.Name)
		if !ok {
			if !infer {
				return nil, fmt.Errorf("service %q secret %q: definition not found", scope.Service, ref.Name)
			}
			resolvedScope = scope
		}
		physical, ok := index[defKey{kind: "secret", scope: normalizeScopeKey(resolvedScope), name: ref.Name}]
		if !ok {
			return nil, fmt.Errorf("service %q secret %q: rendered definition missing", scope.Service, ref.Name)
		}
		target := firstNonEmpty(ref.Target, def.Target)
		if target == "" {
			target = "/run/secrets/" + ref.Name
		} else {
			renderedTarget, err := render.RenderTemplateString(engine, data, fmt.Sprintf("secrets.%s.target", ref.Name), target)
			if err != nil {
				return nil, fmt.Errorf("service %q secret %q: %w", scope.Service, ref.Name, err)
			}
			target = templates.ExpandPathTokens(renderedTarget, scope)
		}
		modeValue, err := render.RenderTemplateString(engine, data, fmt.Sprintf("secrets.%s.mode", ref.Name), firstNonEmpty(ref.Mode, def.Mode))
		if err != nil {
			return nil, fmt.Errorf("service %q secret %q: %w", scope.Service, ref.Name, err)
		}
		mode, err := parseFileMode(modeValue)
		if err != nil {
			return nil, fmt.Errorf("service %q secret %q: %w", scope.Service, ref.Name, err)
		}
		uid, err := render.RenderTemplateString(engine, data, fmt.Sprintf("secrets.%s.uid", ref.Name), firstNonEmpty(ref.UID, def.UID))
		if err != nil {
			return nil, fmt.Errorf("service %q secret %q: %w", scope.Service, ref.Name, err)
		}
		gid, err := render.RenderTemplateString(engine, data, fmt.Sprintf("secrets.%s.gid", ref.Name), firstNonEmpty(ref.GID, def.GID))
		if err != nil {
			return nil, fmt.Errorf("service %q secret %q: %w", scope.Service, ref.Name, err)
		}
		out = append(out, ServiceMount{
			Name:   physical,
			Target: target,
			UID:    uid,
			GID:    gid,
			Mode:   mode,
		})
	}
	return out, nil
}

func envSlice(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+env[key])
	}
	return out
}

func desiredVolumeMounts(cfg *config.Config, engine *templates.Engine, data render.TemplateData, stackName string, stack config.Stack, partitionName string, serviceName string, service config.Service) ([]mount.Mount, error) {
	if len(service.Volumes) == 0 {
		return nil, nil
	}
	scope := templates.Scope{
		Project:    cfg.Project.Name,
		Deployment: cfg.Project.Deployment,
		Stack:      stackName,
		Partition:  partitionName,
		Service:    serviceName,
	}
	serviceStandard := config.ServiceStandardName(cfg)
	serviceTarget := config.ServiceTarget(cfg)
	basePath := strings.TrimSpace(cfg.Project.Defaults.Volumes.BasePath)
	if basePath != "" {
		var err error
		basePath, err = render.RenderTemplateString(engine, data, "volume.base_path", basePath)
		if err != nil {
			return nil, err
		}
	}
	out := make([]mount.Mount, 0, len(service.Volumes))
	for _, ref := range service.Volumes {
		if ref.Name != "" {
			volumeDef, ok := stack.Volumes[ref.Name]
			if basePath == "" {
				return nil, fmt.Errorf("service %q volume %q: project.defaults.volumes.base_path is required", serviceName, ref.Name)
			}
			target, err := render.RenderTemplateString(engine, data, fmt.Sprintf("volumes.%s.target", ref.Name), firstNonEmpty(ref.Target, volumeDef.Target))
			if err != nil {
				return nil, err
			}
			if target == "" {
				return nil, fmt.Errorf("service %q volume %q: target is required", serviceName, ref.Name)
			}
			target = templates.ExpandPathTokens(target, scope)
			defSubpath, err := render.RenderTemplateString(engine, data, fmt.Sprintf("volumes.%s.subpath", ref.Name), volumeDef.Subpath)
			if err != nil {
				return nil, err
			}
			refSubpath, err := render.RenderTemplateString(engine, data, fmt.Sprintf("volumes.%s.subpath", ref.Name), ref.Subpath)
			if err != nil {
				return nil, err
			}
			source := config.StackVolumeSource(basePath, cfg.Project.Name, stackName, stack.Mode, partitionName, serviceName, ref.Name, defSubpath, refSubpath, ok)
			out = append(out, mount.Mount{
				Type:     mount.TypeBind,
				Source:   source,
				Target:   target,
				ReadOnly: ref.ReadOnly,
			})
			continue
		}
		if ref.Standard != "" {
			if ref.Standard == serviceStandard {
				if basePath == "" {
					return nil, fmt.Errorf("service %q volume %q: project.defaults.volumes.base_path is required", serviceName, ref.Standard)
				}
				target, err := render.RenderTemplateString(engine, data, "volumes.standard.target", firstNonEmpty(ref.Target, serviceTarget))
				if err != nil {
					return nil, err
				}
				if target == "" {
					return nil, fmt.Errorf("service %q volume %q: target is required", serviceName, ref.Standard)
				}
				target = templates.ExpandPathTokens(target, scope)
				subpath, err := render.RenderTemplateString(engine, data, "volumes.standard.subpath", ref.Subpath)
				if err != nil {
					return nil, err
				}
				source := config.StackVolumeSource(basePath, cfg.Project.Name, stackName, stack.Mode, partitionName, serviceName, "", "", subpath, false)
				out = append(out, mount.Mount{
					Type:     mount.TypeBind,
					Source:   source,
					Target:   target,
					ReadOnly: ref.ReadOnly,
				})
				continue
			}
			standard, ok := cfg.Project.Defaults.Volumes.Standards[ref.Standard]
			if !ok {
				return nil, fmt.Errorf("service %q volume %q: standard mount not found", serviceName, ref.Standard)
			}
			standardSource, err := render.RenderTemplateString(engine, data, fmt.Sprintf("volumes.standard.%s.source", ref.Standard), standard.Source)
			if err != nil {
				return nil, err
			}
			standardTarget, err := render.RenderTemplateString(engine, data, fmt.Sprintf("volumes.standard.%s.target", ref.Standard), standard.Target)
			if err != nil {
				return nil, err
			}
			out = append(out, mount.Mount{
				Type:     mount.TypeBind,
				Source:   standardSource,
				Target:   standardTarget,
				ReadOnly: standard.ReadOnly,
			})
			continue
		}
		source, err := render.RenderTemplateString(engine, data, "volumes.bind.source", ref.Source)
		if err != nil {
			return nil, err
		}
		target, err := render.RenderTemplateString(engine, data, "volumes.bind.target", ref.Target)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(ref.Source) == "" || strings.TrimSpace(ref.Target) == "" {
			return nil, fmt.Errorf("service %q volume: source and target are required for ad-hoc binds", serviceName)
		}
		out = append(out, mount.Mount{
			Type:     mount.TypeBind,
			Source:   source,
			Target:   templates.ExpandPathTokens(target, scope),
			ReadOnly: ref.ReadOnly,
		})
	}
	sort.Slice(out, func(i, j int) bool { return volumeMountLess(out[i], out[j]) })
	return out, nil
}

func desiredPlacementConstraints(cfg *config.Config, stackName string, stack config.Stack, partitionName string, serviceName string, service config.Service) []string {
	var constraints []string
	constraints = append(constraints, service.Placement.Constraints...)
	labelKey := strings.TrimSpace(cfg.Project.Defaults.Volumes.NodeLabelKey)
	if labelKey == "" {
		return uniqueSortedConstraints(constraints)
	}
	serviceStandard := config.ServiceStandardName(cfg)
	for _, ref := range service.Volumes {
		switch {
		case ref.Name != "":
			constraints = append(constraints, volumeConstraint(labelKey, ref.Name))
		case ref.Standard == serviceStandard:
			volumeName := serviceScopedVolumeName(stackName, stack.Mode, partitionName, serviceName)
			constraints = append(constraints, volumeConstraint(labelKey, volumeName))
		}
	}
	return uniqueSortedConstraints(constraints)
}

func volumeConstraint(labelKey, volume string) string {
	return fmt.Sprintf("node.labels.%s.%s == true", labelKey, volume)
}

func serviceScopedVolumeName(stackName string, stackMode string, partition string, serviceName string) string {
	resolved := config.ResolvedStackName(stackName, stackMode, partition)
	return resolved + "." + serviceName
}

func uniqueSortedConstraints(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		unique[value] = struct{}{}
	}
	out := make([]string, 0, len(unique))
	for value := range unique {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func volumeMountLess(a, b mount.Mount) bool {
	if a.Type != b.Type {
		return a.Type < b.Type
	}
	if a.Source != b.Source {
		return a.Source < b.Source
	}
	if a.Target != b.Target {
		return a.Target < b.Target
	}
	if a.ReadOnly != b.ReadOnly {
		return !a.ReadOnly && b.ReadOnly
	}
	return false
}

func portIntents(ports []config.Port) ([]portIntent, error) {
	if len(ports) == 0 {
		return nil, nil
	}
	out := make([]portIntent, 0, len(ports))
	for _, port := range ports {
		protocol := dockerapi.PortConfigProtocolTCP
		if port.Protocol == "udp" {
			protocol = dockerapi.PortConfigProtocolUDP
		}
		mode := dockerapi.PortConfigPublishModeIngress
		if port.Mode == "host" {
			mode = dockerapi.PortConfigPublishModeHost
		}
		out = append(out, portIntent{
			Target:    uint32(port.Target),
			Published: uint32(port.Published),
			Protocol:  protocol,
			Mode:      mode,
		})
	}
	sort.Slice(out, func(i, j int) bool { return portIntentLess(out[i], out[j]) })
	return out, nil
}

func portIntentsFromSpec(spec *dockerapi.EndpointSpec) []portIntent {
	if spec == nil || len(spec.Ports) == 0 {
		return nil
	}
	out := make([]portIntent, 0, len(spec.Ports))
	for _, port := range spec.Ports {
		out = append(out, portIntent{
			Target:    port.TargetPort,
			Published: port.PublishedPort,
			Protocol:  port.Protocol,
			Mode:      port.PublishMode,
		})
	}
	sort.Slice(out, func(i, j int) bool { return portIntentLess(out[i], out[j]) })
	return out
}

func portIntentsEqual(left []portIntent, right []portIntent) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func portIntentLess(a, b portIntent) bool {
	if a.Target != b.Target {
		return a.Target < b.Target
	}
	if a.Published != b.Published {
		return a.Published < b.Published
	}
	if a.Protocol != b.Protocol {
		return a.Protocol < b.Protocol
	}
	return a.Mode < b.Mode
}

func parseHealthcheck(raw map[string]any) (*container.HealthConfig, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	data, err := yaml.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var cfg container.HealthConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func healthcheckEqual(left, right *container.HealthConfig) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	if !stringSlicesEqual(left.Test, right.Test) {
		return false
	}
	if left.Interval != right.Interval {
		return false
	}
	if left.Timeout != right.Timeout {
		return false
	}
	if left.StartPeriod != right.StartPeriod {
		return false
	}
	if left.StartInterval != right.StartInterval {
		return false
	}
	if left.Retries != right.Retries {
		return false
	}
	return true
}

func stringSlicesEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func cloneStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, len(items))
	copy(out, items)
	return out
}

func cloneMounts(items []mount.Mount) []mount.Mount {
	if len(items) == 0 {
		return nil
	}
	out := make([]mount.Mount, len(items))
	copy(out, items)
	return out
}

func applyPorts(spec *dockerapi.EndpointSpec, ports []portIntent) *dockerapi.EndpointSpec {
	if spec == nil {
		spec = &dockerapi.EndpointSpec{}
	}
	if len(ports) == 0 {
		spec.Ports = nil
		return spec
	}
	out := make([]dockerapi.PortConfig, 0, len(ports))
	for _, port := range ports {
		out = append(out, dockerapi.PortConfig{
			TargetPort:    port.Target,
			PublishedPort: port.Published,
			Protocol:      port.Protocol,
			PublishMode:   port.Mode,
		})
	}
	spec.Ports = out
	return spec
}

func applyMode(mode dockerapi.ServiceMode, desired string, replicas uint64) dockerapi.ServiceMode {
	if desired == "global" {
		mode.Global = &dockerapi.GlobalService{}
		mode.Replicated = nil
		return mode
	}
	mode.Replicated = &dockerapi.ReplicatedService{Replicas: &replicas}
	mode.Global = nil
	return mode
}

func modeFromSpec(mode dockerapi.ServiceMode) (string, uint64) {
	if mode.Global != nil {
		return "global", 0
	}
	if mode.Replicated != nil && mode.Replicated.Replicas != nil {
		return "replicated", *mode.Replicated.Replicas
	}
	return "replicated", 0
}

func volumeMountsEqual(left, right []mount.Mount) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func normalizeMounts(mounts []mount.Mount) []mount.Mount {
	if len(mounts) == 0 {
		return nil
	}
	out := make([]mount.Mount, 0, len(mounts))
	for _, item := range mounts {
		out = append(out, mount.Mount{
			Type:     item.Type,
			Source:   item.Source,
			Target:   item.Target,
			ReadOnly: item.ReadOnly,
		})
	}
	return out
}

func applyNetworks(current []dockerapi.NetworkAttachmentConfig, networks []string) []dockerapi.NetworkAttachmentConfig {
	if len(networks) == 0 {
		return nil
	}
	out := make([]dockerapi.NetworkAttachmentConfig, 0, len(networks))
	for _, name := range networks {
		out = append(out, dockerapi.NetworkAttachmentConfig{Target: name})
	}
	return out
}

func networksFromSpec(current []dockerapi.NetworkAttachmentConfig, targets map[string]string) []string {
	if len(current) == 0 {
		return nil
	}
	out := make([]string, 0, len(current))
	for _, net := range current {
		target := net.Target
		if name, ok := targets[target]; ok {
			target = name
		}
		if target != "" && target != "ingress" {
			out = append(out, target)
		}
	}
	sort.Strings(out)
	return sliceutil.DedupeSortedStrings(out)
}

func normalizeServiceImage(image string, labels map[string]string) string {
	if labels == nil {
		return image
	}
	if stackImage, ok := labels["com.docker.stack.image"]; ok && stackImage != "" {
		return stackImage
	}
	return image
}

func filterServiceLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	out := make(map[string]string, len(labels))
	for key, value := range labels {
		if strings.HasPrefix(key, "com.docker.") {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func configMountsFromRefs(refs []*dockerapi.ConfigReference) []ServiceMount {
	out := make([]ServiceMount, 0, len(refs))
	for _, ref := range refs {
		if ref == nil || ref.File == nil {
			continue
		}
		out = append(out, ServiceMount{
			Name:   ref.ConfigName,
			Target: ref.File.Name,
			UID:    ref.File.UID,
			GID:    ref.File.GID,
			Mode:   ref.File.Mode,
		})
	}
	return out
}

func secretMountsFromRefs(refs []*dockerapi.SecretReference) []ServiceMount {
	out := make([]ServiceMount, 0, len(refs))
	for _, ref := range refs {
		if ref == nil || ref.File == nil {
			continue
		}
		out = append(out, ServiceMount{
			Name:   ref.SecretName,
			Target: ref.File.Name,
			UID:    ref.File.UID,
			GID:    ref.File.GID,
			Mode:   ref.File.Mode,
		})
	}
	return out
}

func mountSlicesEqual(left []ServiceMount, right []ServiceMount) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func mountLess(a, b ServiceMount) bool {
	if a.Name != b.Name {
		return a.Name < b.Name
	}
	if a.Target != b.Target {
		return a.Target < b.Target
	}
	if a.UID != b.UID {
		return a.UID < b.UID
	}
	if a.GID != b.GID {
		return a.GID < b.GID
	}
	return a.Mode < b.Mode
}

func normalizeServiceMounts(mounts []ServiceMount) []ServiceMount {
	if len(mounts) == 0 {
		return nil
	}
	out := make([]ServiceMount, 0, len(mounts))
	for _, mount := range mounts {
		out = append(out, normalizeServiceMount(mount))
	}
	return out
}

func canonicalizeIntentForCompare(intent serviceIntent) serviceIntent {
	intent.Env = canonicalizeStringSlice(intent.Env)                 // env order does not change container environment.
	intent.Constraints = canonicalizeStringSlice(intent.Constraints) // placement constraints are unordered selectors.
	intent.Networks = canonicalizeStringSlice(intent.Networks)       // network attachments are unordered.
	intent.Ports = canonicalizePortIntents(intent.Ports)             // published ports are unordered in the spec.
	intent.Configs = canonicalizeServiceMounts(intent.Configs)       // config mounts are unordered.
	intent.Secrets = canonicalizeServiceMounts(intent.Secrets)       // secret mounts are unordered.
	intent.Volumes = canonicalizeVolumeMounts(intent.Volumes)        // volume mounts are unordered.
	return intent
}

func canonicalizeStringSlice(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := cloneStrings(items)
	sort.Strings(out)
	return out
}

func canonicalizePortIntents(items []portIntent) []portIntent {
	if len(items) == 0 {
		return nil
	}
	out := make([]portIntent, len(items))
	copy(out, items)
	sort.Slice(out, func(i, j int) bool { return portIntentLess(out[i], out[j]) })
	return out
}

func canonicalizeServiceMounts(mounts []ServiceMount) []ServiceMount {
	if len(mounts) == 0 {
		return nil
	}
	out := normalizeServiceMounts(mounts)
	sort.Slice(out, func(i, j int) bool { return mountLess(out[i], out[j]) })
	return out
}

func canonicalizeVolumeMounts(mounts []mount.Mount) []mount.Mount {
	if len(mounts) == 0 {
		return nil
	}
	out := normalizeMounts(mounts)
	sort.Slice(out, func(i, j int) bool { return volumeMountLess(out[i], out[j]) })
	return out
}

func normalizeServiceMount(mount ServiceMount) ServiceMount {
	if mount.UID == "" {
		mount.UID = "0"
	}
	if mount.GID == "" {
		mount.GID = "0"
	}
	if mount.Mode == 0 {
		mount.Mode = 0o444
	}
	return mount
}

type serviceKey struct {
	project   string
	stack     string
	partition string
	service   string
}

func (k serviceKey) labelKey() string {
	partition := k.partition
	if partition == "" {
		partition = "none"
	}
	return k.project + "|" + k.stack + "|" + partition + "|" + k.service
}

func serviceFullName(project, stack, partition, service string) string {
	if partition == "" {
		return fmt.Sprintf("%s_%s_%s", project, stack, service)
	}
	return fmt.Sprintf("%s_%s_%s_%s", project, partition, stack, service)
}

func serviceLabels(scope templates.Scope, serviceName string, userLabels map[string]string, resolver templates.Resolver, data render.TemplateData) (map[string]string, error) {
	managed := render.Labels(scope, serviceName, "")
	if managed[render.LabelHash] == "" {
		delete(managed, render.LabelHash)
	}
	if len(userLabels) == 0 {
		return managed, nil
	}
	engine := templates.New(resolver)
	rendered, err := renderLabelTemplates(engine, data, scope, userLabels)
	if err != nil {
		return nil, fmt.Errorf("service %q labels: %w", serviceName, err)
	}
	merged := cloneLabels(rendered)
	for key, value := range managed {
		merged[key] = value
	}
	return merged, nil
}

func cloneLabels(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func renderLabelTemplates(engine *templates.Engine, data render.TemplateData, scope templates.Scope, labels map[string]string) (map[string]string, error) {
	rendered := make(map[string]string, len(labels))
	for key, value := range labels {
		expandedKey := templates.ExpandTokens(key, scope)
		if strings.TrimSpace(expandedKey) == "" {
			return nil, fmt.Errorf("label %q: key is empty after token expansion", key)
		}
		if strings.HasPrefix(expandedKey, "swarmcp.io/") {
			return nil, fmt.Errorf("label %q: key uses reserved prefix swarmcp.io/", expandedKey)
		}
		if _, ok := rendered[expandedKey]; ok {
			return nil, fmt.Errorf("label %q: duplicate key after token expansion", expandedKey)
		}
		result, err := engine.Render("label:"+expandedKey, value, data)
		if err != nil {
			return nil, fmt.Errorf("label %q: %w", expandedKey, err)
		}
		rendered[expandedKey] = result
	}
	return rendered, nil
}

func labelsEqual(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func stringSlicesEqualSorted(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	if len(left) == 0 {
		return true
	}
	leftCopy := cloneStrings(left)
	rightCopy := cloneStrings(right)
	sort.Strings(leftCopy)
	sort.Strings(rightCopy)
	return stringSlicesEqual(leftCopy, rightCopy)
}

func applyPlacement(current *dockerapi.Placement, constraints []string) *dockerapi.Placement {
	if len(constraints) == 0 {
		return nil
	}
	return &dockerapi.Placement{
		Constraints: cloneStrings(constraints),
	}
}

func indexServices(services []swarm.Service, projectName string) map[string]swarm.Service {
	out := make(map[string]swarm.Service)
	for _, svc := range services {
		if !isManagedProject(svc.Labels, projectName) {
			continue
		}
		stack := svc.Labels[render.LabelStack]
		service := svc.Labels[render.LabelService]
		partition := svc.Labels[render.LabelPartition]
		if stack == "" || service == "" || partition == "" {
			continue
		}
		key := serviceKey{
			project:   projectName,
			stack:     stack,
			partition: partition,
			service:   service,
		}
		out[key.labelKey()] = svc
	}
	return out
}

func parseFileMode(raw string) (os.FileMode, error) {
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseUint(raw, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid file mode %q", raw)
	}
	return os.FileMode(value), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func updateService(ctx context.Context, client swarm.Client, update ServiceUpdate, configIDs map[string]string, secretIDs map[string]string) error {
	spec := update.Spec
	if spec.TaskTemplate.ContainerSpec == nil {
		spec.TaskTemplate.ContainerSpec = &dockerapi.ContainerSpec{}
	}

	configRefs, err := buildConfigRefs(update.Service.Name, update.Configs, configIDs)
	if err != nil {
		return err
	}
	secretRefs, err := buildSecretRefs(update.Service.Name, update.Secrets, secretIDs)
	if err != nil {
		return err
	}

	spec.TaskTemplate.ContainerSpec.Configs = configRefs
	spec.TaskTemplate.ContainerSpec.Secrets = secretRefs

	return client.UpdateService(ctx, update.Service, spec)
}

func createService(ctx context.Context, client swarm.Client, create ServiceCreate, configIDs map[string]string, secretIDs map[string]string) error {
	spec := create.Spec
	if spec.TaskTemplate.ContainerSpec == nil {
		spec.TaskTemplate.ContainerSpec = &dockerapi.ContainerSpec{}
	}
	configRefs, err := buildConfigRefs(create.Name, create.Configs, configIDs)
	if err != nil {
		return err
	}
	secretRefs, err := buildSecretRefs(create.Name, create.Secrets, secretIDs)
	if err != nil {
		return err
	}
	spec.TaskTemplate.ContainerSpec.Configs = configRefs
	spec.TaskTemplate.ContainerSpec.Secrets = secretRefs
	_, err = client.CreateService(ctx, spec)
	return err
}

func buildConfigRefs(serviceName string, mounts []ServiceMount, configIDs map[string]string) ([]*dockerapi.ConfigReference, error) {
	configRefs := make([]*dockerapi.ConfigReference, 0, len(mounts))
	for _, serviceMount := range mounts {
		id, ok := configIDs[serviceMount.Name]
		if !ok {
			return nil, fmt.Errorf("service %q config %q: id not found", serviceName, serviceMount.Name)
		}
		configRefs = append(configRefs, &dockerapi.ConfigReference{
			ConfigID:   id,
			ConfigName: serviceMount.Name,
			File: &dockerapi.ConfigReferenceFileTarget{
				Name: serviceMount.Target,
				UID:  serviceMount.UID,
				GID:  serviceMount.GID,
				Mode: serviceMount.Mode,
			},
		})
	}

	return configRefs, nil
}

func buildSecretRefs(serviceName string, mounts []ServiceMount, secretIDs map[string]string) ([]*dockerapi.SecretReference, error) {
	secretRefs := make([]*dockerapi.SecretReference, 0, len(mounts))
	for _, serviceMount := range mounts {
		id, ok := secretIDs[serviceMount.Name]
		if !ok {
			return nil, fmt.Errorf("service %q secret %q: id not found", serviceName, serviceMount.Name)
		}
		secretRefs = append(secretRefs, &dockerapi.SecretReference{
			SecretID:   id,
			SecretName: serviceMount.Name,
			File: &dockerapi.SecretReferenceFileTarget{
				Name: serviceMount.Target,
				UID:  serviceMount.UID,
				GID:  serviceMount.GID,
				Mode: serviceMount.Mode,
			},
		})
	}

	return secretRefs, nil
}
