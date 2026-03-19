package workflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aface/sortie/internal/config"
)

func TestSyncPathsToWorktreeFile(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// Create a source file
	os.WriteFile(filepath.Join(src, "config.txt"), []byte("hello"), 0644)

	err := SyncPathsToWorktree(src, dst, config.WorktreeSyncPathsConfig{
		Copy: []string{"config.txt"},
	})
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

	err := SyncPathsToWorktree(src, dst, config.WorktreeSyncPathsConfig{
		Copy: []string{".claude"},
	})
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

	err := SyncPathsToWorktree(src, dst, config.WorktreeSyncPathsConfig{
		Copy: []string{"run.sh"},
	})
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

	err := SyncPathsToWorktree(src, dst, config.WorktreeSyncPathsConfig{
		Copy: []string{"nonexistent", "exists.txt"},
	})
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

	err := SyncPathsToWorktree(src, dst, config.WorktreeSyncPathsConfig{})
	if err != nil {
		t.Fatalf("unexpected error for empty config: %v", err)
	}
}

func TestSyncPathsToWorktreeOverwritesExisting(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// Create old file in dst
	os.WriteFile(filepath.Join(dst, "file.txt"), []byte("old"), 0644)
	// Create new file in src
	os.WriteFile(filepath.Join(src, "file.txt"), []byte("new"), 0644)

	err := SyncPathsToWorktree(src, dst, config.WorktreeSyncPathsConfig{
		Copy: []string{"file.txt"},
	})
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

	err := SyncPathsToWorktree(src, dst, config.WorktreeSyncPathsConfig{
		Copy: []string{"deep/path/file.txt"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dst, "deep", "path", "file.txt"))
	if string(data) != "nested" {
		t.Errorf("expected 'nested', got %q", string(data))
	}
}

// --- Link mode tests ---

func TestSyncPathsToWorktreeLinkFile(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	os.WriteFile(filepath.Join(src, "CLAUDE.md"), []byte("instructions"), 0644)

	err := SyncPathsToWorktree(src, dst, config.WorktreeSyncPathsConfig{
		Link: []string{"CLAUDE.md"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be a symlink
	linkDst, err := os.Readlink(filepath.Join(dst, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("expected symlink, got error: %v", err)
	}
	if linkDst != filepath.Join(src, "CLAUDE.md") {
		t.Errorf("expected symlink to %q, got %q", filepath.Join(src, "CLAUDE.md"), linkDst)
	}

	// Content should be readable through the symlink
	data, err := os.ReadFile(filepath.Join(dst, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("failed to read through symlink: %v", err)
	}
	if string(data) != "instructions" {
		t.Errorf("expected 'instructions', got %q", string(data))
	}
}

func TestSyncPathsToWorktreeLinkDirectory(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	os.MkdirAll(filepath.Join(src, ".claude", "skills"), 0755)
	os.WriteFile(filepath.Join(src, ".claude", "settings.json"), []byte(`{"key":"val"}`), 0644)

	err := SyncPathsToWorktree(src, dst, config.WorktreeSyncPathsConfig{
		Link: []string{".claude"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be a symlink
	linkDst, err := os.Readlink(filepath.Join(dst, ".claude"))
	if err != nil {
		t.Fatalf("expected symlink, got error: %v", err)
	}
	if linkDst != filepath.Join(src, ".claude") {
		t.Errorf("expected symlink to %q, got %q", filepath.Join(src, ".claude"), linkDst)
	}

	// Content should be accessible through the symlink
	data, err := os.ReadFile(filepath.Join(dst, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("failed to read through symlinked dir: %v", err)
	}
	if string(data) != `{"key":"val"}` {
		t.Errorf("expected settings content, got %q", string(data))
	}
}

func TestSyncPathsToWorktreeLinkReplacesExisting(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// Create existing file at destination (e.g. from worktree checkout)
	os.WriteFile(filepath.Join(dst, "CLAUDE.md"), []byte("old"), 0644)
	// Create source
	os.WriteFile(filepath.Join(src, "CLAUDE.md"), []byte("new"), 0644)

	err := SyncPathsToWorktree(src, dst, config.WorktreeSyncPathsConfig{
		Link: []string{"CLAUDE.md"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be a symlink now
	if _, err := os.Readlink(filepath.Join(dst, "CLAUDE.md")); err != nil {
		t.Fatalf("expected symlink, got error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dst, "CLAUDE.md"))
	if string(data) != "new" {
		t.Errorf("expected 'new', got %q", string(data))
	}
}

func TestSyncPathsToWorktreeLinkSkipsMissing(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	err := SyncPathsToWorktree(src, dst, config.WorktreeSyncPathsConfig{
		Link: []string{"nonexistent"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Lstat(filepath.Join(dst, "nonexistent")); !os.IsNotExist(err) {
		t.Error("expected nonexistent link to be skipped")
	}
}

func TestSyncPathsToWorktreeLinkNestedPath(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	os.MkdirAll(filepath.Join(src, "deep", "path"), 0755)
	os.WriteFile(filepath.Join(src, "deep", "path", "file.txt"), []byte("nested"), 0644)

	err := SyncPathsToWorktree(src, dst, config.WorktreeSyncPathsConfig{
		Link: []string{"deep/path/file.txt"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	linkDst, err := os.Readlink(filepath.Join(dst, "deep", "path", "file.txt"))
	if err != nil {
		t.Fatalf("expected symlink: %v", err)
	}
	if linkDst != filepath.Join(src, "deep", "path", "file.txt") {
		t.Errorf("expected symlink to source, got %q", linkDst)
	}
}

func TestSyncPathsToWorktreeMixed(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// Set up source files
	os.MkdirAll(filepath.Join(src, "node_modules", "pkg"), 0755)
	os.WriteFile(filepath.Join(src, "node_modules", "pkg", "index.js"), []byte("module"), 0644)
	os.WriteFile(filepath.Join(src, "CLAUDE.md"), []byte("instructions"), 0644)

	err := SyncPathsToWorktree(src, dst, config.WorktreeSyncPathsConfig{
		Copy: []string{"node_modules"},
		Link: []string{"CLAUDE.md"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// node_modules should be copied (not a symlink)
	info, err := os.Lstat(filepath.Join(dst, "node_modules"))
	if err != nil {
		t.Fatalf("failed to stat node_modules: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("expected node_modules to be copied, not symlinked")
	}
	data, _ := os.ReadFile(filepath.Join(dst, "node_modules", "pkg", "index.js"))
	if string(data) != "module" {
		t.Errorf("expected 'module', got %q", string(data))
	}

	// CLAUDE.md should be symlinked
	if _, err := os.Readlink(filepath.Join(dst, "CLAUDE.md")); err != nil {
		t.Fatalf("expected CLAUDE.md to be a symlink: %v", err)
	}
	data, _ = os.ReadFile(filepath.Join(dst, "CLAUDE.md"))
	if string(data) != "instructions" {
		t.Errorf("expected 'instructions', got %q", string(data))
	}
}
