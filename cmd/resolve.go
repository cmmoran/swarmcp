package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"
)

var (
	resolveOutput string
	resolvePath   string
)

var resolveCmd = &cobra.Command{
	Use:   "resolve",
	Short: "Print the resolved config model",
	RunE: func(cmd *cobra.Command, args []string) error {
		configPaths, err := effectiveProjectConfigPaths()
		if err != nil {
			return err
		}
		deployment, err := singleSelector("deployment", opts.Deployments)
		if err != nil {
			return err
		}
		partition, err := singleSelector("partition", opts.Partitions)
		if err != nil {
			return err
		}
		stack, err := singleSelector("stack", opts.Stacks)
		if err != nil {
			return err
		}
		output, err := normalizeResolveOutput(resolveOutput)
		if err != nil {
			return err
		}

		resolvedModel, err := config.LoadResolvedModel(config.ResolvedModelOptions{
			ConfigPaths:        configPaths,
			ReleaseConfigPaths: effectiveReleaseConfigPaths(),
			Deployment:         deployment,
			Partition:          partition,
			Stack:              stack,
			LoadOptions:        config.LoadOptions{Offline: opts.Offline, Debug: opts.Debug},
		})
		if err != nil {
			return err
		}
		value := any(resolvedModel.Model)
		if strings.TrimSpace(resolvePath) != "" {
			value, err = config.LookupResolvedPath(resolvedModel.Model, resolvePath)
			if err != nil {
				return err
			}
		}
		return writeResolvedValue(cmd.OutOrStdout(), value, output)
	},
}

func init() {
	resolveCmd.Flags().StringVar(&resolveOutput, "output", "yaml", "Resolved model output format: yaml|json")
	resolveCmd.Flags().StringVar(&resolvePath, "path", "", "Field path within the resolved model to print")
}

func normalizeResolveOutput(value string) (string, error) {
	output := strings.ToLower(strings.TrimSpace(value))
	if output == "" {
		output = "yaml"
	}
	switch output {
	case "yaml", "json":
		return output, nil
	default:
		return "", fmt.Errorf("invalid --output %q (expected yaml or json)", value)
	}
}

func writeResolvedValue(out io.Writer, value any, output string) error {
	var data []byte
	var err error
	switch output {
	case "json":
		data, err = json.MarshalIndent(value, "", "  ")
	case "yaml":
		data, err = yaml.Marshal(value)
	default:
		return fmt.Errorf("unsupported output format %q", output)
	}
	if err != nil {
		return err
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	_, err = out.Write(data)
	return err
}
