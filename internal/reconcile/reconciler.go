package reconcile

import (
	"context"
	"fmt"
	"sort"

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
			for _, n := range svc.EffectiveNets {
				pl.Networks = append(pl.Networks, swarm.NetworkSpec{
					Name: n, Driver: "overlay",
					Labels: map[string]string{},
				})
			}
			// configs
			for cfgName, bytes := range svc.RenderedConfigs {
				pl.Configs = append(pl.Configs, swarm.ConfigPayload{
					Name: cfgName, Bytes: bytes, Labels: map[string]string{
						"swarmcp.fingerprint": util.Fingerprint(bytes),
					},
				})
			}
			// service
			n := specnorm.FromManifestSpec(svc.Service.Spec)
			fp := util.MustFingerprintJSON(n)
			_ = fp // reserved for label usage later
			name := formatServiceName(eff.Project.Metadata.Name, st.Stack.Metadata.Name, inst, svc.Name)
			env := make([]string, 0, len(svc.EffectiveEnv))
			keys := make([]string, 0, len(svc.EffectiveEnv))
			for k := range svc.EffectiveEnv {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				env = append(env, fmt.Sprintf("%s=%s", k, svc.EffectiveEnv[k]))
			}
			serviceSpec := minServiceSpec(name, svc.Service.Spec.Image.Repo+":"+svc.Service.Spec.Image.Tag, env, svc.EffectiveNets, svc.Service.Spec.Deploy.Replicas)
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
