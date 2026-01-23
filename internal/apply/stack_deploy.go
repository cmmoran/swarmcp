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
	"github.com/cmmoran/swarmcp/internal/render"
	"github.com/cmmoran/swarmcp/internal/sliceutil"
	"github.com/cmmoran/swarmcp/internal/templates"
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
	Mode      string            `yaml:"mode,omitempty"`
	Replicas  *int              `yaml:"replicas,omitempty"`
	Labels    map[string]string `yaml:"labels,omitempty"`
	Placement *composePlacement `yaml:"placement,omitempty"`
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

func BuildStackDeploys(cfg *config.Config, desired DesiredState, values any, partitionFilter string, filter map[string]struct{}, creates []ServiceCreate, updates []ServiceUpdate, infer bool) ([]StackDeploy, error) {
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
		partitions := []string{""}
		if stack.Mode == "partitioned" && len(cfg.Project.Partitions) > 0 {
			partitions = sliceutil.FilterPartition(cfg.Project.Partitions, partitionFilter)
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
				networkEphemeral := ""
				if service.NetworkEphemeral != nil {
					networkEphemeral = config.EphemeralNetworkName(cfg, stackName, stack.Mode, partitionName, serviceName)
				}
				scope := templates.Scope{
					Project:          cfg.Project.Name,
					Deployment:       cfg.Project.Deployment,
					Stack:            stackName,
					Partition:        partitionName,
					Service:          serviceName,
					NetworksShared:   config.NetworksSharedString(cfg, partitionName),
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

				renderedService, err := render.RenderServiceTemplates(engine, templateData, service)
				if err != nil {
					return nil, err
				}
				if infer {
					renderedService.Configs = mergeConfigRefs(renderedService.Configs, inferredConfigs)
					renderedService.Secrets = mergeSecretRefs(renderedService.Secrets, inferredSecrets)
					extraConfigs, extraSecrets, err := render.InferTemplateRefDeps(cfg, scope, renderedService.Configs, renderedService.Secrets)
					if err != nil {
						return nil, err
					}
					renderedService.Configs = mergeConfigRefs(renderedService.Configs, extraConfigs)
					renderedService.Secrets = mergeSecretRefs(renderedService.Secrets, extraSecrets)
				}

				configMounts, err := desiredConfigMounts(resolver, engine, templateData, index, scope, renderedService.Configs, infer)
				if err != nil {
					return nil, err
				}
				secretMounts, err := desiredSecretMounts(resolver, engine, templateData, index, scope, renderedService.Secrets, infer)
				if err != nil {
					return nil, err
				}
				volumeMounts, err := desiredVolumeMounts(cfg, engine, templateData, stackName, stack, partitionName, serviceName, renderedService)
				if err != nil {
					return nil, err
				}
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
				constraints := desiredPlacementConstraints(cfg, stackName, stack, partitionName, serviceName, renderedService)
				labels, err := serviceLabels(scope, serviceName, renderedService.Labels, resolver, templateData)
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
					Deploy:      composeDeploySpec(renderedService, constraints, labels),
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

func DeployStacks(ctx context.Context, stacks []StackDeploy, contextName string, pruneServices bool, parallel int, noUI bool) error {
	if len(stacks) == 0 {
		return nil
	}
	if parallel < 1 {
		parallel = 1
	}

	var once sync.Once
	var firstErr error
	sem := make(chan struct{}, parallel)
	wg := sync.WaitGroup{}
	outputs := make(map[string]*bytes.Buffer, len(stacks))
	statuses := make(map[string]string, len(stacks))
	for _, deploy := range stacks {
		statuses[deploy.Name] = "queued"
	}
	uiEnabled := !noUI && stdoutIsTTY()
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

			if uiEnabled {
				ui.Update(deploy.Name, "running")
			}
			path, err := writeStackCompose(deploy)
			if err != nil {
				trackError(deploy.Name, err)
				return
			}
			defer os.Remove(path)

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
				outputs[deploy.Name] = &buf
				trackError(deploy.Name, err)
				return
			}
			outputs[deploy.Name] = &buf
			if uiEnabled {
				ui.Update(deploy.Name, "done")
			} else {
				printMutex.Lock()
				if buf.Len() > 0 {
					fmt.Fprintf(os.Stdout, "stack %s output:\n%s\n", deploy.Name, strings.TrimRight(buf.String(), "\n"))
				}
				printMutex.Unlock()
			}
		}()
	}

	wg.Wait()
	if uiEnabled {
		close(uiDone)
		ui.Render()
	}
	if firstErr != nil && !uiEnabled {
		return firstErr
	}
	if firstErr != nil {
		for _, deploy := range stacks {
			buf := outputs[deploy.Name]
			if buf == nil || buf.Len() == 0 {
				continue
			}
			fmt.Fprintf(os.Stdout, "stack %s output:\n%s\n", deploy.Name, strings.TrimRight(buf.String(), "\n"))
		}
		return firstErr
	}
	return nil
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
		fmt.Fprintf(ui.out, "\x1b[%dA", lines)
	} else {
		ui.inited = true
	}
	for _, name := range ui.order {
		status := ui.statuses[name]
		elapsed := ui.elapsedString(name, status)
		if elapsed != "" {
			elapsed = " " + elapsed
		}
		fmt.Fprintf(ui.out, "\x1b[2K[%s] %s%s\n", status, name, elapsed)
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
	defer file.Close()
	if _, err := file.Write(deploy.Compose); err != nil {
		return "", err
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	return filepath.Clean(file.Name()), nil
}

func composeDeploySpec(service config.Service, constraints []string, labels map[string]string) *composeDeploy {
	mode := strings.TrimSpace(strings.ToLower(service.Mode))
	if mode == "" {
		mode = "replicated"
	}
	deploy := &composeDeploy{
		Mode:   mode,
		Labels: labels,
	}
	if mode != "global" {
		replicas := service.Replicas
		deploy.Replicas = &replicas
	}
	if len(constraints) > 0 {
		deploy.Placement = &composePlacement{Constraints: cloneStrings(constraints)}
	}
	return deploy
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
