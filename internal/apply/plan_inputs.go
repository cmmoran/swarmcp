package apply

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func FileInputs(kind string, paths []string) ([]PlanInput, error) {
	out := make([]PlanInput, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		absPath, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("%s input %q: %w", kind, path, err)
		}
		hash, err := fileSHA256(absPath)
		if err != nil {
			return nil, fmt.Errorf("%s input %q: %w", kind, path, err)
		}
		out = append(out, PlanInput{
			Kind:   kind,
			Path:   filepath.Clean(absPath),
			SHA256: hash,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Path < out[j].Path
	})
	return out, nil
}

func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
