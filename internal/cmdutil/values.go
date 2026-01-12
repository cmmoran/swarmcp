package cmdutil

import (
	"errors"
	"os"

	"github.com/cmmoran/swarmcp/internal/secrets"
	"github.com/cmmoran/swarmcp/internal/templates"
)

func LoadSecretsStore(path string) (*secrets.Store, error) {
	if path == "" {
		return nil, nil
	}
	return secrets.Load(path)
}

func LoadValuesStore(paths []string, scope templates.Scope) (any, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	return templates.LoadValuesFiles(paths, scope)
}

func LoadOrInitSecretsStore(path string) (*secrets.Store, error) {
	store, err := secrets.Load(path)
	if err == nil {
		return store, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return &secrets.Store{Values: map[string]string{}}, nil
}

func TruncateContent(content string, max int) string {
	if max <= 0 || len(content) <= max {
		return content
	}
	return content[:max] + "... [truncated]"
}
