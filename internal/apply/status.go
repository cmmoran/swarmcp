package apply

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/render"
	"github.com/cmmoran/swarmcp/internal/sliceutil"
	"github.com/cmmoran/swarmcp/internal/swarm"
	"github.com/cmmoran/swarmcp/internal/templates"
	"github.com/docker/docker/api/types/mount"
	dockerapi "github.com/docker/docker/api/types/swarm"
)

type ServiceState struct {
	Stack         string
	Partition     string
	Service       string
	Missing       bool
	MountsMatch   bool
	IntentMatch   bool
	IntentDiffs   []string
	IntentDetails []IntentDetail
	Unmanaged     []string
	Desired       int
	Running       int
	Health        string
}

type IntentDetail struct {
	Field   string
	Current string
	Desired string
}

type DriftItem struct {
	Name   string
	Labels map[string]string
	Reason string
}

type StatusReport struct {
	MissingConfigs  []swarm.ConfigSpec
	MissingSecrets  []swarm.SecretSpec
	MissingNetworks []swarm.NetworkSpec
	StaleConfigs    []swarm.Config
	StaleSecrets    []swarm.Secret
	DriftConfigs    []DriftItem
	DriftSecrets    []DriftItem
	SkippedDeletes  SkippedDeletes
	Services        []ServiceState
}

func BuildStatus(ctx context.Context, client swarm.Client, cfg *config.Config, desired DesiredState, values any, partitionFilter string, infer bool) (StatusReport, error) {
	existingConfigs, err := client.ListConfigs(ctx)
	if err != nil {
		return StatusReport{}, err
	}
	existingSecrets, err := client.ListSecrets(ctx)
	if err != nil {
		return StatusReport{}, err
	}
	existingServices, err := client.ListServices(ctx)
	if err != nil {
		return StatusReport{}, err
	}
	existingNetworks, err := client.ListNetworks(ctx)
	if err != nil {
		return StatusReport{}, err
	}

	existingConfigNames := make(map[string]struct{}, len(existingConfigs))
	existingConfigByName := make(map[string]swarm.Config, len(existingConfigs))
	configIDs := make(map[string]string, len(existingConfigs))
	for _, cfg := range existingConfigs {
		existingConfigNames[cfg.Name] = struct{}{}
		existingConfigByName[cfg.Name] = cfg
		configIDs[cfg.Name] = cfg.ID
	}
	existingSecretNames := make(map[string]struct{}, len(existingSecrets))
	existingSecretByName := make(map[string]swarm.Secret, len(existingSecrets))
	secretIDs := make(map[string]string, len(existingSecrets))
	for _, sec := range existingSecrets {
		existingSecretNames[sec.Name] = struct{}{}
		existingSecretByName[sec.Name] = sec
		secretIDs[sec.Name] = sec.ID
	}
	networkNames := make(map[string]struct{}, len(existingNetworks))
	networkTargets := buildNetworkTargetIndex(existingNetworks)
	for _, net := range existingNetworks {
		networkNames[net.Name] = struct{}{}
	}

	inUseConfigIDs, inUseSecretIDs := collectInUseIDs(existingServices, configIDs, secretIDs)
	desiredConfigNames := make(map[string]struct{}, len(desired.Configs))
	desiredSecretNames := make(map[string]struct{}, len(desired.Secrets))
	projectName := cfg.Project.Name

	var report StatusReport
	for _, cfg := range desired.Configs {
		desiredConfigNames[cfg.Name] = struct{}{}
		existing, ok := existingConfigByName[cfg.Name]
		if !ok {
			report.MissingConfigs = append(report.MissingConfigs, cfg)
			continue
		}
		if drift := configLabelDrift(cfg.Labels, existing.Labels, projectName); drift != "" {
			report.DriftConfigs = append(report.DriftConfigs, DriftItem{
				Name:   cfg.Name,
				Labels: existing.Labels,
				Reason: drift,
			})
		}
	}
	for _, sec := range desired.Secrets {
		desiredSecretNames[sec.Name] = struct{}{}
		existing, ok := existingSecretByName[sec.Name]
		if !ok {
			report.MissingSecrets = append(report.MissingSecrets, sec)
			continue
		}
		if drift := configLabelDrift(sec.Labels, existing.Labels, projectName); drift != "" {
			report.DriftSecrets = append(report.DriftSecrets, DriftItem{
				Name:   sec.Name,
				Labels: existing.Labels,
				Reason: drift,
			})
		}
	}
	for _, net := range desired.Networks {
		if _, ok := networkNames[net.Name]; !ok {
			report.MissingNetworks = append(report.MissingNetworks, net)
		}
	}
	for _, cfg := range existingConfigs {
		if !isManagedProject(cfg.Labels, projectName) {
			continue
		}
		if _, ok := inUseConfigIDs[cfg.ID]; ok {
			report.SkippedDeletes.Configs++
			continue
		}
		if _, ok := desiredConfigNames[cfg.Name]; ok {
			continue
		}
		report.StaleConfigs = append(report.StaleConfigs, cfg)
	}
	for _, sec := range existingSecrets {
		if !isManagedProject(sec.Labels, projectName) {
			continue
		}
		if _, ok := inUseSecretIDs[sec.ID]; ok {
			report.SkippedDeletes.Secrets++
			continue
		}
		if _, ok := desiredSecretNames[sec.Name]; ok {
			continue
		}
		report.StaleSecrets = append(report.StaleSecrets, sec)
	}

	serviceIndex := indexServices(existingServices, cfg.Project.Name)
	defIndex := buildDefIndex(desired.Defs)
	expectedServices, err := expectedServices(cfg, partitionFilter)
	if err != nil {
		return StatusReport{}, err
	}
	for _, expected := range expectedServices {
		networkEphemeral := ""
		if expected.Service.NetworkEphemeral != nil {
			stack := cfg.Stacks[expected.Stack]
			networkEphemeral = config.EphemeralNetworkName(cfg, expected.Stack, stack.Mode, expected.Partition, expected.Name)
		}
		scope := templates.Scope{
			Project:          cfg.Project.Name,
			Deployment:       cfg.Project.Deployment,
			Stack:            expected.Stack,
			Partition:        expected.Partition,
			Service:          expected.Name,
			NetworksShared:   config.NetworksSharedString(cfg, expected.Partition),
			NetworkEphemeral: networkEphemeral,
		}
		var inferredConfigs map[string]struct{}
		var inferredSecrets map[string]struct{}
		var trace func(templates.TraceCall)
		if infer {
			inferredConfigs = make(map[string]struct{})
			inferredSecrets = make(map[string]struct{})
			trace = func(call templates.TraceCall) {
				switch call.Func {
				case "config_ref":
					inferredConfigs[call.Name] = struct{}{}
				case "secret_ref":
					inferredSecrets[call.Name] = struct{}{}
				}
			}
		}
		resolver, engine, templateData := render.NewServiceTemplateEngine(cfg, scope, values, infer, trace)

		renderedService, err := render.RenderServiceTemplates(engine, templateData, expected.Service)
		if err != nil {
			return StatusReport{}, err
		}
		if infer {
			renderedService.Configs = mergeConfigRefs(renderedService.Configs, inferredConfigs)
			renderedService.Secrets = mergeSecretRefs(renderedService.Secrets, inferredSecrets)
			extraConfigs, extraSecrets, err := render.InferTemplateRefDeps(cfg, scope, renderedService.Configs, renderedService.Secrets)
			if err != nil {
				return StatusReport{}, err
			}
			renderedService.Configs = mergeConfigRefs(renderedService.Configs, extraConfigs)
			renderedService.Secrets = mergeSecretRefs(renderedService.Secrets, extraSecrets)
		}

		configMounts, err := desiredConfigMounts(resolver, engine, templateData, defIndex, scope, renderedService.Configs, infer)
		if err != nil {
			return StatusReport{}, err
		}
		secretMounts, err := desiredSecretMounts(resolver, engine, templateData, defIndex, scope, renderedService.Secrets, infer)
		if err != nil {
			return StatusReport{}, err
		}
		volumeMounts, err := desiredVolumeMounts(cfg, engine, templateData, expected.Stack, cfg.Stacks[expected.Stack], expected.Partition, expected.Name, renderedService)
		if err != nil {
			return StatusReport{}, err
		}
		serviceNetworks := desiredServiceNetworks(cfg, expected.Stack, cfg.Stacks[expected.Stack].Mode, expected.Partition, expected.Name, renderedService)

		key := serviceKey{
			project:   cfg.Project.Name,
			stack:     expected.Stack,
			partition: expected.Partition,
			service:   expected.Name,
		}
		current, ok := serviceIndex[key.labelKey()]
		state := ServiceState{
			Stack:     expected.Stack,
			Partition: expected.Partition,
			Service:   expected.Name,
		}
		if !ok {
			state.Missing = true
			state.Health = "missing"
			report.Services = append(report.Services, state)
			continue
		}

		labels, err := serviceLabels(scope, expected.Name, renderedService.Labels, resolver, templateData)
		if err != nil {
			return StatusReport{}, err
		}
		constraints := desiredPlacementConstraints(cfg, expected.Stack, cfg.Stacks[expected.Stack], expected.Partition, expected.Name, renderedService)
		desiredIntent, err := intentFromConfig(renderedService, labels, constraints, configMounts, secretMounts, volumeMounts, serviceNetworks)
		if err != nil {
			return StatusReport{}, err
		}
		currentIntent := intentFromSpec(current.Spec, networkTargets)
		compareCurrent := canonicalizeIntentForCompare(currentIntent)
		compareDesired := canonicalizeIntentForCompare(desiredIntent)
		state.IntentDiffs = intentDiffs(compareCurrent, compareDesired)
		state.IntentDetails = intentDetails(currentIntent, desiredIntent, state.IntentDiffs)
		state.MountsMatch = mountSlicesEqual(compareCurrent.Configs, compareDesired.Configs) &&
			mountSlicesEqual(compareCurrent.Secrets, compareDesired.Secrets) &&
			volumeMountsEqual(compareCurrent.Volumes, compareDesired.Volumes)
		state.IntentMatch = len(state.IntentDiffs) == 0
		state.Unmanaged = unmanagedSpecDiffs(current.Spec)
		state.Desired, state.Running = serviceStatusCounts(current.Status)
		state.Health = serviceHealth(state.Desired, state.Running)
		report.Services = append(report.Services, state)
	}

	return report, nil
}

func configLabelDrift(expected, actual map[string]string, projectName string) string {
	if len(actual) == 0 {
		return "labels missing"
	}
	if !isManagedProject(actual, projectName) {
		return "unmanaged resource with matching name"
	}
	if expected == nil {
		return ""
	}
	if expected[render.LabelName] != "" && actual[render.LabelName] != expected[render.LabelName] {
		return "logical name label mismatch"
	}
	if expected[render.LabelProject] != "" && actual[render.LabelProject] != expected[render.LabelProject] {
		return "project label mismatch"
	}
	if expected[render.LabelHash] != "" && actual[render.LabelHash] != expected[render.LabelHash] {
		return "hash label mismatch"
	}
	return ""
}

func intentDiffs(current, desired serviceIntent) []string {
	current = canonicalizeIntentForCompare(current)
	desired = canonicalizeIntentForCompare(desired)
	var diffs []string
	if current.Image != desired.Image {
		diffs = append(diffs, "image")
	}
	if current.Workdir != desired.Workdir {
		diffs = append(diffs, "workdir")
	}
	if current.Mode != desired.Mode || current.Replicas != desired.Replicas {
		diffs = append(diffs, "mode/replicas")
	}
	if !labelsEqual(current.Labels, desired.Labels) {
		diffs = append(diffs, "labels")
	}
	if !stringSlicesEqual(current.Constraints, desired.Constraints) {
		diffs = append(diffs, "placement")
	}
	if !stringSlicesEqual(current.Command, desired.Command) {
		diffs = append(diffs, "command")
	}
	if !stringSlicesEqual(current.Args, desired.Args) {
		diffs = append(diffs, "args")
	}
	if !stringSlicesEqual(current.Env, desired.Env) {
		diffs = append(diffs, "env")
	}
	if !portIntentsEqual(current.Ports, desired.Ports) {
		diffs = append(diffs, "ports")
	}
	if !healthcheckEqual(current.Healthcheck, desired.Healthcheck) {
		diffs = append(diffs, "healthcheck")
	}
	if !mountSlicesEqual(current.Configs, desired.Configs) {
		diffs = append(diffs, "configs")
	}
	if !mountSlicesEqual(current.Secrets, desired.Secrets) {
		diffs = append(diffs, "secrets")
	}
	if !volumeMountsEqual(current.Volumes, desired.Volumes) {
		diffs = append(diffs, "volumes")
	}
	if !stringSlicesEqual(current.Networks, desired.Networks) {
		diffs = append(diffs, "networks")
	}
	return diffs
}

func unmanagedSpecDiffs(spec dockerapi.ServiceSpec) []string {
	var diffs []string
	if spec.UpdateConfig != nil {
		diffs = append(diffs, "update_config")
	}
	if spec.RollbackConfig != nil {
		diffs = append(diffs, "rollback_config")
	}
	if spec.TaskTemplate.RestartPolicy != nil {
		diffs = append(diffs, "restart_policy")
	}
	if spec.TaskTemplate.Resources != nil {
		if spec.TaskTemplate.Resources.Limits != nil || spec.TaskTemplate.Resources.Reservations != nil {
			diffs = append(diffs, "resources")
		}
	}
	if spec.TaskTemplate.Placement != nil && len(spec.TaskTemplate.Placement.Preferences) > 0 {
		diffs = append(diffs, "placement_prefs")
	}
	return diffs
}

func intentDetails(current, desired serviceIntent, diffs []string) []IntentDetail {
	if len(diffs) == 0 {
		return nil
	}
	details := make([]IntentDetail, 0, len(diffs))
	for _, diff := range diffs {
		switch diff {
		case "image":
			details = append(details, IntentDetail{Field: diff, Current: current.Image, Desired: desired.Image})
		case "workdir":
			details = append(details, IntentDetail{Field: diff, Current: current.Workdir, Desired: desired.Workdir})
		case "mode/replicas":
			details = append(details, IntentDetail{
				Field:   diff,
				Current: fmt.Sprintf("%s/%d", current.Mode, current.Replicas),
				Desired: fmt.Sprintf("%s/%d", desired.Mode, desired.Replicas),
			})
		case "labels":
			details = append(details, IntentDetail{
				Field:   diff,
				Current: formatLabels(current.Labels),
				Desired: formatLabels(desired.Labels),
			})
		case "placement":
			details = append(details, IntentDetail{
				Field:   diff,
				Current: formatStringSlice(current.Constraints),
				Desired: formatStringSlice(desired.Constraints),
			})
		case "command":
			details = append(details, IntentDetail{
				Field:   diff,
				Current: strings.Join(current.Command, " "),
				Desired: strings.Join(desired.Command, " "),
			})
		case "args":
			details = append(details, IntentDetail{
				Field:   diff,
				Current: strings.Join(current.Args, " "),
				Desired: strings.Join(desired.Args, " "),
			})
		case "env":
			details = append(details, IntentDetail{
				Field:   diff,
				Current: formatStringSlice(current.Env),
				Desired: formatStringSlice(desired.Env),
			})
		case "ports":
			details = append(details, IntentDetail{
				Field:   diff,
				Current: formatPorts(current.Ports),
				Desired: formatPorts(desired.Ports),
			})
		case "healthcheck":
			details = append(details, IntentDetail{
				Field:   diff,
				Current: fmt.Sprintf("%v", current.Healthcheck),
				Desired: fmt.Sprintf("%v", desired.Healthcheck),
			})
		case "configs":
			details = append(details, IntentDetail{
				Field:   diff,
				Current: formatMounts(current.Configs),
				Desired: formatMounts(desired.Configs),
			})
		case "secrets":
			details = append(details, IntentDetail{
				Field:   diff,
				Current: formatMounts(current.Secrets),
				Desired: formatMounts(desired.Secrets),
			})
		case "volumes":
			details = append(details, IntentDetail{
				Field:   diff,
				Current: formatVolumeMounts(current.Volumes),
				Desired: formatVolumeMounts(desired.Volumes),
			})
		case "networks":
			details = append(details, IntentDetail{
				Field:   diff,
				Current: formatStringSliceSorted(current.Networks),
				Desired: formatStringSliceSorted(desired.Networks),
			})
		}
	}
	if len(details) == 0 {
		return nil
	}
	return details
}

func formatStringSlice(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	return "[" + strings.Join(values, ", ") + "]"
}

func formatStringSliceSorted(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	out := append([]string(nil), values...)
	sort.Strings(out)
	return formatStringSlice(out)
}

func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+labels[key])
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func formatMounts(mounts []ServiceMount) string {
	if len(mounts) == 0 {
		return "[]"
	}
	out := normalizeServiceMounts(mounts)
	sort.Slice(out, func(i, j int) bool { return mountLess(out[i], out[j]) })
	parts := make([]string, 0, len(out))
	for _, mount := range out {
		entry := mount.Name
		if mount.Target != "" {
			entry += ":" + mount.Target
		}
		entry += fmt.Sprintf(" uid=%s gid=%s mode=%#o", mount.UID, mount.GID, mount.Mode)
		parts = append(parts, entry)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func formatVolumeMounts(mounts []mount.Mount) string {
	if len(mounts) == 0 {
		return "[]"
	}
	out := append([]mount.Mount(nil), mounts...)
	sort.Slice(out, func(i, j int) bool { return volumeMountLess(out[i], out[j]) })
	parts := make([]string, 0, len(out))
	for _, mount := range out {
		entry := string(mount.Type) + ":" + mount.Source + "->" + mount.Target
		if mount.ReadOnly {
			entry += ":ro"
		}
		parts = append(parts, entry)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func formatPorts(ports []portIntent) string {
	if len(ports) == 0 {
		return "[]"
	}
	out := append([]portIntent(nil), ports...)
	sort.Slice(out, func(i, j int) bool { return portIntentLess(out[i], out[j]) })
	parts := make([]string, 0, len(out))
	for _, port := range out {
		entry := fmt.Sprintf("%d:%d/%s/%s", port.Published, port.Target, port.Protocol, port.Mode)
		parts = append(parts, entry)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

type expectedService struct {
	Stack     string
	Partition string
	Name      string
	Service   config.Service
}

func expectedServices(cfg *config.Config, partitionFilter string) ([]expectedService, error) {
	var services []expectedService
	for stackName, stack := range cfg.Stacks {
		partitions := []string{""}
		if stack.Mode == "partitioned" && len(cfg.Project.Partitions) > 0 {
			partitions = sliceutil.FilterPartition(cfg.Project.Partitions, partitionFilter)
		}
		for _, partitionName := range partitions {
			stackServices, err := cfg.StackServices(stackName, partitionName)
			if err != nil {
				return nil, err
			}
			if len(stackServices) == 0 {
				continue
			}
			for serviceName, service := range stackServices {
				services = append(services, expectedService{
					Stack:     stackName,
					Partition: partitionName,
					Name:      serviceName,
					Service:   service,
				})
			}
		}
	}
	return services, nil
}

func serviceStatusCounts(status *dockerapi.ServiceStatus) (int, int) {
	if status == nil {
		return -1, -1
	}
	return int(status.DesiredTasks), int(status.RunningTasks)
}

func serviceHealth(desired, running int) string {
	if desired < 0 || running < 0 {
		return "unknown"
	}
	if desired == 0 {
		if running == 0 {
			return "disabled"
		}
		return "degraded"
	}
	if running >= desired {
		return "healthy"
	}
	return "degraded"
}
