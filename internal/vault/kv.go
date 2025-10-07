package vault

import (
	"context"
	"fmt"
	"strings"

	vapi "github.com/hashicorp/vault/api"
)

// ResolveSecret loads a secret from KV v2 (or best-effort generic) and returns a single field.
// Path may be "mount/path#field". If no field is specified, we try "value", then if only one field exists, use it.
func (c *sdkClient) ResolveSecret(ctx context.Context, path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("vault: empty path")
	}
	basePath, field := splitField(path)
	// Try KV v2 first
	if b, err := readKVv2(ctx, c.v, basePath, field); err == nil {
		return b, nil
	}
	// Fallback: raw read
	sec, err := c.v.Logical().ReadWithContext(ctx, basePath)
	if err != nil {
		return nil, err
	}
	if sec == nil || sec.Data == nil {
		return nil, fmt.Errorf("vault: no data at %s", basePath)
	}
	return pickField(sec.Data, field)
}

func splitField(s string) (string, string) {
	parts := strings.SplitN(s, "#", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return s, ""
}

func readKVv2(ctx context.Context, client *vapi.Client, basePath, field string) ([]byte, error) {
	kvPath := toKVv2Path(basePath)
	sec, err := client.Logical().ReadWithContext(ctx, kvPath)
	if err != nil {
		return nil, err
	}
	if sec == nil || sec.Data == nil {
		return nil, fmt.Errorf("no data")
	}
	// KV v2 shape: data.data
	rawData, ok := sec.Data["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("not kvv2")
	}
	return pickField(rawData, field)
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
	// prefer "value"
	if v, ok := m["value"]; ok {
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
