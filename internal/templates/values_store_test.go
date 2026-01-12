package templates

import "testing"

func TestMergeValuesAppend(t *testing.T) {
	base := map[string]any{
		"list": []any{1, 2},
	}
	overlay := map[string]any{
		"list+": []any{3, 4},
	}
	merged, err := mergeValues(base, overlay)
	if err != nil {
		t.Fatalf("mergeValues: %v", err)
	}
	out, ok := merged.(map[string]any)
	if !ok {
		t.Fatalf("expected map result")
	}
	list, ok := out["list"].([]any)
	if !ok {
		t.Fatalf("expected list result")
	}
	if len(list) != 4 {
		t.Fatalf("expected 4 items, got %d", len(list))
	}
}

func TestMergeValuesKeyedMergeDefaultName(t *testing.T) {
	base := map[string]any{
		"items": []any{
			map[string]any{"name": "a", "value": 1},
		},
	}
	overlay := map[string]any{
		"items~": []any{
			map[string]any{"name": "a", "value": 2},
			map[string]any{"name": "b", "value": 3},
		},
	}
	merged, err := mergeValues(base, overlay)
	if err != nil {
		t.Fatalf("mergeValues: %v", err)
	}
	out := merged.(map[string]any)
	items := out["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	first := items[0].(map[string]any)
	if first["value"].(int) != 2 {
		t.Fatalf("expected merged value, got %v", first["value"])
	}
}

func TestMergeValuesKeyedMergeWithKey(t *testing.T) {
	base := map[string]any{
		"ports": []any{
			map[string]any{"port": "http", "target": 80},
		},
	}
	overlay := map[string]any{
		"ports~": map[string]any{
			"_key": "port",
			"items": []any{
				map[string]any{"port": "http", "target": 8080},
				map[string]any{"port": "grpc", "target": 5000},
			},
		},
	}
	merged, err := mergeValues(base, overlay)
	if err != nil {
		t.Fatalf("mergeValues: %v", err)
	}
	out := merged.(map[string]any)
	items := out["ports"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	first := items[0].(map[string]any)
	if first["target"].(int) != 8080 {
		t.Fatalf("expected merged target, got %v", first["target"])
	}
}

func TestMergeValuesScalarKeyedMerge(t *testing.T) {
	base := map[string]any{
		"list": []any{"a", "b", "b"},
	}
	overlay := map[string]any{
		"list~": []any{"b", "b", "c"},
	}
	merged, err := mergeValues(base, overlay)
	if err != nil {
		t.Fatalf("mergeValues: %v", err)
	}
	out := merged.(map[string]any)
	list := out["list"].([]any)
	if len(list) != 4 {
		t.Fatalf("expected 4 items, got %d", len(list))
	}
	if list[2].(string) != "b" || list[3].(string) != "c" {
		t.Fatalf("unexpected merged list: %#v", list)
	}
}
