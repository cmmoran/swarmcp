package apply

import "testing"

func TestValidateDeployOutputMode(t *testing.T) {
	valid := []string{"", "auto", "summary", "stack", "error-only", "  ERROR-ONLY  "}
	for _, mode := range valid {
		if err := ValidateDeployOutputMode(mode); err != nil {
			t.Fatalf("expected mode %q to be valid: %v", mode, err)
		}
	}
	if err := ValidateDeployOutputMode("json"); err == nil {
		t.Fatalf("expected invalid mode error")
	}
}

func TestResolveDeployOutputMode(t *testing.T) {
	if got := resolveDeployOutputMode("auto", true, true); got != "summary" {
		t.Fatalf("explicit auto should resolve to summary, got %q", got)
	}
	if got := resolveDeployOutputMode("", true, false); got != "stack" {
		t.Fatalf("no-ui auto should resolve to stack, got %q", got)
	}
	if got := resolveDeployOutputMode("error-only", false, true); got != "error-only" {
		t.Fatalf("explicit mode should be preserved, got %q", got)
	}
}
