package apply

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileInputsHashesAndSortsFiles(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.yaml")
	b := filepath.Join(dir, "b.yaml")
	if err := os.WriteFile(a, []byte("alpha"), 0o600); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(b, []byte("beta"), 0o600); err != nil {
		t.Fatalf("write b: %v", err)
	}

	inputs, err := FileInputs("project", []string{b, a})
	if err != nil {
		t.Fatalf("FileInputs: %v", err)
	}
	if len(inputs) != 2 {
		t.Fatalf("expected two inputs, got %#v", inputs)
	}
	if inputs[0].Path != a || inputs[1].Path != b {
		t.Fatalf("expected sorted paths, got %#v", inputs)
	}
	if inputs[0].SHA256 == "" || inputs[1].SHA256 == "" || inputs[0].SHA256 == inputs[1].SHA256 {
		t.Fatalf("unexpected hashes: %#v", inputs)
	}
}
