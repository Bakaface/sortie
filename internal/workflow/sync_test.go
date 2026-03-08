package workflow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyncPathsToWorktreeFile(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// Create a source file
	os.WriteFile(filepath.Join(src, "config.txt"), []byte("hello"), 0644)

	err := SyncPathsToWorktree(src, dst, []string{"config.txt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dst, "config.txt"))
	if err != nil {
		t.Fatalf("failed to read synced file: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("expected 'hello', got %q", string(data))
	}
}

func TestSyncPathsToWorktreeDirectory(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// Create a source directory with nested files
	os.MkdirAll(filepath.Join(src, ".claude", "skills"), 0755)
	os.WriteFile(filepath.Join(src, ".claude", "settings.json"), []byte(`{"key":"val"}`), 0644)
	os.WriteFile(filepath.Join(src, ".claude", "skills", "review.md"), []byte("review skill"), 0755)

	err := SyncPathsToWorktree(src, dst, []string{".claude"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check nested file exists
	data, err := os.ReadFile(filepath.Join(dst, ".claude", "skills", "review.md"))
	if err != nil {
		t.Fatalf("failed to read synced nested file: %v", err)
	}
	if string(data) != "review skill" {
		t.Errorf("expected 'review skill', got %q", string(data))
	}

	// Check settings file
	data, err = os.ReadFile(filepath.Join(dst, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("failed to read synced settings: %v", err)
	}
	if string(data) != `{"key":"val"}` {
		t.Errorf("expected settings content, got %q", string(data))
	}
}

func TestSyncPathsToWorktreePreservesPermissions(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// Create an executable file
	os.WriteFile(filepath.Join(src, "run.sh"), []byte("#!/bin/sh\necho hi"), 0755)

	err := SyncPathsToWorktree(src, dst, []string{"run.sh"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := os.Stat(filepath.Join(dst, "run.sh"))
	if err != nil {
		t.Fatalf("failed to stat synced file: %v", err)
	}
	if info.Mode().Perm()&0100 == 0 {
		t.Error("expected executable permission to be preserved")
	}
}

func TestSyncPathsToWorktreeSkipsMissing(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// Create one file but reference a missing path too
	os.WriteFile(filepath.Join(src, "exists.txt"), []byte("here"), 0644)

	err := SyncPathsToWorktree(src, dst, []string{"nonexistent", "exists.txt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Missing path should be skipped, existing path should be synced
	if _, err := os.Stat(filepath.Join(dst, "nonexistent")); !os.IsNotExist(err) {
		t.Error("expected nonexistent path to be skipped")
	}
	data, _ := os.ReadFile(filepath.Join(dst, "exists.txt"))
	if string(data) != "here" {
		t.Errorf("expected 'here', got %q", string(data))
	}
}

func TestSyncPathsToWorktreeEmpty(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	err := SyncPathsToWorktree(src, dst, nil)
	if err != nil {
		t.Fatalf("unexpected error for nil paths: %v", err)
	}

	err = SyncPathsToWorktree(src, dst, []string{})
	if err != nil {
		t.Fatalf("unexpected error for empty paths: %v", err)
	}
}

func TestSyncPathsToWorktreeOverwritesExisting(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// Create old file in dst
	os.WriteFile(filepath.Join(dst, "file.txt"), []byte("old"), 0644)
	// Create new file in src
	os.WriteFile(filepath.Join(src, "file.txt"), []byte("new"), 0644)

	err := SyncPathsToWorktree(src, dst, []string{"file.txt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dst, "file.txt"))
	if string(data) != "new" {
		t.Errorf("expected 'new', got %q", string(data))
	}
}

func TestSyncPathsToWorktreeNestedPath(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// Create a file nested in directories
	os.MkdirAll(filepath.Join(src, "deep", "path"), 0755)
	os.WriteFile(filepath.Join(src, "deep", "path", "file.txt"), []byte("nested"), 0644)

	err := SyncPathsToWorktree(src, dst, []string{"deep/path/file.txt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dst, "deep", "path", "file.txt"))
	if string(data) != "nested" {
		t.Errorf("expected 'nested', got %q", string(data))
	}
}
