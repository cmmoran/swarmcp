package reconcile

import (
	"github.com/docker/docker/api/types/swarm"
)

func minServiceSpec(name string, image string, env []string, networks []string, replicas int) swarm.ServiceSpec {
	netAtts := make([]swarm.NetworkAttachmentConfig, 0, len(networks))
	for _, n := range networks {
		netAtts = append(netAtts, swarm.NetworkAttachmentConfig{Target: n})
	}
	mode := swarm.ServiceMode{}
	if replicas > 0 {
		mode.Replicated = &swarm.ReplicatedService{Replicas: uint64Ptr(uint64(replicas))}
	}
	return swarm.ServiceSpec{
		Annotations: swarm.Annotations{Name: name},
		Mode:        mode,
		TaskTemplate: swarm.TaskSpec{
			ContainerSpec: &swarm.ContainerSpec{
				Image: image,
				Env:   env,
			},
			Networks: netAtts,
		},
	}
}

func uint64Ptr(u uint64) *uint64 {
	return &u
}
