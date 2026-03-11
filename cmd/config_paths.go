package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cmmoran/swarmcp/internal/fsutil"
)

func effectiveProjectConfigPaths() ([]string, error) {
	if len(opts.ConfigPaths) > 0 {
		return normalizeConfigPaths(opts.ConfigPaths), nil
	}
	paths, err := readProjectConfigPaths()
	if err != nil {
		return nil, err
	}
	if len(paths) > 0 {
		return paths, nil
	}
	return nil, fmt.Errorf("project config is required (--config or .swarmcp.project)")
}

func effectiveReleaseConfigPaths() []string {
	return normalizeConfigPaths(opts.ReleaseConfigs)
}

func effectiveConfigPaths() ([]string, error) {
	paths, err := effectiveProjectConfigPaths()
	if err != nil {
		return nil, err
	}
	paths = append(paths, effectiveReleaseConfigPaths()...)
	return paths, nil
}

func primaryConfigPath() (string, error) {
	paths, err := effectiveProjectConfigPaths()
	if err != nil {
		return "", err
	}
	return paths[0], nil
}

func normalizeConfigPaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		out = append(out, path)
	}
	return out
}

func readProjectConfigPaths() ([]string, error) {
	projectFile, err := filepath.Abs(".swarmcp.project")
	if err != nil || !fsutil.FileExists(projectFile) {
		return nil, nil
	}
	data, err := os.ReadFile(projectFile)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		paths = append(paths, line)
	}
	return normalizeConfigPaths(paths), nil
}

func writeProjectConfigPaths(paths []string) error {
	paths = normalizeConfigPaths(paths)
	if len(paths) == 0 {
		return nil
	}
	var buf bytes.Buffer
	for _, path := range paths {
		_, _ = buf.WriteString(path)
		_ = buf.WriteByte('\n')
	}
	return os.WriteFile(".swarmcp.project", buf.Bytes(), 0o644)
}
