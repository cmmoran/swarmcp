package mergeutil

import (
	"reflect"
	"strings"
	"testing"
)

func TestMergeValuesAndMaps(t *testing.T) {
	base := map[string]any{
		"name": "demo",
		"meta": map[string]any{"env": "dev"},
		"list": []any{"a"},
	}
	overlay := map[string]any{
		"meta":  map[string]any{"region": "us"},
		"list+": []any{"b"},
	}
	got, err := MergeValues(base, overlay, Options{})
	if err != nil {
		t.Fatalf("MergeValues: %v", err)
	}
	want := map[string]any{
		"name": "demo",
		"meta": map[string]any{"env": "dev", "region": "us"},
		"list": []any{"a", "b"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected merged value:\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestMergeValueMapsErrorsAndKeyedMerge(t *testing.T) {
	if _, err := MergeListAppend("bad", []any{"x"}); err == nil || !strings.Contains(err.Error(), "append expects list base") {
		t.Fatalf("expected append base error, got %v", err)
	}
	if _, err := MergeValueMaps(map[string]any{}, map[string]any{"items~": []any{"x"}}, Options{}); err == nil || !strings.Contains(err.Error(), "keyed merge requires a handler") {
		t.Fatalf("expected keyed merge handler error, got %v", err)
	}

	got, err := MergeValueMaps(
		map[string]any{"items": []any{map[string]any{"name": "a", "value": 1}}},
		map[string]any{"items~": []any{map[string]any{"name": "a", "value": 2}, map[string]any{"name": "b", "value": 3}}},
		Options{KeyedMerge: MergeListByKeySimple},
	)
	if err != nil {
		t.Fatalf("MergeValueMaps keyed: %v", err)
	}
	items := got["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected 2 merged items, got %#v", items)
	}
}

func TestMergeListByKeySimple(t *testing.T) {
	got, err := MergeListByKeySimple(
		[]any{map[string]any{"name": "a", "value": 1}, "plain"},
		map[string]any{
			"_key":  "id",
			"items": []any{map[string]any{"id": "x", "value": 2}},
		},
	)
	if err != nil {
		t.Fatalf("MergeListByKeySimple: %v", err)
	}
	if len(got.([]any)) != 3 {
		t.Fatalf("expected appended keyed item, got %#v", got)
	}

	if _, err := MergeListByKeySimple("bad", []any{"x"}); err == nil {
		t.Fatalf("expected bad base error")
	}
	if key, ok := keyForItem("name", "name"); !ok || key != "name" {
		t.Fatalf("expected default key lookup for scalar items")
	}
	if _, ok := keyForItem("name", "id"); ok {
		t.Fatalf("did not expect scalar to have non-name key")
	}
}
