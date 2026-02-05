package cmd

import (
	"fmt"

	"github.com/cmmoran/swarmcp/internal/apply"
	"github.com/cmmoran/swarmcp/internal/cmdutil"
	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/spf13/cobra"
)

func resolvePreserve(cmd *cobra.Command, cfg *config.Config, opts Options) (int, error) {
	preserve := config.PreserveUnusedResources(cfg)
	if flag := cmd.Flags().Lookup("preserve"); flag != nil && flag.Changed {
		preserve = opts.Preserve
	}
	if preserve < 0 {
		return 0, fmt.Errorf("preserve must be >= 0")
	}
	return preserve, nil
}

func confirmPruneConfigs(cmd *cobra.Command, plan apply.Plan, preserve int, confirm bool) (bool, error) {
	if !confirm {
		return true, nil
	}
	message := fmt.Sprintf("Prune unused configs/secrets? configs=%d secrets=%d preserve=%d", len(plan.DeleteConfigs), len(plan.DeleteSecrets), preserve)
	return cmdutil.ConfirmPrompt(cmd.InOrStdin(), cmd.OutOrStdout(), message)
}

func confirmPruneServices(cmd *cobra.Command, stackDeploys int, confirm bool) (bool, error) {
	if !confirm {
		return true, nil
	}
	message := fmt.Sprintf("Prune removed services via stack deploy --prune? stacks=%d", stackDeploys)
	return cmdutil.ConfirmPrompt(cmd.InOrStdin(), cmd.OutOrStdout(), message)
}
