package store

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	vault "github.com/hashicorp/vault/api"
	bao "github.com/openbao/openbao/api/v2"
	. "github.com/samber/lo"
)

type Client interface {
	ResolveSecret(ctx context.Context, path string) ([]byte, error)

	// Read Raw logical ops
	Read(path string) (map[string]any, error)
	Write(path string, data map[string]any) (map[string]any, error)

	// KVGet KV v2 helpers (mount-aware)
	KVGet(mount, path string) (map[string]any, error)
	KVPut(mount, path string, data map[string]any) error

	// WithToken Auth
	WithToken(token string)
	Token() string

	// StartAutoRenew Renewal (token/secret if renewable)
	StartAutoRenew(ctx context.Context, lease any) (stop func(), err error)

	// WithNamespace Multi-tenancy
	WithNamespace(ns string)

	// Unwrap Optional unwrap helper
	Unwrap(wrappingToken string) (map[string]any, error)

	Close()
}

func splitField(s string) (string, string) {
	parts := strings.SplitN(s, "#", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return s, ""
}

func toKVv2Path(base string) string {
	// heuristic: insert /data/ after first path element (mount name)
	base = strings.TrimLeft(base, "/")
	parts := strings.SplitN(base, "/", 2)
	if len(parts) == 1 {
		return parts[0] + "/data"
	}
	return parts[0] + "/data/" + parts[1]
}

func pickField(m map[string]interface{}, field string) ([]byte, error) {
	if field != "" {
		if v, ok := m[field]; ok {
			if s, ok := v.(string); ok {
				return []byte(s), nil
			}
			return nil, fmt.Errorf("field %q exists but is not a string", field)
		}
		return nil, fmt.Errorf("field %q not found", field)
	}
	// prefer "data"
	if v, ok := m["data"]; ok {
		if s, ok := v.(string); ok {
			return []byte(s), nil
		}
	}
	// if only one key, return it
	if len(m) == 1 {
		for _, v := range m {
			if s, ok := v.(string); ok {
				return []byte(s), nil
			}
		}
	}
	return nil, fmt.Errorf("could not choose a value; specify #field in the path")
}

func trimS(s string) string {
	if len(s) > 0 && s[len(s)-1] == '/' {
		return s[:len(s)-1]
	}
	return s
}
func trimL(s string) string {
	if len(s) > 0 && s[0] == '/' {
		return s[1:]
	}
	return s
}

func coerce(s any, err error, fieldsFilter ...string) (map[string]any, error) {
	if err != nil {
		return nil, err
	}

	rval := reflect.ValueOf(s)
	if !rval.IsValid() || rval.IsNil() {
		return nil, nil
	}

	var sData map[string]any
	switch s.(type) {
	case *bao.Secret:
		sData = s.(*bao.Secret).Data
	case *vault.Secret:
		sData = s.(*vault.Secret).Data
	}
	data := make(map[string]any, len(sData))
	maps.Copy(data, sData)

	if len(fieldsFilter) > 0 {
		filteredKeys := Filter(Keys(data), func(k string, _ int) bool {
			if slices.ContainsFunc(fieldsFilter, func(c string) bool {
				return strings.EqualFold(c, k)
			}) {
				return true
			}
			return false
		})
		maps.DeleteFunc(data, func(k string, _ any) bool {
			return !slices.Contains(filteredKeys, k)
		})
	}

	return data, nil
}
