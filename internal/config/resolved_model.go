package config

import "fmt"

type ResolvedModelOptions struct {
	ConfigPaths        []string
	ReleaseConfigPaths []string
	Deployment         string
	Partition          string
	Stack              string
	LoadOptions        LoadOptions
}

type ResolvedModel struct {
	Config      *Config
	Model       map[string]any
	Partition   string
	Stack       string
	StackFilter []string
	Trace       *LoadTrace
}

func LoadResolvedModel(opts ResolvedModelOptions) (*ResolvedModel, error) {
	return loadResolvedModel(opts, nil)
}

func LoadResolvedModelTrace(opts ResolvedModelOptions, fieldPath []string) (*ResolvedModel, error) {
	trace := &LoadTrace{FieldPath: append([]string(nil), fieldPath...)}
	return loadResolvedModel(opts, trace)
}

func loadResolvedModel(opts ResolvedModelOptions, trace *LoadTrace) (*ResolvedModel, error) {
	var (
		cfg *Config
		err error
	)
	if trace != nil {
		cfg, trace, err = LoadFilesWithReleaseTrace(opts.ConfigPaths, opts.ReleaseConfigPaths, opts.LoadOptions, trace.FieldPath)
	} else {
		cfg, err = LoadFilesWithReleaseOptions(opts.ConfigPaths, opts.ReleaseConfigPaths, opts.LoadOptions)
	}
	if err != nil {
		return nil, err
	}
	if opts.Deployment != "" {
		cfg.Project.Deployment = opts.Deployment
	}
	if err := ValidateDeployment(cfg); err != nil {
		return nil, err
	}
	if opts.Partition != "" && !partitionExists(cfg, opts.Partition) {
		return nil, fmt.Errorf("partition %q not found in project.partitions", opts.Partition)
	}
	stackFilter := []string(nil)
	if opts.Stack != "" {
		if _, ok := cfg.Stacks[opts.Stack]; !ok {
			return nil, fmt.Errorf("stack %q not found in stacks", opts.Stack)
		}
		stackFilter = []string{opts.Stack}
	}
	model, err := debugResolvedMapWithTrace(cfg, opts.Partition, stackFilter, trace)
	if err != nil {
		return nil, err
	}
	return &ResolvedModel{
		Config:      cfg,
		Model:       model,
		Partition:   opts.Partition,
		Stack:       opts.Stack,
		StackFilter: stackFilter,
		Trace:       trace,
	}, nil
}
