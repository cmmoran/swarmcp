package config

import (
	"strings"

	"github.com/cmmoran/swarmcp/internal/sliceutil"
)

const networkEphemeralPrefix = "svc_"

func StackInstanceName(project, stack, partition, mode string) string {
	if mode == "partitioned" && partition != "" {
		return project + "_" + partition + "_" + stack
	}
	return project + "_" + stack
}

func RenderNetworkTemplate(input, project, partition string) string {
	out := strings.ReplaceAll(input, "<project>", project)
	out = strings.ReplaceAll(out, "<partition>", partition)
	out = strings.ReplaceAll(out, "{project}", project)
	out = strings.ReplaceAll(out, "{partition}", partition)
	return out
}

func SharedNetworkNames(cfg *Config, partition string) []string {
	if cfg == nil || len(cfg.Project.Defaults.Networks.Shared) == 0 {
		return nil
	}
	out := make([]string, 0, len(cfg.Project.Defaults.Networks.Shared))
	for _, name := range cfg.Project.Defaults.Networks.Shared {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out = append(out, RenderNetworkTemplate(name, cfg.Project.Name, partition))
	}
	if len(out) == 0 {
		return nil
	}
	return sliceutil.DedupeStringsPreserveOrder(out)
}

func NetworksSharedString(cfg *Config, partition string) string {
	names := SharedNetworkNames(cfg, partition)
	if len(names) == 0 {
		return ""
	}
	return strings.Join(names, ",")
}

func EphemeralNetworkKey(serviceName string) string {
	if serviceName == "" {
		return ""
	}
	return networkEphemeralPrefix + serviceName
}

func EphemeralNetworkName(cfg *Config, stackName, stackMode, partition, serviceName string) string {
	if cfg == nil || stackName == "" || serviceName == "" {
		return ""
	}
	key := EphemeralNetworkKey(serviceName)
	if key == "" {
		return ""
	}
	stackInstance := StackInstanceName(cfg.Project.Name, stackName, partition, stackMode)
	if stackInstance == "" {
		return ""
	}
	return stackInstance + "_" + key
}
