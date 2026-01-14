package cmd

import (
	"errors"
	"flag"
	"github.com/cmmoran/swarmcp/internal/fsutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var Version = "0.1.0-dev"

var opts Options

var rootCmd = &cobra.Command{
	Use:           "swarmcp",
	Short:         "SwarmCP provisions and manages Docker Swarm resources from YAML",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() {
	cmd, err := rootCmd.ExecuteC()
	if err == nil {
		return
	}
	if cmd == nil {
		cmd = rootCmd
	}
	cmd.PrintErrln(err)
	if shouldShowUsage(err) {
		_ = cmd.Usage()
	}
	os.Exit(1)
}

func init() {
	rootCmd.PersistentFlags().StringVar(&opts.ConfigPath, "config", "", "Path to project config")
	if absFile, err := filepath.Abs(".swarmcp.project"); err == nil {
		if ok := fsutil.FileExists(absFile); len(opts.ConfigPath) == 0 && ok {
			if cfg, cerr := os.ReadFile(".swarmcp.project"); cerr == nil {
				opts.ConfigPath = strings.TrimSpace(string(cfg))
			}
		}
	}
	rootCmd.PersistentFlags().BoolVar(&opts.NoWarnUnmanaged, "no-warn-unmanaged", false, "Suppress warnings for unmanaged resources")
	rootCmd.PersistentFlags().BoolVar(&opts.SkipHealthcheck, "skip-healthcheck", false, "Skip healthcheck requirement (not recommended)")
	rootCmd.PersistentFlags().StringVar(&opts.SecretsFile, "secrets-file", "", "Path to secrets values file (YAML)")
	rootCmd.PersistentFlags().StringArrayVar(&opts.ValuesFiles, "values", nil, "Path to values file (YAML; repeatable)")
	rootCmd.PersistentFlags().StringVar(&opts.Deployment, "deployment", "", "Deployment name (overrides project.deployment)")
	rootCmd.PersistentFlags().StringVar(&opts.Context, "context", "", "Docker context name (overrides project.contexts)")
	rootCmd.PersistentFlags().StringVar(&opts.Partition, "partition", "", "Limit to a single partition")
	rootCmd.PersistentFlags().BoolVar(&opts.AllowMissing, "allow-missing-secrets", false, "Allow missing secrets with placeholder values")
	rootCmd.PersistentFlags().BoolVar(&opts.NoInfer, "no-infer", false, "Disable inferred config/secret mounts and definitions from template refs (only explicitly declared configs/secrets are rendered and mounted)")
	rootCmd.PersistentFlags().BoolVar(&opts.DebugContent, "debug-content", false, "Print rendered config/secret content")
	rootCmd.PersistentFlags().IntVar(&opts.DebugContentMax, "debug-content-max", 0, "Max bytes of rendered content to print (0 for unlimited)")
	rootCmd.PersistentFlags().BoolVar(&opts.Debug, "debug", false, "Enable debug output")
	rootCmd.PersistentFlags().BoolVar(&opts.Prune, "prune", false, "Remove unused managed configs/secrets and prune removed services")
	rootCmd.PersistentFlags().BoolVar(&opts.PruneServices, "prune-services", false, "Prune removed services without touching configs/secrets")
	rootCmd.PersistentFlags().IntVar(&opts.Preserve, "preserve", 0, "Preserve the most recent unused configs/secrets when pruning (0 for none)")
	rootCmd.PersistentFlags().BoolVar(&opts.NoConfirm, "no-confirm", false, "Skip confirmation prompts")
	rootCmd.PersistentFlags().BoolVar(&opts.Offline, "offline", false, "Disable remote fetches; use cached sources only")

	rootCmd.AddCommand(planCmd)
	rootCmd.AddCommand(diffCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(secretsCmd)
	rootCmd.AddCommand(applyCmd)
	rootCmd.AddCommand(bootstrapCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(versionCmd)
}

func shouldShowUsage(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, flag.ErrHelp) {
		return true
	}
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "unknown command "),
		strings.HasPrefix(msg, "unknown flag:"),
		strings.HasPrefix(msg, "unknown shorthand flag:"),
		strings.HasPrefix(msg, "flag needs an argument:"),
		strings.HasPrefix(msg, "invalid argument "),
		strings.Contains(msg, "requires at least "),
		strings.Contains(msg, "accepts at most "),
		strings.Contains(msg, "accepts between "),
		strings.Contains(msg, "accepts ") && strings.Contains(msg, " arg(s)"),
		strings.Contains(msg, "required flag(s) "),
		strings.Contains(msg, "requires --stack"),
		strings.Contains(msg, "--partition is required"),
		strings.HasPrefix(msg, "secret value is required "):
		return true
	default:
		return false
	}
}
