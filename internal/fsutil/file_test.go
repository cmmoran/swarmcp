package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("ok"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if !FileExists(file) {
		t.Fatalf("expected regular file to exist")
	}
	if FileExists(dir) {
		t.Fatalf("did not expect directory to count as file")
	}
	if FileExists(filepath.Join(dir, "missing.txt")) {
		t.Fatalf("did not expect missing path to exist")
	}
}
