package cmd

import (
	"fmt"
	"io"

	"github.com/cmmoran/swarmcp/internal/cmdutil"
)

type runtimeTargetOptions struct {
	includeValues  bool
	includeSecrets bool
}

type runtimeTargets struct {
	configPaths        []string
	releaseConfigPaths []string
	configPath         string
	deployments        []string
	partitionFilters   []string
	stackFilters       []string
}

type runtimeTarget struct {
	configPaths        []string
	releaseConfigPaths []string
	configPath         string
	deployment         string
	partitionFilters   []string
	stackFilters       []string
	projectCtx         *cmdutil.ProjectContext
}

func prepareRuntimeTargets() (*runtimeTargets, error) {
	configPaths, err := effectiveProjectConfigPaths()
	if err != nil {
		return nil, err
	}
	return &runtimeTargets{
		configPaths:        configPaths,
		releaseConfigPaths: effectiveReleaseConfigPaths(),
		configPath:         configPaths[0],
		deployments:        deploymentTargets(opts.Deployments),
		partitionFilters:   normalizeSelectors(opts.Partitions),
		stackFilters:       normalizeSelectors(opts.Stacks),
	}, nil
}

func forEachRuntimeTarget(out io.Writer, targets *runtimeTargets, opts runtimeTargetOptions, fn func(runtimeTarget) error) error {
	for deploymentIndex, deployment := range targets.deployments {
		if len(targets.deployments) > 1 {
			if deploymentIndex > 0 {
				_, _ = fmt.Fprintln(out)
			}
			label := deployment
			if label == "" {
				label = "(default)"
			}
			_, _ = fmt.Fprintf(out, "target deployment: %s\n", label)
		}

		projectCtx, err := loadValidatedProjectContext(targets, deployment, opts)
		if err != nil {
			return err
		}
		if err := fn(runtimeTarget{
			configPaths:        targets.configPaths,
			releaseConfigPaths: targets.releaseConfigPaths,
			configPath:         targets.configPath,
			deployment:         deployment,
			partitionFilters:   targets.partitionFilters,
			stackFilters:       targets.stackFilters,
			projectCtx:         projectCtx,
		}); err != nil {
			return err
		}
	}
	return nil
}

func loadValidatedProjectContext(targets *runtimeTargets, deployment string, loadOpts runtimeTargetOptions) (*cmdutil.ProjectContext, error) {
	projectOpts := cmdutil.ProjectOptions{
		ConfigPaths:        targets.configPaths,
		ReleaseConfigPaths: targets.releaseConfigPaths,
		ConfigPath:         targets.configPath,
		SecretsFile:        opts.SecretsFile,
		ValuesFiles:        opts.ValuesFiles,
		Deployment:         deployment,
		Context:            opts.Context,
		Offline:            opts.Offline,
		Debug:              opts.Debug,
	}
	cfg, configPath, err := cmdutil.LoadProjectConfig(projectOpts)
	if err != nil {
		return nil, err
	}
	scope, err := cmdutil.ResolveProjectScope(cfg, projectOpts)
	if err != nil {
		return nil, err
	}
	projectCtx := cmdutil.NewProjectContext(cfg, scope, projectOpts)
	if err := cmdutil.LoadProjectInputs(projectCtx, configPath, projectOpts, loadOpts.includeValues, loadOpts.includeSecrets); err != nil {
		return nil, err
	}
	for _, partition := range targets.partitionFilters {
		if !cmdutil.PartitionInProject(cfg, partition) {
			return nil, fmt.Errorf("partition %q not found in project.partitions", partition)
		}
	}
	for _, stack := range targets.stackFilters {
		if !cmdutil.StackInProject(cfg, stack) {
			return nil, fmt.Errorf("stack %q not found in stacks", stack)
		}
	}
	return projectCtx, nil
}
