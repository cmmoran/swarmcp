package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAndReadState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	want := State{
		Version:     CurrentVersion,
		GeneratedAt: "2026-03-12T16:00:00Z",
		Command:     "apply",
		ConfigPath:  "/tmp/project.yaml",
		Project:     "demo",
		Deployment:  "prod",
		Partition:   "blue",
		Stack:       "core",
		Plan: PlanSummary{
			ConfigsCreated:  1,
			SecretsCreated:  2,
			ServicesUpdated: 3,
		},
	}
	if err := Write(path, want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		t.Fatalf("expected state file to end with newline")
	}
	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Command != want.Command || got.Project != want.Project || got.Plan.ServicesUpdated != want.Plan.ServicesUpdated {
		t.Fatalf("unexpected round trip state: %#v", got)
	}
}

func TestReadStateErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := Read(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatalf("expected missing file to fail")
	}
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{not-json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := Read(bad); err == nil {
		t.Fatalf("expected invalid json to fail")
	}
}
