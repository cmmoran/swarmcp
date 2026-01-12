package config

import (
	"path/filepath"
	"strings"
)

const (
	DefaultServiceStandardName = "service"
	DefaultServiceTarget       = "/data"
)

func ServiceStandardName(cfg *Config) string {
	if cfg == nil {
		return DefaultServiceStandardName
	}
	name := strings.TrimSpace(cfg.Project.Defaults.Volumes.ServiceStandard)
	if name == "" {
		return DefaultServiceStandardName
	}
	return name
}

func ServiceTarget(cfg *Config) string {
	if cfg == nil {
		return DefaultServiceTarget
	}
	target := strings.TrimSpace(cfg.Project.Defaults.Volumes.ServiceTarget)
	if target == "" {
		return DefaultServiceTarget
	}
	return target
}

func ResolvedStackName(stackName string, mode string, partitionName string) string {
	if mode == "partitioned" && partitionName != "" {
		return partitionName + "_" + stackName
	}
	return stackName
}

func StackVolumeSource(basePath string, projectName string, stackName string, mode string, partitionName string, serviceName string, volumeName string, defSubpath string, refSubpath string, stackScoped bool) string {
	stackSegment := ResolvedStackName(stackName, mode, partitionName)
	var parts []string
	parts = append(parts, basePath, projectName, stackSegment)
	subpath := strings.TrimSpace(defSubpath)
	if subpath == "" {
		subpath = strings.TrimSpace(refSubpath)
	}
	if stackScoped {
		if subpath == "" {
			subpath = volumeName
		}
		if subpath != "" {
			parts = append(parts, subpath)
		}
	} else {
		parts = append(parts, serviceName)
		if subpath == "" {
			subpath = volumeName
		}
		if subpath != "" {
			parts = append(parts, subpath)
		}
	}
	return filepath.Join(parts...)
}
