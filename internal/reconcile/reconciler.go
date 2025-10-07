package reconcile

import (
	"context"

	"github.com/infamousity/swarmcp/internal/diff"
	"github.com/infamousity/swarmcp/internal/manifest"
	"github.com/infamousity/swarmcp/internal/status"
	"github.com/infamousity/swarmcp/internal/swarm"
	"github.com/infamousity/swarmcp/internal/vault"
)

type Reconciler struct {
	swarm swarm.Client
	vault vault.Client
}

func New(s swarm.Client, v vault.Client) *Reconciler { return &Reconciler{swarm: s, vault: v} }

func (r *Reconciler) Plan(ctx context.Context, eff *manifest.EffectiveProject) (*diff.Plan, error) {
	_ = ctx
	_ = eff
	// MVP: return empty plan
	return diff.New(), nil
}

func (r *Reconciler) Apply(ctx context.Context, pl *diff.Plan) (*status.Report, error) {
	_ = ctx
	_ = pl
	rep := &status.Report{}
	// MVP: do nothing
	return rep, nil
}
