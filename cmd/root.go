package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var (
	driver      string
	projectPath string
	rootCmd     = &cobra.Command{
		Use:   "swarmcp",
		Short: "a simple swarm control plane",
	}
)

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&projectPath, "project", "p", "project.example", "Path to project root")
	rootCmd.PersistentFlags().StringVar(&driver, "driver", "noop", "Backend driver: docker|noop")
}
