package templates

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/cmmoran/swarmcp/internal/yamlutil"
	"gopkg.in/yaml.v3"
)

type Resolver interface {
	ConfigValue(name string) (any, error)
	ConfigRef(name string) (string, error)
	ConfigRefs(pattern string) ([]string, error)
	SecretValue(name string) (string, error)
	SecretRef(name string) (string, error)
	SecretRefs(pattern string) ([]string, error)
	RuntimeValue(args ...string) (string, error)
}

type Engine struct {
	resolver Resolver
}

func New(resolver Resolver) *Engine {
	return &Engine{resolver: resolver}
}

func (e *Engine) FuncMap() template.FuncMap {
	funcs := sprig.TxtFuncMap()
	funcs["config_value"] = func(name string) (any, error) {
		return e.resolver.ConfigValue(name)
	}
	funcs["config_value_index"] = func(name string, index any) (any, error) {
		value, err := e.resolver.ConfigValue(name)
		if err != nil {
			return "", fmt.Errorf("config_value_index %q: %w", name, err)
		}
		value = coerceStructuredValue(value)
		idx, err := toTemplateIndex(index)
		if err != nil {
			return "", err
		}
		switch typed := value.(type) {
		case []any:
			if idx < 0 || idx >= len(typed) {
				return "", fmt.Errorf("config %q index %d out of range", name, idx)
			}
			return typed[idx], nil
		case []string:
			if idx < 0 || idx >= len(typed) {
				return "", fmt.Errorf("config %q index %d out of range", name, idx)
			}
			return typed[idx], nil
		default:
			return "", fmt.Errorf("config %q is not a list", name)
		}
	}
	funcs["config_value_get"] = func(name string, key any) (any, error) {
		value, err := e.resolver.ConfigValue(name)
		if err != nil {
			return "", fmt.Errorf("config_value_get %q: %w", name, err)
		}
		value = coerceStructuredValue(value)
		keyStr, ok := key.(string)
		if !ok {
			return "", fmt.Errorf("config %q key must be a string", name)
		}
		switch typed := value.(type) {
		case map[string]any:
			item, ok := typed[keyStr]
			if !ok {
				return "", fmt.Errorf("config %q key %q not found", name, keyStr)
			}
			return item, nil
		case map[string]string:
			item, ok := typed[keyStr]
			if !ok {
				return "", fmt.Errorf("config %q key %q not found", name, keyStr)
			}
			return item, nil
		default:
			return "", fmt.Errorf("config %q is not a map", name)
		}
	}
	funcs["config_ref"] = func(name string) (string, error) {
		return e.resolver.ConfigRef(name)
	}
	funcs["config_refs"] = func(pattern string) ([]string, error) {
		return e.resolver.ConfigRefs(pattern)
	}
	funcs["secret_value"] = func(name string) (string, error) {
		return e.resolver.SecretValue(name)
	}
	funcs["secret_ref"] = func(name string) (string, error) {
		return e.resolver.SecretRef(name)
	}
	funcs["secret_refs"] = func(pattern string) ([]string, error) {
		return e.resolver.SecretRefs(pattern)
	}
	funcs["runtime_value"] = func(args ...string) (string, error) {
		return e.resolver.RuntimeValue(args...)
	}
	funcs["external_ip"] = func() (string, error) {
		return ExternalIP()
	}
	funcs["escape_template"] = func(inputs ...string) string {
		return EscapeTemplate(inputs...)
	}
	funcs["escape_swarm_template"] = func(inputs ...string) string {
		return EscapeSwarmTemplate(inputs...)
	}
	funcs["swarm_network_cidrs"] = func(args ...string) ([]string, error) {
		if len(args) == 1 {
			expanded, err := e.resolver.RuntimeValue(args[0])
			if err != nil {
				return nil, err
			}
			return swarmNetworkCIDRs(expanded)
		}
		return swarmNetworkCIDRs(args...)
	}

	return funcs
}

func coerceStructuredValue(value any) any {
	str, ok := value.(string)
	if !ok {
		return value
	}
	var parsed any
	if err := yaml.Unmarshal([]byte(str), &parsed); err != nil {
		return value
	}
	normalized := yamlutil.NormalizeValue(parsed)
	switch normalized.(type) {
	case map[string]any, []any:
		return normalized
	default:
		return value
	}
}

func toTemplateIndex(value any) (int, error) {
	const maxInt = int(^uint(0) >> 1)
	const minInt = -maxInt - 1
	switch typed := value.(type) {
	case int:
		return typed, nil
	case int8:
		return int(typed), nil
	case int16:
		return int(typed), nil
	case int32:
		return int(typed), nil
	case int64:
		if typed > int64(maxInt) || typed < int64(minInt) {
			return 0, fmt.Errorf("index %d overflows int", typed)
		}
		return int(typed), nil
	case uint:
		if typed > uint(maxInt) {
			return 0, fmt.Errorf("index %d overflows int", typed)
		}
		return int(typed), nil
	case uint8:
		return int(typed), nil
	case uint16:
		return int(typed), nil
	case uint32:
		return int(typed), nil
	case uint64:
		if typed > uint64(maxInt) {
			return 0, fmt.Errorf("index %d overflows int", typed)
		}
		return int(typed), nil
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return 0, fmt.Errorf("index %q is not an integer", typed)
		}
		parsed, err := strconv.Atoi(trimmed)
		if err != nil {
			return 0, fmt.Errorf("index %q is not an integer", typed)
		}
		return parsed, nil
	case float32:
		value := float64(typed)
		if math.Trunc(value) != value {
			return 0, fmt.Errorf("index %v is not an integer", value)
		}
		if value > float64(maxInt) || value < float64(minInt) {
			return 0, fmt.Errorf("index %v overflows int", value)
		}
		return int(value), nil
	case float64:
		if math.Trunc(typed) != typed {
			return 0, fmt.Errorf("index %v is not an integer", typed)
		}
		if typed > float64(maxInt) || typed < float64(minInt) {
			return 0, fmt.Errorf("index %v overflows int", typed)
		}
		return int(typed), nil
	default:
		return 0, fmt.Errorf("index %T is not an integer", value)
	}
}

func (e *Engine) Render(name string, content string, data any) (string, error) {
	tpl, err := template.New(name).Funcs(e.FuncMap()).Parse(content)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func (e *Engine) RenderFile(path string, data any) (string, error) {
	base, fragment := SplitSource(path)
	content, err := os.ReadFile(base)
	if err != nil {
		return "", err
	}
	rendered := string(content)
	if IsTemplateSource(base) {
		rendered, err = e.Render(base, rendered, data)
		if err != nil {
			return "", err
		}
	}
	if fragment == "" || fragment == "#" {
		return rendered, nil
	}
	return ResolveYAMLFragment(rendered, fragment)
}
