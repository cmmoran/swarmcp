package render

import (
	"strings"
	"testing"

	"github.com/cmmoran/swarmcp/internal/templates"
)

func TestFormatMountLineMarksInferred(t *testing.T) {
	scope := templates.Scope{Stack: "core", Service: "ingress"}
	line := formatMountLine(scope, "config", "dynamic.yml", "/etc/traefik/dynamic.yml", true)
	if !strings.Contains(line, "(inferred)") {
		t.Fatalf("expected inferred marker in mount line: %q", line)
	}
}

func TestFormatMountLineOmitsInferredForExplicit(t *testing.T) {
	scope := templates.Scope{Stack: "core", Service: "ingress"}
	line := formatMountLine(scope, "config", "dynamic.yml", "/etc/traefik/dynamic.yml", false)
	if strings.Contains(line, "(inferred)") {
		t.Fatalf("did not expect inferred marker in explicit mount line: %q", line)
	}
}
