package workflow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileExistsAndNonEmpty(t *testing.T) {
	dir := t.TempDir()

	// Non-empty file
	nonEmpty := filepath.Join(dir, "nonempty.md")
	os.WriteFile(nonEmpty, []byte("content"), 0644)
	if !fileExistsAndNonEmpty(nonEmpty) {
		t.Error("expected non-empty file to return true")
	}

	// Empty file
	empty := filepath.Join(dir, "empty.md")
	os.WriteFile(empty, []byte(""), 0644)
	if fileExistsAndNonEmpty(empty) {
		t.Error("expected empty file to return false")
	}

	// Missing file
	missing := filepath.Join(dir, "missing.md")
	if fileExistsAndNonEmpty(missing) {
		t.Error("expected missing file to return false")
	}
}
