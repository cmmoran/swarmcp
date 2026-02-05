package cmd

type Options struct {
	ConfigPath      string
	NoWarnUnmanaged bool
	SkipHealthcheck bool
	SecretsFile     string
	ValuesFiles     []string
	Deployment      string
	Context         string
	Partition       string
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
	Confirm         bool
	Offline         bool
	PruneAutoLabels bool
	DiffSources     bool
}
