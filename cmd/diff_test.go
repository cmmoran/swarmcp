package cmd

import (
	"strings"
	"testing"
)

func TestDiffLinesTextWithContext(t *testing.T) {
	before := strings.Join([]string{
		"line1",
		"line2",
		"line3",
		"line4",
		"line5",
	}, "\n")
	after := strings.Join([]string{
		"line1",
		"line2",
		"line3-new",
		"line4",
		"line5",
	}, "\n")

	diff, err := diffLinesTextWithContext(before, after, 1)
	if err != nil {
		t.Fatalf("diffLinesTextWithContext failed: %v", err)
	}
	out := strings.Join(diff, "\n")
	if !strings.Contains(out, "  line2") {
		t.Fatalf("expected context line2 in diff:\n%s", out)
	}
	if !strings.Contains(out, "- line3") || !strings.Contains(out, "+ line3-new") {
		t.Fatalf("expected changed lines in diff:\n%s", out)
	}
	if !strings.Contains(out, "  line4") {
		t.Fatalf("expected context line4 in diff:\n%s", out)
	}
}
