package templates

import "testing"

func TestJoinCompact(t *testing.T) {
	t.Parallel()

	got := JoinCompact(".", "api", "", "  ", "dev", "example.com")
	want := "api.dev.example.com"
	if got != want {
		t.Fatalf("JoinCompact returned %q, want %q", got, want)
	}
}
