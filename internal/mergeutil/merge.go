package mergeutil

import (
	"fmt"
	"sort"
	"strings"
)

type Options struct {
	KeyedMerge func(base any, overlay any) (any, error)
}

func MergeValues(base any, overlay any, opts Options) (any, error) {
	if overlay == nil {
		return base, nil
	}
	if base == nil {
		return overlay, nil
	}
	baseMap, baseOK := base.(map[string]any)
	overlayMap, overlayOK := overlay.(map[string]any)
	if overlayOK {
		if !baseOK {
			baseMap = map[string]any{}
		}
		return MergeValueMaps(baseMap, overlayMap, opts)
	}
	return overlay, nil
}

func MergeValueMaps(base map[string]any, overlay map[string]any, opts Options) (map[string]any, error) {
	out := make(map[string]any, len(base))
	for k, v := range base {
		out[k] = v
	}

	var normalKeys []string
	var appendKeys []string
	var mergeKeys []string
	for key := range overlay {
		if strings.HasSuffix(key, "+") {
			appendKeys = append(appendKeys, key)
		} else if strings.HasSuffix(key, "~") {
			mergeKeys = append(mergeKeys, key)
		} else {
			normalKeys = append(normalKeys, key)
		}
	}
	sort.Strings(normalKeys)
	sort.Strings(appendKeys)
	sort.Strings(mergeKeys)

	for _, key := range normalKeys {
		value, err := MergeValues(out[key], overlay[key], opts)
		if err != nil {
			return nil, err
		}
		out[key] = value
	}
	for _, key := range appendKeys {
		baseKey := strings.TrimSuffix(key, "+")
		value, err := MergeListAppend(out[baseKey], overlay[key])
		if err != nil {
			return nil, err
		}
		out[baseKey] = value
	}
	for _, key := range mergeKeys {
		if opts.KeyedMerge == nil {
			return nil, fmt.Errorf("keyed merge requires a handler")
		}
		baseKey := strings.TrimSuffix(key, "~")
		value, err := opts.KeyedMerge(out[baseKey], overlay[key])
		if err != nil {
			return nil, err
		}
		out[baseKey] = value
	}
	return out, nil
}

func MergeListAppend(base any, overlay any) (any, error) {
	if overlay == nil {
		return base, nil
	}
	overlayList, ok := overlay.([]any)
	if !ok {
		return nil, fmt.Errorf("append expects a list")
	}
	if base == nil {
		return overlayList, nil
	}
	baseList, ok := base.([]any)
	if !ok {
		return nil, fmt.Errorf("append expects list base")
	}
	return append(baseList, overlayList...), nil
}

func MergeListByKeySimple(base any, overlay any) (any, error) {
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

	index := map[any]int{}
	for i, item := range baseList {
		key, ok := keyForItem(item, keyField)
		if ok {
			index[key] = i
		}
	}

	for _, item := range overlayItems {
		key, ok := keyForItem(item, keyField)
		if !ok {
			baseList = append(baseList, item)
			continue
		}
		if idx, ok := index[key]; ok {
			baseList[idx] = item
			continue
		}
		index[key] = len(baseList)
		baseList = append(baseList, item)
	}
	return baseList, nil
}

func keyForItem(item any, field string) (any, bool) {
	switch typed := item.(type) {
	case map[string]any:
		value, ok := typed[field]
		return value, ok
	default:
		if field == "name" {
			return typed, true
		}
		return nil, false
	}
}
