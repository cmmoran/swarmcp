package config

import (
	"fmt"

	"github.com/cmmoran/swarmcp/internal/mergeutil"
)

func mergeMaps(base map[string]any, overlays ...map[string]any) (map[string]any, error) {
	var merged any = base
	for _, overlay := range overlays {
		if overlay == nil {
			continue
		}
		var err error
		merged, err = mergeValues(merged, overlay)
		if err != nil {
			return nil, err
		}
	}
	if merged == nil {
		return map[string]any{}, nil
	}
	out, ok := merged.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("merged document is not a map")
	}
	return out, nil
}

func mergeValues(base any, overlay any) (any, error) {
	return mergeutil.MergeValues(base, overlay, mergeutil.Options{
		KeyedMerge: mergeutil.MergeListByKeySimple,
	})
}
