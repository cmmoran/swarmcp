package docker

import (
	"os"
	"sort"
	"strconv"

	dnetwork "github.com/docker/docker/api/types/network"
	dswarm "github.com/docker/docker/api/types/swarm"
	units "github.com/docker/go-units"

	"github.com/cmmoran/swarmcp/internal/model"
)

// ConfigSpec converts a rendered config into a Docker ConfigSpec.
func ConfigSpec(cfg model.RenderedConfig) dswarm.ConfigSpec {
	return dswarm.ConfigSpec{
		Annotations: dswarm.Annotations{
			Name:   cfg.Spec.Name,
			Labels: cfg.Spec.Labels,
		},
		Data: cfg.Data,
	}
}

// SecretSpec converts a rendered secret into a Docker SecretSpec.
func SecretSpec(sec model.RenderedSecret) dswarm.SecretSpec {
	return dswarm.SecretSpec{
		Annotations: dswarm.Annotations{
			Name:   sec.Spec.Name,
			Labels: sec.Spec.Labels,
		},
		Data: sec.Data,
	}
}

// ServiceSpec converts a rendered service into a Docker ServiceSpec.
func ServiceSpec(svc model.RenderedService) dswarm.ServiceSpec {
	env := make([]string, 0, len(svc.Spec.Env))
	for k, v := range svc.Spec.Env {
		env = append(env, k+"="+v)
	}
	sort.Strings(env)

	netAtts := make([]dswarm.NetworkAttachmentConfig, 0, len(svc.Spec.Networks))
	for _, n := range svc.Spec.Networks {
		netAtts = append(netAtts, dswarm.NetworkAttachmentConfig{Target: n})
	}

	mode := dswarm.ServiceMode{}
	if svc.Spec.Deployment.Replicas > 0 {
		mode.Replicated = &dswarm.ReplicatedService{Replicas: ptr(uint64(svc.Spec.Deployment.Replicas))}
	}

	container := &dswarm.ContainerSpec{Image: svc.Spec.Image, Env: env}

	// Config mounts
	if len(svc.Spec.Configs) > 0 {
		cfgRefs := make([]*dswarm.ConfigReference, 0, len(svc.Spec.Configs))
		for _, cfg := range svc.Spec.Configs {
			cfgRefs = append(cfgRefs, &dswarm.ConfigReference{
				ConfigName: cfg.Name,
				File: &dswarm.ConfigReferenceFileTarget{
					Name: cfg.Target.Target,
					UID:  cfg.Target.UID,
					GID:  cfg.Target.GID,
					Mode: os.FileMode(derefMode(cfg.Target.Mode, 0444)),
				},
			})
		}
		container.Configs = cfgRefs
	}

	// Secret mounts
	if len(svc.Spec.Secrets) > 0 {
		secRefs := make([]*dswarm.SecretReference, 0, len(svc.Spec.Secrets))
		for _, sec := range svc.Spec.Secrets {
			secRefs = append(secRefs, &dswarm.SecretReference{
				SecretName: sec.Name,
				File: &dswarm.SecretReferenceFileTarget{
					Name: sec.Target.Target,
					UID:  sec.Target.UID,
					GID:  sec.Target.GID,
					Mode: os.FileMode(derefMode(sec.Target.Mode, 0400)),
				},
			})
		}
		container.Secrets = secRefs
	}

	placement := &dswarm.Placement{}
	if len(svc.Spec.Deployment.Constraints) > 0 {
		placement.Constraints = append([]string{}, svc.Spec.Deployment.Constraints...)
	}

	// Resources
	resources := &dswarm.ResourceRequirements{}
	resources.Limits = nanoResourceLimits(svc.Spec.Deployment.Resources.Limits)
	resources.Reservations = nanoResourceReservations(svc.Spec.Deployment.Resources.Reservations)
	if resources.Limits == nil && resources.Reservations == nil {
		resources = nil
	}

	return dswarm.ServiceSpec{
		Annotations: dswarm.Annotations{Name: svc.Spec.Name, Labels: svc.Spec.Labels},
		Mode:        mode,
		TaskTemplate: dswarm.TaskSpec{
			ContainerSpec: container,
			Placement:     placement,
			Resources:     resources,
			Networks:      netAtts,
		},
	}
}

// NetworkSpec converts the model shape into a Docker network create request.
func NetworkSpec(name string, driver string, internal bool, labels map[string]string) dnetwork.CreateOptions {
	return dnetwork.CreateOptions{
		Driver:     driver,
		Internal:   internal,
		Attachable: true,
		Labels:     labels,
		Scope:      "swarm",
	}
}

func ptr[T any](v T) *T { return &v }

func derefMode(mode *uint32, def uint32) uint32 {
	if mode == nil {
		return def
	}
	return *mode
}

func nanoResourceLimits(r model.CPUMem) *dswarm.Limit {
	nano, _ := parseNanoCPUs(r.CPUs)
	bytes, _ := units.RAMInBytes(r.Memory)
	if nano == 0 && bytes == 0 {
		return nil
	}
	return &dswarm.Limit{NanoCPUs: nano, MemoryBytes: bytes}
}

func nanoResourceReservations(r model.CPUMem) *dswarm.Resources {
	nano, _ := parseNanoCPUs(r.CPUs)
	bytes, _ := units.RAMInBytes(r.Memory)
	if nano == 0 && bytes == 0 {
		return nil
	}
	return &dswarm.Resources{NanoCPUs: nano, MemoryBytes: bytes}
}

func parseNanoCPUs(in string) (int64, error) {
	if in == "" {
		return 0, nil
	}
	cpus, err := strconv.ParseFloat(in, 64)
	if err != nil {
		return 0, err
	}
	return int64(cpus * 1e9), nil
}
