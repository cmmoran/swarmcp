package yamlutil

import (
	"reflect"
	"testing"
)

func TestNormalizeValue(t *testing.T) {
	input := map[any]any{
		"root": []any{
			map[any]any{1: "one"},
			map[string]any{"nested": []any{map[any]any{"x": "y"}}},
		},
	}
	got := NormalizeValue(input)
	want := map[string]any{
		"root": []any{
			map[string]any{"1": "one"},
			map[string]any{"nested": []any{map[string]any{"x": "y"}}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected normalized value:\nwant: %#v\ngot:  %#v", want, got)
	}

	if NormalizeValue("plain") != "plain" {
		t.Fatalf("expected scalar to be unchanged")
	}
}
