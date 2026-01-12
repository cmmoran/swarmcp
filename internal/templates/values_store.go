package templates

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/cmmoran/swarmcp/internal/mergeutil"
	"github.com/cmmoran/swarmcp/internal/yamlutil"
	"gopkg.in/yaml.v3"
)

func LoadValuesFiles(paths []string, scope Scope) (any, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	var merged any
	for _, path := range paths {
		rendered, err := renderValuesFile(path, scope)
		if err != nil {
			return nil, err
		}
		var parsed any
		if err := yaml.Unmarshal([]byte(rendered), &parsed); err != nil {
			return nil, err
		}
		merged, err = mergeValues(merged, yamlutil.NormalizeValue(parsed))
		if err != nil {
			return nil, fmt.Errorf("values file %q: %w", path, err)
		}
	}
	return merged, nil
}

func renderValuesFile(path string, scope Scope) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if !IsTemplateSource(path) {
		return string(content), nil
	}
	tpl, err := template.New(path).Funcs(valuesFuncMap(scope)).Parse(string(content))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, nil); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func valuesFuncMap(scope Scope) template.FuncMap {
	funcs := sprig.TxtFuncMap()
	funcs["external_ip"] = func() (string, error) {
		return ExternalIP()
	}
	funcs["escape_template"] = func(inputs ...string) string {
		return EscapeTemplate(inputs...)
	}
	funcs["escape_swarm_template"] = func(inputs ...string) string {
		return EscapeSwarmTemplate(inputs...)
	}
	funcs["runtime_value"] = func(args ...string) (string, error) {
		if len(args) == 0 {
			return "", nil
		}
		if len(args) > 1 {
			return "", fmt.Errorf("runtime_value: values templates only support a single argument")
		}
		return ExpandTokens(args[0], scope), nil
	}
	funcs["swarm_network_cidrs"] = func(args ...string) ([]string, error) {
		if len(args) == 1 {
			return swarmNetworkCIDRs(ExpandTokens(args[0], scope))
		}
		return swarmNetworkCIDRs(args...)
	}
	return funcs
}

func mergeValues(base any, overlay any) (any, error) {
	return mergeutil.MergeValues(base, overlay, mergeutil.Options{
		KeyedMerge: mergeListByKey,
	})
}

func mergeListByKey(base any, overlay any) (any, error) {
	if overlay == nil {
		return base, nil
	}
	keyField := "name"
	var overlayItems []any
	switch typed := overlay.(type) {
	case []any:
		overlayItems = typed
	case map[string]any:
		if key, ok := typed["_key"]; ok {
			if keyStr, ok := key.(string); ok && keyStr != "" {
				keyField = keyStr
			}
		}
		rawItems, ok := typed["items"]
		if !ok {
			return nil, fmt.Errorf("keyed merge expects items list")
		}
		items, ok := rawItems.([]any)
		if !ok {
			return nil, fmt.Errorf("keyed merge expects items list")
		}
		overlayItems = items
	default:
		return nil, fmt.Errorf("keyed merge expects list or {_key,items}")
	}

	var baseList []any
	if base == nil {
		baseList = []any{}
	} else {
		list, ok := base.([]any)
		if !ok {
			return nil, fmt.Errorf("keyed merge expects list base")
		}
		baseList = append([]any(nil), list...)
	}

	if isScalarList(baseList) && isScalarList(overlayItems) {
		return mergeScalarListByKey(baseList, overlayItems), nil
	}

	index := make(map[string]int)
	for i, item := range baseList {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("keyed merge expects map items")
		}
		keyValue, ok := obj[keyField]
		if !ok {
			return nil, fmt.Errorf("keyed merge missing key %q", keyField)
		}
		keyStr, ok := keyValue.(string)
		if !ok || keyStr == "" {
			return nil, fmt.Errorf("keyed merge expects string key %q", keyField)
		}
		index[keyStr] = i
	}

	for _, item := range overlayItems {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("keyed merge expects map items")
		}
		keyValue, ok := obj[keyField]
		if !ok {
			return nil, fmt.Errorf("keyed merge missing key %q", keyField)
		}
		keyStr, ok := keyValue.(string)
		if !ok || keyStr == "" {
			return nil, fmt.Errorf("keyed merge expects string key %q", keyField)
		}
		if idx, ok := index[keyStr]; ok {
			merged, err := mergeValues(baseList[idx], obj)
			if err != nil {
				return nil, err
			}
			baseList[idx] = merged
		} else {
			baseList = append(baseList, obj)
			index[keyStr] = len(baseList) - 1
		}
	}

	return baseList, nil
}

func isScalarList(items []any) bool {
	for _, item := range items {
		if _, ok := item.(map[string]any); ok {
			return false
		}
	}
	return true
}

func mergeScalarListByKey(base []any, overlay []any) []any {
	out := append([]any(nil), base...)
	matched := make([]bool, len(out))
	for _, item := range overlay {
		replaced := false
		for i := range out {
			if matched[i] {
				continue
			}
			if reflect.DeepEqual(out[i], item) {
				out[i] = item
				matched[i] = true
				replaced = true
				break
			}
		}
		if !replaced {
			out = append(out, item)
			matched = append(matched, true)
		}
	}
	return out
}
