package cmdutil

import (
	"errors"
	"fmt"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/secrets"
	"github.com/cmmoran/swarmcp/internal/swarm"
	"github.com/cmmoran/swarmcp/internal/templates"
)

type ProjectOptions struct {
	ConfigPaths        []string
	ReleaseConfigPaths []string
	ConfigPath         string
	SecretsFile        string
	ValuesFiles        []string
	Deployment         string
	Context            string
	Partition          string
	Stack              string
	Offline            bool
	Debug              bool
	ClientFactory      func(string) (swarm.Client, error)
}

type ProjectContext struct {
	Config        *config.Config
	Partition     string
	Stack         string
	Values        any
	Secrets       *secrets.Store
	ContextName   string
	ValuesScope   templates.Scope
	clientFactory func(string) (swarm.Client, error)
}

func LoadProjectContext(opts ProjectOptions, includeValues bool, includeSecrets bool) (*ProjectContext, error) {
	configPath := opts.ConfigPath
	if configPath == "" && len(opts.ConfigPaths) > 0 {
		configPath = opts.ConfigPaths[0]
	}
	var (
		cfg *config.Config
		err error
	)
	if len(opts.ConfigPaths) > 0 {
		cfg, err = config.LoadFilesWithReleaseOptions(opts.ConfigPaths, opts.ReleaseConfigPaths, config.LoadOptions{Offline: opts.Offline, Debug: opts.Debug})
	} else if len(opts.ReleaseConfigPaths) > 0 {
		cfg, err = config.LoadFilesWithReleaseOptions([]string{configPath}, opts.ReleaseConfigPaths, config.LoadOptions{Offline: opts.Offline, Debug: opts.Debug})
	} else {
		cfg, err = config.LoadWithOptions(configPath, config.LoadOptions{Offline: opts.Offline, Debug: opts.Debug})
	}
	if err != nil {
		return nil, err
	}
	config.SetBaseDir(cfg, configPath)
	if opts.Deployment != "" {
		cfg.Project.Deployment = opts.Deployment
	}
	if err := config.ValidateDeployment(cfg); err != nil {
		return nil, err
	}

	partition := opts.Partition
	if partition != "" && !PartitionInProject(cfg, partition) {
		return nil, fmt.Errorf("partition %q not found in project.partitions", partition)
	}
	stack := opts.Stack
	if stack != "" && !StackInProject(cfg, stack) {
		return nil, fmt.Errorf("stack %q not found in stacks", stack)
	}

	contextName := ResolveContext(cfg, opts.Context)
	ConfigureTemplateNetworkResolver(contextName)

	ctx := &ProjectContext{
		Config:      cfg,
		Partition:   partition,
		Stack:       stack,
		ContextName: contextName,
		ValuesScope: templates.Scope{
			Project:        cfg.Project.Name,
			Deployment:     cfg.Project.Deployment,
			Partition:      partition,
			NetworksShared: config.NetworksSharedString(cfg, partition),
		},
		clientFactory: opts.ClientFactory,
	}

	if includeSecrets {
		secretsFile := InferSecretsFile(cfg, configPath, opts.SecretsFile)
		store, err := LoadSecretsStore(secretsFile)
		if err != nil {
			return nil, err
		}
		ctx.Secrets = store
	}

	if includeValues {
		valuesFiles := InferValuesFiles(configPath, opts.ValuesFiles)
		values, err := LoadValuesStore(valuesFiles, ctx.ValuesScope)
		if err != nil {
			return nil, err
		}
		ctx.Values = values
	}

	return ctx, nil
}

func (p *ProjectContext) SwarmClient() (swarm.Client, error) {
	factory := p.clientFactory
	if factory == nil {
		factory = swarm.NewClient
	}
	client, err := factory(p.ContextName)
	if err != nil {
		if errors.Is(err, swarm.ErrNotImplemented) {
			return nil, fmt.Errorf("swarm client not implemented (context %q)", p.ContextName)
		}
		return nil, err
	}
	return client, nil
}
