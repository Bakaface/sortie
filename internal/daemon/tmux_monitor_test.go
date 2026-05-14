package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aface/sortie/internal/workflow"
)

// TestStopHookSentinelExists_FindsRealFile verifies the sentinel detector
// returns true when a regular file lives in step-done/.
func TestStopHookSentinelExists_FindsRealFile(t *testing.T) {
	worktree := t.TempDir()
	stepDone := workflow.StepDoneDir(worktree)
	if err := os.MkdirAll(stepDone, 0755); err != nil {
		t.Fatalf("mkdir step-done: %v", err)
	}
	path := filepath.Join(stepDone, "implementing-1234567890.json")
	if err := os.WriteFile(path, []byte(`{}`), 0644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	if !stopHookSentinelExists(worktree) {
		t.Errorf("expected sentinel to be detected")
	}
}

// TestStopHookSentinelExists_NoDir returns false when step-done does not
// exist yet (e.g. hook never installed).
func TestStopHookSentinelExists_NoDir(t *testing.T) {
	worktree := t.TempDir()
	if stopHookSentinelExists(worktree) {
		t.Errorf("expected no sentinel when step-done dir missing")
	}
}

// TestStopHookSentinelExists_IgnoresDotfiles verifies the hook's in-flight
// `.tmp` files don't masquerade as completed sentinels.
func TestStopHookSentinelExists_IgnoresDotfiles(t *testing.T) {
	worktree := t.TempDir()
	stepDone := workflow.StepDoneDir(worktree)
	if err := os.MkdirAll(stepDone, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stepDone, ".inflight.tmp"), []byte("x"), 0644); err != nil {
		t.Fatalf("write tmp: %v", err)
	}
	if stopHookSentinelExists(worktree) {
		t.Errorf("dotfile should be ignored")
	}
}

// TestConsumeSentinels_RemovesAllRegularFiles verifies sentinels are cleared
// after a successful advance attempt so they don't trigger again.
func TestConsumeSentinels_RemovesAllRegularFiles(t *testing.T) {
	worktree := t.TempDir()
	stepDone := workflow.StepDoneDir(worktree)
	if err := os.MkdirAll(stepDone, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, name := range []string{"a.json", "b.json", "c.json"} {
		if err := os.WriteFile(filepath.Join(stepDone, name), []byte(`{}`), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	consumeSentinels(worktree)

	entries, err := os.ReadDir(stepDone)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() && e.Name()[0] != '.' {
			t.Errorf("expected sentinel %q to be removed", e.Name())
		}
	}
}

// TestConsumeSentinels_NoDirIsHarmless ensures we don't blow up when the
// hook was never installed (e.g. user disabled tmux mode after creating the
// task in an older Sortie build).
func TestConsumeSentinels_NoDirIsHarmless(t *testing.T) {
	consumeSentinels(t.TempDir())
}
