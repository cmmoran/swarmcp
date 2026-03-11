package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cmmoran/swarmcp/internal/cmdutil"
	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/render"
	"github.com/cmmoran/swarmcp/internal/secrets"
	"github.com/cmmoran/swarmcp/internal/templates"
	"github.com/spf13/cobra"
)

var secretsCmd = &cobra.Command{
	Use:   "secrets",
	Short: "Inspect or update secret values",
}

var secretsCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Report missing secrets required by templates",
	RunE: func(cmd *cobra.Command, args []string) error {
		configPaths, err := effectiveProjectConfigPaths()
		if err != nil {
			return err
		}
		releaseConfigPaths := effectiveReleaseConfigPaths()
		configPath := configPaths[0]
		deployment, err := singleSelector("deployment", opts.Deployments)
		if err != nil {
			return err
		}
		partitionFilter, err := singleSelector("partition", opts.Partitions)
		if err != nil {
			return err
		}
		cfg, err := config.LoadFilesWithReleaseOptions(configPaths, releaseConfigPaths, config.LoadOptions{Offline: opts.Offline, Debug: opts.Debug})
		if err != nil {
			return err
		}
		config.SetBaseDir(cfg, configPath)
		if deployment != "" {
			cfg.Project.Deployment = deployment
		}
		if err := config.ValidateDeployment(cfg); err != nil {
			return err
		}
		if partitionFilter != "" && !cmdutil.PartitionInProject(cfg, partitionFilter) {
			return fmt.Errorf("partition %q not found in project.partitions", partitionFilter)
		}
		cmdutil.ConfigureTemplateNetworkResolver(cmdutil.ResolveContext(cfg, opts.Context))

		useEngine := cfg.Project.SecretsEngine != nil && opts.SecretsFile == ""
		secretsFile := ""
		if opts.SecretsFile != "" {
			secretsFile = opts.SecretsFile
		} else if !useEngine {
			secretsFile = cmdutil.InferSecretsFile(cfg, configPath, opts.SecretsFile)
		}
		store, err := cmdutil.LoadSecretsStore(secretsFile)
		if err != nil {
			return err
		}
		valuesFiles := cmdutil.InferValuesFiles(configPath, opts.ValuesFiles)
		valuesScope := templates.Scope{
			Project:        cfg.Project.Name,
			Deployment:     cfg.Project.Deployment,
			Partition:      partitionFilter,
			NetworksShared: config.NetworksSharedString(cfg, partitionFilter),
		}
		values, err := cmdutil.LoadValuesStore(valuesFiles, valuesScope)
		if err != nil {
			return err
		}
		partitionFilters := normalizeSelectors(opts.Partitions)
		summary, err := render.RenderProject(cfg, store, values, partitionFilters, nil, true, !opts.NoInfer)
		if err != nil {
			return err
		}

		out := cmd.OutOrStdout()
		printSecretsCheckSources(out, secretsFile, cfg, useEngine)
		if len(summary.MissingSecrets) == 0 {
			_, _ = fmt.Fprintln(out, "secrets check OK\nmissing secrets: 0")
			return nil
		}
		_, _ = fmt.Fprintf(out, "secrets check OK\nmissing secrets: %d\n", len(summary.MissingSecrets))
		for _, item := range summary.MissingSecrets {
			command, ok := formatSecretsPutCommand(item)
			if !ok {
				_, _ = fmt.Fprintf(out, "  - %s\n", item)
				continue
			}
			_, _ = fmt.Fprintf(out, "  - %s\n", command)
		}
		return nil
	},
}

var (
	secretsPutFromFile  string
	secretsPutStdin     bool
	secretsPutStack     string
	secretsPutService   string
	secretsPutPartition string
)

var secretsPutCmd = &cobra.Command{
	Use:   "put <name> [value]",
	Short: "Write a secret value to the secrets file or engine",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		configPaths, err := effectiveProjectConfigPaths()
		if err != nil {
			return err
		}
		releaseConfigPaths := effectiveReleaseConfigPaths()
		configPath := configPaths[0]
		deployment, err := singleSelector("deployment", opts.Deployments)
		if err != nil {
			return err
		}
		cfg, err := config.LoadFilesWithReleaseOptions(configPaths, releaseConfigPaths, config.LoadOptions{Offline: opts.Offline, Debug: opts.Debug})
		if err != nil {
			return err
		}
		config.SetBaseDir(cfg, configPath)
		if deployment != "" {
			cfg.Project.Deployment = deployment
		}
		if err := config.ValidateDeployment(cfg); err != nil {
			return err
		}
		if secretsPutPartition != "" && !cmdutil.PartitionInProject(cfg, secretsPutPartition) {
			return fmt.Errorf("partition %q not found in project.partitions", secretsPutPartition)
		}
		if secretsPutService != "" && secretsPutStack == "" {
			return fmt.Errorf("service requires --stack")
		}
		if secretsPutStack != "" {
			stack, ok := cfg.Stacks[secretsPutStack]
			if !ok {
				return fmt.Errorf("stack %q not found", secretsPutStack)
			}
			if secretsPutService != "" {
				if _, ok := stack.Services[secretsPutService]; !ok {
					return fmt.Errorf("service %q not found in stack %q", secretsPutService, secretsPutStack)
				}
			}
			if stack.Mode == "partitioned" && secretsPutPartition == "" {
				return fmt.Errorf("stack %q is partitioned; --partition is required", secretsPutStack)
			}
		}

		name := args[0]
		value := ""
		if len(args) > 1 {
			value = args[1]
		}
		if value == "" && secretsPutFromFile != "" {
			data, err := os.ReadFile(secretsPutFromFile)
			if err != nil {
				return err
			}
			value = string(data)
		}
		if value == "" && secretsPutStdin {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
			value = string(data)
		}
		value = strings.TrimRight(value, "\n")
		if value == "" {
			return fmt.Errorf("secret value is required (arg, --from-file, or --stdin)")
		}

		if opts.SecretsFile != "" {
			store, err := cmdutil.LoadOrInitSecretsStore(opts.SecretsFile)
			if err != nil {
				return err
			}
			store.Values[name] = value
			if err := secrets.Save(opts.SecretsFile, store); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "secrets put OK (file=%s)\n", opts.SecretsFile)
			return nil
		}

		if cfg.Project.SecretsEngine != nil {
			writer, err := secrets.NewWriter(cfg)
			if err != nil {
				if errors.Is(err, secrets.ErrSecretWriteUnsupported) {
					return fmt.Errorf("secrets write unsupported for configured secrets_engine")
				}
				return err
			}
			scope := templates.Scope{
				Project:        cfg.Project.Name,
				Deployment:     cfg.Project.Deployment,
				Stack:          secretsPutStack,
				Partition:      secretsPutPartition,
				Service:        secretsPutService,
				NetworksShared: config.NetworksSharedString(cfg, secretsPutPartition),
			}
			if err := writer.Put(scope, name, value); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "secrets put OK")
			return nil
		}

		secretsFile := cmdutil.InferSecretsFile(cfg, configPath, opts.SecretsFile)
		if secretsFile == "" {
			return fmt.Errorf("secrets file is required when secrets_engine is not configured")
		}
		store, err := cmdutil.LoadOrInitSecretsStore(secretsFile)
		if err != nil {
			return err
		}
		store.Values[name] = value
		if err := secrets.Save(secretsFile, store); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "secrets put OK (file=%s)\n", secretsFile)
		return nil
	},
}

func init() {
	secretsPutCmd.Flags().StringVar(&secretsPutFromFile, "from-file", "", "Read secret value from a file")
	secretsPutCmd.Flags().BoolVar(&secretsPutStdin, "stdin", false, "Read secret value from stdin")
	secretsPutCmd.Flags().StringVar(&secretsPutStack, "stack", "", "Stack name for scoped secrets engine writes")
	secretsPutCmd.Flags().StringVar(&secretsPutPartition, "partition", "", "Partition name for scoped secrets engine writes")
	secretsPutCmd.Flags().StringVar(&secretsPutService, "service", "", "Service name for scoped secrets engine writes")

	secretsCmd.AddCommand(secretsCheckCmd)
	secretsCmd.AddCommand(secretsPutCmd)
}

func printSecretsCheckSources(out io.Writer, secretsFile string, cfg *config.Config, useEngine bool) {
	var parts []string
	if secretsFile != "" {
		parts = append(parts, fmt.Sprintf("secrets file=%s", secretsFile))
	}
	if useEngine && cfg.Project.SecretsEngine != nil && cfg.Project.SecretsEngine.Provider != "" {
		parts = append(parts, fmt.Sprintf("secrets engine=%s", cfg.Project.SecretsEngine.Provider))
	}
	if len(parts) == 0 {
		_, _ = fmt.Fprintln(out, "secrets sources: none")
		return
	}
	_, _ = fmt.Fprintf(out, "secrets sources: %s\n", strings.Join(parts, ", "))
}

type missingSecretScope struct {
	Project   string
	Stack     string
	Partition string
	Service   string
	Name      string
}

func parseMissingSecret(value string) (missingSecretScope, bool) {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return missingSecretScope{}, false
	}
	scope := missingSecretScope{}
	for _, field := range fields {
		parts := strings.SplitN(field, "=", 2)
		if len(parts) != 2 {
			return missingSecretScope{}, false
		}
		switch parts[0] {
		case "project":
			scope.Project = parts[1]
		case "stack":
			scope.Stack = parts[1]
		case "partition":
			scope.Partition = parts[1]
		case "service":
			scope.Service = parts[1]
		case "name":
			scope.Name = parts[1]
		default:
			return missingSecretScope{}, false
		}
	}
	if scope.Name == "" {
		return missingSecretScope{}, false
	}
	return scope, true
}

func formatSecretsPutCommand(value string) (string, bool) {
	scope, ok := parseMissingSecret(value)
	if !ok {
		return "", false
	}
	args := []string{"swarmcp"}
	for _, path := range normalizeConfigPaths(opts.ConfigPaths) {
		if path == "" {
			continue
		}
		if path == "swarmcp.yaml" {
			continue
		}
		args = append(args, "--config", path)
	}
	if opts.SecretsFile != "" {
		args = append(args, "--secrets-file", opts.SecretsFile)
	}
	if deployment := normalizeSelectors(opts.Deployments); len(deployment) == 1 {
		args = append(args, "--deployment", deployment[0])
	}
	args = append(args, "secrets", "put", scope.Name, "--stdin")
	if scope.Stack != "" && scope.Stack != "none" {
		args = append(args, "--stack", scope.Stack)
	}
	if scope.Partition != "" && scope.Partition != "none" {
		args = append(args, "--partition", scope.Partition)
	}
	if scope.Service != "" && scope.Service != "none" {
		args = append(args, "--service", scope.Service)
	}
	return strings.Join(args, " "), true
}
