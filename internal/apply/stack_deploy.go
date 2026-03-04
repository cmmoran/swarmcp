package apply

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/sliceutil"
	"github.com/docker/docker/api/types/mount"
	"gopkg.in/yaml.v3"
)

type StackDeploy struct {
	Name           string
	Compose        []byte
	ServiceCreates int
	ServiceUpdates int
}

type composeFile struct {
	Version  string                     `yaml:"version"`
	Services map[string]composeService  `yaml:"services"`
	Networks map[string]composeNetwork  `yaml:"networks,omitempty"`
	Configs  map[string]composeExternal `yaml:"configs,omitempty"`
	Secrets  map[string]composeExternal `yaml:"secrets,omitempty"`
	Volumes  map[string]composeVolume   `yaml:"volumes,omitempty"`
}

type composeService struct {
	Image       string             `yaml:"image"`
	Entrypoint  []string           `yaml:"entrypoint,omitempty"`
	Command     []string           `yaml:"command,omitempty"`
	WorkingDir  string             `yaml:"working_dir,omitempty"`
	Environment map[string]string  `yaml:"environment,omitempty"`
	Ports       []composePort      `yaml:"ports,omitempty"`
	Configs     []composeConfigRef `yaml:"configs,omitempty"`
	Secrets     []composeSecretRef `yaml:"secrets,omitempty"`
	Volumes     []composeMount     `yaml:"volumes,omitempty"`
	Networks    []string           `yaml:"networks,omitempty"`
	Healthcheck map[string]any     `yaml:"healthcheck,omitempty"`
	Deploy      *composeDeploy     `yaml:"deploy,omitempty"`
}

type composePort struct {
	Target    int    `yaml:"target"`
	Published *int   `yaml:"published,omitempty"`
	Protocol  string `yaml:"protocol,omitempty"`
	Mode      string `yaml:"mode,omitempty"`
}

type composeConfigRef struct {
	Source string `yaml:"source"`
	Target string `yaml:"target,omitempty"`
	UID    string `yaml:"uid,omitempty"`
	GID    string `yaml:"gid,omitempty"`
	Mode   string `yaml:"mode,omitempty"`
}

type composeSecretRef struct {
	Source string `yaml:"source"`
	Target string `yaml:"target,omitempty"`
	UID    string `yaml:"uid,omitempty"`
	GID    string `yaml:"gid,omitempty"`
	Mode   string `yaml:"mode,omitempty"`
}

type composeMount struct {
	Type     string `yaml:"type"`
	Source   string `yaml:"source"`
	Target   string `yaml:"target"`
	ReadOnly bool   `yaml:"read_only,omitempty"`
}

type composeNetwork struct {
	External   bool   `yaml:"external,omitempty"`
	Name       string `yaml:"name,omitempty"`
	Attachable bool   `yaml:"attachable,omitempty"`
	Internal   bool   `yaml:"internal,omitempty"`
}

type composeDeploy struct {
	Mode           string                `yaml:"mode,omitempty"`
	Replicas       *int                  `yaml:"replicas,omitempty"`
	Labels         map[string]string     `yaml:"labels,omitempty"`
	Placement      *composePlacement     `yaml:"placement,omitempty"`
	RestartPolicy  *composeRestartPolicy `yaml:"restart_policy,omitempty"`
	UpdateConfig   *composeUpdateConfig  `yaml:"update_config,omitempty"`
	RollbackConfig *composeUpdateConfig  `yaml:"rollback_config,omitempty"`
}

type composeExternal struct {
	External bool   `yaml:"external"`
	Name     string `yaml:"name,omitempty"`
}

type composePlacement struct {
	Constraints []string `yaml:"constraints,omitempty"`
}

type composeVolume struct {
	Driver     string            `yaml:"driver,omitempty"`
	DriverOpts map[string]string `yaml:"driver_opts,omitempty"`
}

func BuildStackDeploys(cfg *config.Config, desired DesiredState, values any, partitionFilters []string, stackFilters []string, filter map[string]struct{}, creates []ServiceCreate, updates []ServiceUpdate, infer bool) ([]StackDeploy, error) {
	index := buildDefIndex(desired.Defs)
	var deploys []StackDeploy
	changes := map[string]struct {
		creates int
		updates int
	}{}
	for _, create := range creates {
		if stack, ok := cfg.Stacks[create.Stack]; ok {
			name := config.StackInstanceName(cfg.Project.Name, create.Stack, create.Partition, stack.Mode)
			entry := changes[name]
			entry.creates++
			changes[name] = entry
		}
	}
	for _, update := range updates {
		if stack, ok := cfg.Stacks[update.Stack]; ok {
			name := config.StackInstanceName(cfg.Project.Name, update.Stack, update.Partition, stack.Mode)
			entry := changes[name]
			entry.updates++
			changes[name] = entry
		}
	}

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
				return nil, err
			}
			if len(services) == 0 {
				continue
			}
			deployName := config.StackInstanceName(cfg.Project.Name, stackName, partitionName, stack.Mode)
			if filter != nil {
				if _, ok := filter[deployName]; !ok {
					continue
				}
			}
			compose := composeFile{
				Version:  "3.9",
				Services: make(map[string]composeService),
			}
			configs := make(map[string]composeExternal)
			secrets := make(map[string]composeExternal)
			networks := make(map[string]composeNetwork)
			volumes := make(map[string]composeVolume)
			for serviceName, service := range services {
				build, err := buildServiceIntent(cfg, stackName, stack, partitionName, serviceName, service, values, infer, index)
				if err != nil {
					return nil, err
				}
				renderedService := build.Rendered
				configMounts := build.ConfigMounts
				secretMounts := build.SecretMounts
				volumeMounts := build.VolumeMounts
				if err := addComposeVolumes(cfg, stackName, stack, partitionName, serviceName, renderedService, volumes); err != nil {
					return nil, err
				}
				externalNetworks := desiredServiceExternalNetworks(cfg, stackName, stack.Mode, partitionName, serviceName, renderedService)
				serviceNetworks := append([]string(nil), externalNetworks...)
				if renderedService.NetworkEphemeral != nil {
					ephemeralKey := config.EphemeralNetworkKey(serviceName)
					if ephemeralKey != "" {
						serviceNetworks = append(serviceNetworks, ephemeralKey)
						attachable, internal := ephemeralNetworkSettings(renderedService.NetworkEphemeral)
						networks[ephemeralKey] = composeNetwork{
							Attachable: attachable,
							Internal:   internal,
						}
					}
				}

				deploySpec, err := composeDeploySpec(renderedService, build.Constraints, build.Labels, renderedService.RestartPolicy, renderedService.UpdateConfig, renderedService.RollbackConfig)
				if err != nil {
					return nil, err
				}
				composeService := composeService{
					Image:       renderedService.Image,
					Entrypoint:  cloneStrings(renderedService.Command),
					Command:     cloneStrings(renderedService.Args),
					WorkingDir:  renderedService.Workdir,
					Environment: renderedService.Env,
					Ports:       composePorts(renderedService.Ports),
					Configs:     composeConfigRefs(configMounts),
					Secrets:     composeSecretRefs(secretMounts),
					Volumes:     composeMounts(volumeMounts),
					Networks:    serviceNetworks,
					Healthcheck: renderedService.Healthcheck,
					Deploy:      deploySpec,
				}
				compose.Services[serviceName] = composeService

				for _, mount := range configMounts {
					configs[mount.Name] = composeExternal{External: true, Name: mount.Name}
				}
				for _, mount := range secretMounts {
					secrets[mount.Name] = composeExternal{External: true, Name: mount.Name}
				}
				for _, name := range externalNetworks {
					networks[name] = composeNetwork{External: true, Name: name}
				}
			}
			if len(configs) > 0 {
				compose.Configs = configs
			}
			if len(secrets) > 0 {
				compose.Secrets = secrets
			}
			if len(networks) > 0 {
				compose.Networks = networks
			}
			if len(volumes) > 0 {
				compose.Volumes = volumes
			}
			raw, err := yaml.Marshal(compose)
			if err != nil {
				return nil, err
			}
			deploy := StackDeploy{
				Name:    deployName,
				Compose: raw,
			}
			if entry, ok := changes[deployName]; ok {
				deploy.ServiceCreates = entry.creates
				deploy.ServiceUpdates = entry.updates
			}
			deploys = append(deploys, deploy)
		}
	}
	return deploys, nil
}

func ValidateDeployOutputMode(mode string) error {
	switch normalizeDeployOutputMode(mode) {
	case "", "auto", "summary", "stack", "error-only":
		return nil
	default:
		return fmt.Errorf("invalid --output %q (expected auto|summary|stack|error-only)", mode)
	}
}

func DeployStacks(ctx context.Context, stacks []StackDeploy, contextName string, pruneServices bool, parallel int, noUI bool, outputMode string, outputExplicit bool) error {
	if len(stacks) == 0 {
		return nil
	}
	if parallel < 1 {
		parallel = len(stacks)
		if parallel < 1 {
			parallel = 1
		}
	}

	mode := resolveDeployOutputMode(outputMode, noUI, outputExplicit)

	var once sync.Once
	var firstErr error
	sem := make(chan struct{}, parallel)
	wg := sync.WaitGroup{}
	outputs := make(map[string]string, len(stacks))
	failed := make(map[string]struct{}, len(stacks))
	outputsMu := sync.Mutex{}
	statuses := make(map[string]string, len(stacks))
	for _, deploy := range stacks {
		statuses[deploy.Name] = "queued"
	}
	uiEnabled := mode == "ui"
	countMu := sync.Mutex{}
	queuedCount := len(stacks)
	runningCount := 0
	doneCount := 0
	failedCount := 0
	startedAt := time.Now()

	if mode == "summary" {
		_, _ = fmt.Fprintf(os.Stdout, "deploy start: stacks=%d parallel=%d\n", len(stacks), parallel)
	}

	var ui *stackUI
	if uiEnabled {
		ui = newStackUI(os.Stdout, statuses)
		ui.Render()
	}
	var uiDone chan struct{}
	if uiEnabled {
		uiDone = make(chan struct{})
		go func() {
			ticker := time.NewTicker(1 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					ui.Render()
				case <-uiDone:
					return
				}
			}
		}()
	}
	var heartbeatDone chan struct{}
	if mode == "error-only" {
		heartbeatDone = make(chan struct{})
		go func() {
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					countMu.Lock()
					q := queuedCount
					r := runningCount
					d := doneCount
					f := failedCount
					countMu.Unlock()
					_, _ = fmt.Fprintf(os.Stdout, "deploy in progress: queued=%d running=%d done=%d failed=%d elapsed=%s\n", q, r, d, f, formatDuration(time.Since(startedAt)))
				case <-heartbeatDone:
					return
				}
			}
		}()
	}

	printMutex := sync.Mutex{}
	trackError := func(name string, err error) {
		once.Do(func() { firstErr = fmt.Errorf("stack deploy %q: %w", name, err) })
		if uiEnabled {
			ui.Update(name, "error")
		}
	}

	for _, deploy := range stacks {
		deploy := deploy
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			countMu.Lock()
			queuedCount--
			runningCount++
			countMu.Unlock()
			deployStarted := time.Now()

			if uiEnabled {
				ui.Update(deploy.Name, "running")
			}
			path, err := writeStackCompose(deploy)
			if err != nil {
				countMu.Lock()
				runningCount--
				failedCount++
				countMu.Unlock()
				trackError(deploy.Name, err)
				return
			}
			defer func() { _ = os.Remove(path) }()

			args := []string{"stack", "deploy", "--with-registry-auth", "--detach=false"}
			if pruneServices {
				args = append(args, "--prune")
			}
			args = append(args, "-c", path, deploy.Name)
			if contextName != "" {
				args = append([]string{"--context", contextName}, args...)
			}
			cmd := exec.CommandContext(ctx, "docker", args...)
			var buf bytes.Buffer
			cmd.Stdout = &buf
			cmd.Stderr = &buf
			if err := cmd.Run(); err != nil {
				output := strings.TrimRight(buf.String(), "\n")
				outputsMu.Lock()
				outputs[deploy.Name] = output
				failed[deploy.Name] = struct{}{}
				outputsMu.Unlock()
				countMu.Lock()
				runningCount--
				failedCount++
				countMu.Unlock()
				trackError(deploy.Name, err)
				if mode == "summary" {
					printMutex.Lock()
					_, _ = fmt.Fprintf(os.Stdout, "stack %s: failed %s\n", deploy.Name, formatDuration(time.Since(deployStarted)))
					printMutex.Unlock()
				}
				return
			}
			output := strings.TrimRight(buf.String(), "\n")
			outputsMu.Lock()
			outputs[deploy.Name] = output
			outputsMu.Unlock()
			countMu.Lock()
			runningCount--
			doneCount++
			countMu.Unlock()
			if uiEnabled {
				ui.Update(deploy.Name, "done")
			} else if mode == "stack" {
				printMutex.Lock()
				if output != "" {
					_, _ = fmt.Fprintf(os.Stdout, "stack %s output:\n%s\n", deploy.Name, output)
				}
				printMutex.Unlock()
			} else if mode == "summary" {
				printMutex.Lock()
				_, _ = fmt.Fprintf(os.Stdout, "stack %s: ok %s\n", deploy.Name, formatDuration(time.Since(deployStarted)))
				printMutex.Unlock()
			}
		}()
	}

	wg.Wait()
	if heartbeatDone != nil {
		close(heartbeatDone)
	}
	if uiEnabled {
		close(uiDone)
		ui.Render()
	}
	if mode == "summary" || mode == "error-only" {
		countMu.Lock()
		d := doneCount
		f := failedCount
		countMu.Unlock()
		_, _ = fmt.Fprintf(os.Stdout, "deploy complete: ok=%d failed=%d total=%d duration=%s\n", d, f, len(stacks), formatDuration(time.Since(startedAt)))
	}
	if firstErr != nil {
		for _, deploy := range stacks {
			if _, ok := failed[deploy.Name]; !ok {
				continue
			}
			output := outputs[deploy.Name]
			if output == "" {
				continue
			}
			_, _ = fmt.Fprintf(os.Stdout, "stack %s output:\n%s\n", deploy.Name, output)
		}
		return firstErr
	}
	return nil
}

func resolveDeployOutputMode(mode string, noUI bool, outputExplicit bool) string {
	normalized := normalizeDeployOutputMode(mode)
	if normalized == "" || normalized == "auto" {
		if outputExplicit {
			return "summary"
		}
		if noUI {
			return "stack"
		}
		if stdoutIsTTY() {
			return "ui"
		}
		return "summary"
	}
	return normalized
}

func normalizeDeployOutputMode(mode string) string {
	return strings.ToLower(strings.TrimSpace(mode))
}

type stackUI struct {
	out      *os.File
	order    []string
	statuses map[string]string
	mu       sync.Mutex
	inited   bool
	started  map[string]time.Time
	ended    map[string]time.Time
}

func newStackUI(out *os.File, statuses map[string]string) *stackUI {
	order := make([]string, 0, len(statuses))
	for name := range statuses {
		order = append(order, name)
	}
	sort.Strings(order)
	return &stackUI{
		out:      out,
		order:    order,
		statuses: statuses,
		started:  make(map[string]time.Time),
		ended:    make(map[string]time.Time),
	}
}

func (ui *stackUI) Update(name, status string) {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	if _, ok := ui.statuses[name]; !ok {
		return
	}
	ui.statuses[name] = status
	if status == "running" {
		ui.started[name] = time.Now()
	}
	if status == "done" || status == "error" {
		ui.ended[name] = time.Now()
	}
	ui.renderLocked()
}

func (ui *stackUI) Render() {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	ui.renderLocked()
}

func (ui *stackUI) renderLocked() {
	lines := len(ui.order)
	if lines == 0 {
		return
	}
	if ui.inited {
		_, _ = fmt.Fprintf(ui.out, "\x1b[%dA", lines)
	} else {
		ui.inited = true
	}
	for _, name := range ui.order {
		status := ui.statuses[name]
		elapsed := ui.elapsedString(name, status)
		if elapsed != "" {
			elapsed = " " + elapsed
		}
		_, _ = fmt.Fprintf(ui.out, "\x1b[2K[%s] %s%s\n", status, name, elapsed)
	}
}

func stdoutIsTTY() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func (ui *stackUI) elapsedString(name, status string) string {
	start, ok := ui.started[name]
	if !ok {
		return ""
	}
	switch status {
	case "running":
		return formatDuration(time.Since(start))
	case "done", "error":
		if end, ok := ui.ended[name]; ok {
			return formatDuration(end.Sub(start))
		}
	}
	return ""
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("(%ds)", int(d.Round(time.Second).Seconds()))
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		secs := int(d.Round(time.Second).Seconds()) % 60
		return fmt.Sprintf("(%dm%ds)", mins, secs)
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	return fmt.Sprintf("(%dh%dm)", hours, mins)
}

func writeStackCompose(deploy StackDeploy) (string, error) {
	dir := os.TempDir()
	base := fmt.Sprintf("swarmcp-%s-", deploy.Name)
	file, err := os.CreateTemp(dir, base)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	if _, err := file.Write(deploy.Compose); err != nil {
		return "", err
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	return filepath.Clean(file.Name()), nil
}

func composeDeploySpec(service config.Service, constraints []string, labels map[string]string, restartPolicy *config.RestartPolicy, updatePolicy *config.UpdatePolicy, rollbackPolicy *config.UpdatePolicy) (*composeDeploy, error) {
	mode := strings.TrimSpace(strings.ToLower(service.Mode))
	if mode == "" {
		mode = "replicated"
	}
	deploy := &composeDeploy{
		Mode:   mode,
		Labels: labels,
	}
	if restartPolicy != nil {
		policy, err := composeRestartPolicySpec(restartPolicy)
		if err != nil {
			return nil, err
		}
		deploy.RestartPolicy = policy
	}
	if updatePolicy != nil {
		configSpec, err := composeUpdateConfigSpec(updatePolicy, "update_config")
		if err != nil {
			return nil, err
		}
		deploy.UpdateConfig = configSpec
	}
	if rollbackPolicy != nil {
		configSpec, err := composeUpdateConfigSpec(rollbackPolicy, "rollback_config")
		if err != nil {
			return nil, err
		}
		deploy.RollbackConfig = configSpec
	}
	if mode != "global" {
		replicas := service.Replicas
		deploy.Replicas = &replicas
	}
	if len(constraints) > 0 {
		deploy.Placement = &composePlacement{Constraints: cloneStrings(constraints)}
	}
	return deploy, nil
}

func ephemeralNetworkSettings(spec *config.ServiceNetworkEphemeral) (bool, bool) {
	attachable := true
	internal := false
	if spec == nil {
		return attachable, internal
	}
	if spec.Attachable != nil {
		attachable = *spec.Attachable
	}
	if spec.Internal != nil {
		internal = *spec.Internal
	}
	return attachable, internal
}

func composePorts(ports []config.Port) []composePort {
	if len(ports) == 0 {
		return nil
	}
	out := make([]composePort, 0, len(ports))
	for _, port := range ports {
		entry := composePort{
			Target: port.Target,
		}
		if port.Published != 0 {
			published := port.Published
			entry.Published = &published
		}
		if port.Protocol != "" {
			entry.Protocol = port.Protocol
		}
		if port.Mode != "" {
			entry.Mode = port.Mode
		}
		out = append(out, entry)
	}
	return out
}

func composeConfigRefs(mounts []ServiceMount) []composeConfigRef {
	if len(mounts) == 0 {
		return nil
	}
	out := make([]composeConfigRef, 0, len(mounts))
	for _, mount := range mounts {
		ref := composeConfigRef{
			Source: mount.Name,
			Target: mount.Target,
			UID:    mount.UID,
			GID:    mount.GID,
		}
		if mount.Mode != 0 {
			ref.Mode = fmt.Sprintf("%#o", mount.Mode)
		}
		out = append(out, ref)
	}
	return out
}

func composeSecretRefs(mounts []ServiceMount) []composeSecretRef {
	if len(mounts) == 0 {
		return nil
	}
	out := make([]composeSecretRef, 0, len(mounts))
	for _, mount := range mounts {
		ref := composeSecretRef{
			Source: mount.Name,
			Target: mount.Target,
			UID:    mount.UID,
			GID:    mount.GID,
		}
		if mount.Mode != 0 {
			ref.Mode = fmt.Sprintf("%#o", mount.Mode)
		}
		out = append(out, ref)
	}
	return out
}

func composeMounts(mounts []mount.Mount) []composeMount {
	if len(mounts) == 0 {
		return nil
	}
	out := make([]composeMount, 0, len(mounts))
	for _, mount := range mounts {
		out = append(out, composeMount{
			Type:     string(mount.Type),
			Source:   mount.Source,
			Target:   mount.Target,
			ReadOnly: mount.ReadOnly,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		return out[i].Target < out[j].Target
	})
	return out
}

func addComposeVolumes(cfg *config.Config, stackName string, stack config.Stack, partitionName string, serviceName string, service config.Service, volumes map[string]composeVolume) error {
	if len(service.Volumes) == 0 {
		return nil
	}
	basePath := strings.TrimSpace(cfg.Project.Defaults.Volumes.BasePath)
	for _, ref := range service.Volumes {
		if ref.Name == "" {
			continue
		}
		volumeDef, ok := stack.Volumes[ref.Name]
		if !ok {
			continue
		}
		if basePath == "" {
			return fmt.Errorf("service %q volume %q: project.defaults.volumes.base_path is required", serviceName, ref.Name)
		}
		device := config.StackVolumeSource(
			basePath,
			cfg.Project.Name,
			stackName,
			stack.Mode,
			partitionName,
			serviceName,
			ref.Name,
			volumeDef.Subpath,
			ref.Subpath,
			true,
		)
		if existing, ok := volumes[ref.Name]; ok {
			if existing.DriverOpts != nil && existing.DriverOpts["device"] != device {
				return fmt.Errorf("service %q volume %q: multiple subpaths detected; define subpath on the stack volume", serviceName, ref.Name)
			}
			continue
		}
		volumes[ref.Name] = composeVolume{
			Driver: "local",
			DriverOpts: map[string]string{
				"type":   "none",
				"o":      "bind",
				"device": device,
			},
		}
	}
	return nil
}
