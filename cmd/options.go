package cmd

type Options struct {
	ConfigPath      string
	NoWarnUnmanaged bool
	SkipHealthcheck bool
	SecretsFile     string
	ValuesFiles     []string
	Deployments     []string
	Context         string
	Partitions      []string
	Stacks          []string
	AllowMissing    bool
	NoInfer         bool
	DebugContent    bool
	DebugContentMax int
	Debug           bool
	Prune           bool
	PruneServices   bool
	Preserve        int
	Serial          bool
	NoUI            bool
	Output          string
	Confirm         bool
	Offline         bool
	PruneAutoLabels bool
	DiffSources     bool
}
