package templates

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/cmmoran/swarmcp/internal/yamlutil"
	"gopkg.in/yaml.v3"
)

func ResolveYAMLFragment(content string, fragment string) (string, error) {
	if fragment == "" || fragment == "#" {
		return content, nil
	}
	if !strings.HasPrefix(fragment, "#/") {
		return "", fmt.Errorf("unsupported fragment %q", fragment)
	}

	var root any
	if err := yaml.Unmarshal([]byte(content), &root); err != nil {
		return "", err
	}

	return ResolveFragment(root, fragment)
}

func decodePointerToken(token string) string {
	token = strings.ReplaceAll(token, "~1", "/")
	token = strings.ReplaceAll(token, "~0", "~")
	return token
}

func lookupFragment(node any, key string) (any, bool) {
	switch typed := node.(type) {
	case map[string]any:
		value, ok := typed[key]
		return value, ok
	case map[any]any:
		if value, ok := typed[key]; ok {
			return value, true
		}
		for rawKey, value := range typed {
			if fmt.Sprint(rawKey) == key {
				return value, true
			}
		}
		return nil, false
	case []any:
		index, err := strconv.Atoi(key)
		if err != nil || index < 0 || index >= len(typed) {
			return nil, false
		}
		return typed[index], true
	default:
		return nil, false
	}
}

func formatFragmentValue(value any) (string, error) {
	switch typed := value.(type) {
	case nil:
		return "", nil
	case string:
		return typed, nil
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return fmt.Sprint(typed), nil
	}

	normalized := yamlutil.NormalizeValue(value)
	switch normalized.(type) {
	case []any, map[string]any:
		encoded, err := json.Marshal(normalized)
		if err != nil {
			return "", err
		}
		return string(encoded), nil
	default:
		return fmt.Sprint(normalized), nil
	}
}
