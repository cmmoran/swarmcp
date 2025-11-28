package reconcile

import (
	"context"
	"fmt"

	"github.com/cmmoran/swarmcp/internal/docker"
	"github.com/cmmoran/swarmcp/internal/model"
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
			// configs and secrets as rendered model objects
			renderedConfigs := make([]model.RenderedConfig, 0, len(svc.Configs))
			for _, cfg := range svc.Configs {
				rcfg := model.RenderedConfig{
					Spec: model.ConfigSpec{
						Name:   cfg.Name,
						Labels: map[string]string{"swarmcp.fingerprint": util.Fingerprint(cfg.Data)},
						Target: model.FileTarget{Target: cfg.File.Target, UID: cfg.File.UID, GID: cfg.File.GID, Mode: cfg.File.Mode},
					},
					Data: cfg.Data,
				}
				renderedConfigs = append(renderedConfigs, rcfg)
				pl.Configs = append(pl.Configs, swarm.ConfigPayload{Name: rcfg.Spec.Name, Bytes: rcfg.Data, Labels: rcfg.Spec.Labels})
			}

			renderedSecrets := make([]model.RenderedSecret, 0, len(svc.Secrets))
			for _, sec := range svc.Secrets {
				rsec := model.RenderedSecret{
					Spec: model.SecretSpec{
						Name:   sec.Name,
						Labels: map[string]string{},
						Target: model.FileTarget{Target: sec.File.Target, UID: sec.File.UID, GID: sec.File.GID, Mode: sec.File.Mode},
					},
					Data: sec.Data,
				}
				renderedSecrets = append(renderedSecrets, rsec)
				pl.Secrets = append(pl.Secrets, swarm.SecretPayload{Name: rsec.Spec.Name, Bytes: rsec.Data, Labels: rsec.Spec.Labels})
			}

			// service
			n := specnorm.FromManifestSpec(svc.Service.Spec)
			fp := util.MustFingerprintJSON(n)
			name := formatServiceName(eff.Project.Metadata.Name, st.Stack.Metadata.Name, inst, svc.Name)
			specLabels := map[string]string{"swarmcp.fingerprint": fp}
			if len(svc.Service.Spec.Labels) > 0 {
				specLabels = map[string]string{}
				for k, v := range svc.Service.Spec.Labels {
					specLabels[k] = v
				}
				specLabels["swarmcp.fingerprint"] = fp
			}

			renderedService := model.RenderedService{
				Spec: model.ServiceSpec{
					Name:     name,
					Image:    svc.Service.Spec.Image.Repo + ":" + svc.Service.Spec.Image.Tag,
					Env:      svc.Env,
					Networks: svc.Networks,
					Labels:   specLabels,
					Configs:  extractConfigSpecs(renderedConfigs),
					Secrets:  extractSecretSpecs(renderedSecrets),
					Deployment: model.DeploymentSpec{
						Replicas:    svc.Service.Spec.Deploy.Replicas,
						Constraints: svc.Service.Spec.Deploy.Placement.Constraints,
						Resources: model.Resources{
							Limits:       model.CPUMem{CPUs: svc.Service.Spec.Deploy.Resources.Limits.CPUs, Memory: svc.Service.Spec.Deploy.Resources.Limits.Memory},
							Reservations: model.CPUMem{CPUs: svc.Service.Spec.Deploy.Resources.Reservations.CPUs, Memory: svc.Service.Spec.Deploy.Resources.Reservations.Memory},
						},
					},
				},
				Configs: renderedConfigs,
				Secrets: renderedSecrets,
			}

			if err := model.ValidateRenderedService(renderedService); err != nil {
				return nil, fmt.Errorf("service %s validation failed: %w", name, err)
			}

			serviceSpec := docker.ServiceSpec(renderedService)
			pl.Services = append(pl.Services, swarm.ServiceApply{
				Name:   name,
				Labels: specLabels,
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

func extractConfigSpecs(cfgs []model.RenderedConfig) []model.ConfigSpec {
	specs := make([]model.ConfigSpec, 0, len(cfgs))
	for _, c := range cfgs {
		specs = append(specs, c.Spec)
	}
	return specs
}

func extractSecretSpecs(secs []model.RenderedSecret) []model.SecretSpec {
	specs := make([]model.SecretSpec, 0, len(secs))
	for _, s := range secs {
		specs = append(specs, s.Spec)
	}
	return specs
}
