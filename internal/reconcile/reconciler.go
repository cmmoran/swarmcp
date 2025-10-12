package reconcile

import (
	"context"
	"fmt"
	"os"

	dswarm "github.com/docker/docker/api/types/swarm"

	"github.com/cmmoran/swarmcp/internal/spec"
	"github.com/cmmoran/swarmcp/internal/specnorm"
	"github.com/cmmoran/swarmcp/internal/swarm"
	"github.com/cmmoran/swarmcp/internal/util"
)

type Reconciler struct {
	cli swarm.Client
}

func New(cli swarm.Client) *Reconciler { return &Reconciler{cli: cli} }

type Plan struct {
	Networks []swarm.NetworkSpec   `json:"networks"`
	Configs  []swarm.ConfigPayload `json:"configs"`
	Secrets  []swarm.SecretPayload `json:"secrets"`
	Services []swarm.ServiceApply  `json:"services"`
	Summary  []string              `json:"summary"`
}

func AddMounts(out *dswarm.ServiceSpec, cfgs []spec.EffectiveConfig, secs []spec.EffectiveSecret) {
	if out.TaskTemplate.ContainerSpec == nil {
		out.TaskTemplate.ContainerSpec = &dswarm.ContainerSpec{}
	}

	// Configs
	if len(cfgs) > 0 {
		refs := make([]*dswarm.ConfigReference, 0, len(cfgs))
		for _, c := range cfgs {
			mode := uint32(0444)
			if c.File.Mode != nil {
				mode = *c.File.Mode
			}
			refs = append(refs, &dswarm.ConfigReference{
				ConfigName: c.Name,
				File: &dswarm.ConfigReferenceFileTarget{
					Name: c.File.Target,
					UID:  c.File.UID,
					GID:  c.File.GID,
					Mode: os.FileMode(mode),
				},
			})
		}
		out.TaskTemplate.ContainerSpec.Configs = refs
	}

	// Secrets
	if len(secs) > 0 {
		refs := make([]*dswarm.SecretReference, 0, len(secs))
		for _, s := range secs {
			mode := uint32(0400)
			if s.File.Mode != nil {
				mode = *s.File.Mode
			}
			refs = append(refs, &dswarm.SecretReference{
				SecretName: s.Name,
				File: &dswarm.SecretReferenceFileTarget{
					Name: s.File.Target,
					UID:  s.File.UID,
					GID:  s.File.GID,
					Mode: os.FileMode(mode),
				},
			})
		}
		out.TaskTemplate.ContainerSpec.Secrets = refs
	}
}

func (r *Reconciler) Plan(ctx context.Context, eff *spec.EffectiveProject) (*Plan, error) {
	pl := &Plan{}
	for _, st := range eff.Stacks {
		inst := ""
		if st.Instance != nil {
			inst = st.Instance.Name
		}
		for _, svc := range st.Services {
			// networks (as names)
			for _, n := range svc.Networks {
				pl.Networks = append(pl.Networks, swarm.NetworkSpec{
					Name:   n,
					Driver: "overlay",
					Labels: map[string]string{},
				})
			}
			// configs
			for _, cfg := range svc.Configs {
				pl.Configs = append(pl.Configs, swarm.ConfigPayload{
					Name:  cfg.Name,
					Bytes: cfg.Data,
					Labels: map[string]string{
						"swarmcp.fingerprint": util.Fingerprint(cfg.Data),
					},
				})
			}
			// service
			n := specnorm.FromManifestSpec(svc.Service.Spec)
			fp := util.MustFingerprintJSON(n)
			_ = fp // reserved for label usage later
			name := formatServiceName(eff.Project.Metadata.Name, st.Stack.Metadata.Name, inst, svc.Name)
			serviceSpec := minServiceSpec(name, svc.Service.Spec.Image.Repo+":"+svc.Service.Spec.Image.Tag, svc.EnvDecl(), svc.Networks, svc.Service.Spec.Deploy.Replicas)
			AddMounts(&serviceSpec, svc.Configs, svc.Secrets)
			pl.Services = append(pl.Services, swarm.ServiceApply{
				Name:   name,
				Labels: map[string]string{"swarmcp.fingerprint": fp},
				Spec:   serviceSpec,
			})
		}
	}
	pl.Summary = append(pl.Summary, fmt.Sprintf("networks:%d configs:%d services:%d", len(pl.Networks), len(pl.Configs), len(pl.Services)))
	return pl, nil
}

func formatServiceName(project, stack, instance, service string) string {
	base := "proj." + project + ".stack." + stack
	if instance != "" {
		base += ".inst." + instance
	}
	return base + ".svc." + service
}
