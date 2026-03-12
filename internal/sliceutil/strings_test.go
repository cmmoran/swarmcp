package sliceutil

import (
	"reflect"
	"testing"
)

func TestFilterPartition(t *testing.T) {
	partitions := []string{"dev", "qa", "prod"}
	if got := FilterPartition(partitions, "qa"); !reflect.DeepEqual(got, []string{"qa"}) {
		t.Fatalf("unexpected filtered partition: %#v", got)
	}
	if got := FilterPartition(partitions, ""); !reflect.DeepEqual(got, partitions) {
		t.Fatalf("expected empty filter to keep all partitions: %#v", got)
	}
}

func TestFilterPartitionsAndDedupe(t *testing.T) {
	partitions := []string{"dev", "qa", "prod"}
	if got := FilterPartitions(partitions, []string{"prod", "dev"}); !reflect.DeepEqual(got, []string{"dev", "prod"}) {
		t.Fatalf("unexpected filtered partitions: %#v", got)
	}
	if got := FilterPartitions(partitions, []string{""}); !reflect.DeepEqual(got, partitions) {
		t.Fatalf("expected blank filters to keep all partitions: %#v", got)
	}
	if got := DedupeSortedStrings([]string{"a", "a", "b", "b", "c"}); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("unexpected deduped sorted strings: %#v", got)
	}
	if got := DedupeStringsPreserveOrder([]string{"a", "", "b", "a", "c", "b"}); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("unexpected deduped strings: %#v", got)
	}
}
