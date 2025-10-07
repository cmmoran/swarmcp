package specnorm

// Helpers to build order-independent, minimal shapes of a service spec
// for stable fingerprinting and diffing.
//
// We avoid raw json.Marshal of maps because Go map iteration order is
// randomized. Instead, convert maps/slices into sorted slices.

import (
	"sort"

	"github.com/cmmoran/swarmcp/internal/manifest"
)

// NormalizedService is an order-independent projection of manifest.ServiceSpec.
type NormalizedService struct {
	Image        string   `json:"image"`
	Replicas     int      `json:"replicas"`
	Networks     []string `json:"networks,omitempty"`
	Env          []KV     `json:"env,omitempty"`
	Labels       []KV     `json:"labels,omitempty"`
	Configs      []string `json:"configs,omitempty"`
	Secrets      []string `json:"secrets,omitempty"`
	Constraints  []string `json:"constraints,omitempty"`
	Limits       CPUMem   `json:"limits,omitempty"`
	Reservations CPUMem   `json:"reservations,omitempty"`
}

type KV struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type CPUMem struct {
	CPUs   string `json:"cpus,omitempty"`
	Memory string `json:"memory,omitempty"`
}

func FromManifestSpec(s manifest.ServiceSpec) NormalizedService {
	n := NormalizedService{
		Image:       s.Image.Repo + ":" + s.Image.Tag,
		Replicas:    s.Deploy.Replicas,
		Constraints: append([]string{}, s.Deploy.Placement.Constraints...),
		Limits: CPUMem{
			CPUs:   s.Deploy.Resources.Limits.CPUs,
			Memory: s.Deploy.Resources.Limits.Memory,
		},
		Reservations: CPUMem{
			CPUs:   s.Deploy.Resources.Reservations.CPUs,
			Memory: s.Deploy.Resources.Reservations.Memory,
		},
	}

	// Networks
	for _, natt := range s.Networks {
		n.Networks = append(n.Networks, natt.Name)
	}
	sort.Strings(n.Networks)

	// Env
	for _, e := range s.Env {
		n.Env = append(n.Env, KV{Key: e.Name, Value: e.Value})
	}
	sort.Slice(n.Env, func(i, j int) bool { return n.Env[i].Key < n.Env[j].Key })

	// Labels
	if len(s.Labels) > 0 {
		keys := make([]string, 0, len(s.Labels))
		for k := range s.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			n.Labels = append(n.Labels, KV{Key: k, Value: s.Labels[k]})
		}
	}

	// Configs / Secrets by declared name (not content)
	for _, c := range s.Configs {
		n.Configs = append(n.Configs, c.Name)
	}
	sort.Strings(n.Configs)
	for _, sec := range s.Secrets {
		n.Secrets = append(n.Secrets, sec.Name)
	}
	sort.Strings(n.Secrets)

	return n
}
