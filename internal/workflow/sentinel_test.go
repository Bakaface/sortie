package workflow

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeSentinel writes a sentinel file into a worktree's step-done dir and
// returns the worktree root.
func writeSentinel(t *testing.T, worktree, name, body string) {
	t.Helper()
	dir := StepDoneDir(worktree)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir step-done: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0644); err != nil {
		t.Fatalf("write sentinel %q: %v", name, err)
	}
}

func TestStepSentinelExists_MatchesStepByName(t *testing.T) {
	worktree := t.TempDir()
	writeSentinel(t, worktree, "implementing-1234567890.json", `{}`)

	if !StepSentinelExists(worktree, "implementing") {
		t.Errorf("expected implementing sentinel to be detected")
	}
	if StepSentinelExists(worktree, "reviewing") {
		t.Errorf("a sentinel for a different step must not match")
	}
}

func TestStepSentinelExists_NoDir(t *testing.T) {
	if StepSentinelExists(t.TempDir(), "implementing") {
		t.Errorf("expected no sentinel when step-done dir is missing")
	}
}

func TestStepSentinelExists_IgnoresDotfiles(t *testing.T) {
	worktree := t.TempDir()
	// The hook writes its in-flight temp file as `.<pid>.<ts>.tmp` before the
	// atomic rename; it must never read as a completed sentinel.
	writeSentinel(t, worktree, ".999.123.tmp", "x")
	if StepSentinelExists(worktree, "implementing") {
		t.Errorf("dotfile temp must be ignored")
	}
}

// TestSentinelMatchesStep_PrefixSibling guards the disambiguation between a step
// and a longer sibling that shares its prefix: "reviewing" must not claim
// "reviewing-tests" sentinels.
func TestSentinelMatchesStep_PrefixSibling(t *testing.T) {
	worktree := t.TempDir()
	writeSentinel(t, worktree, "reviewing-tests-1234567890.json", `{}`)

	if StepSentinelExists(worktree, "reviewing") {
		t.Errorf("step %q must not match sibling %q sentinel", "reviewing", "reviewing-tests")
	}
	if !StepSentinelExists(worktree, "reviewing-tests") {
		t.Errorf("step %q must match its own sentinel", "reviewing-tests")
	}
}

// TestSentinelMatchesStep_BsdTimestamp covers the BSD-date case where %N is
// unsupported and the timestamp carries a trailing literal "N".
func TestSentinelMatchesStep_BsdTimestamp(t *testing.T) {
	worktree := t.TempDir()
	writeSentinel(t, worktree, "implementing-1718721960N.json", `{}`)
	if !StepSentinelExists(worktree, "implementing") {
		t.Errorf("expected sentinel with BSD-style timestamp to match")
	}
}

func TestLatestStepSentinel_ParsesPayloadAndPicksNewest(t *testing.T) {
	worktree := t.TempDir()
	writeSentinel(t, worktree, "implementing-100.json", `{"session_id":"old","transcript_path":"/old.jsonl"}`)
	// Make the second sentinel strictly newer by mtime.
	writeSentinel(t, worktree, "implementing-200.json", `{"session_id":"new","transcript_path":"/new.jsonl","cwd":"/wt"}`)
	newer := filepath.Join(StepDoneDir(worktree), "implementing-200.json")
	future := time.Now().Add(time.Second)
	if err := os.Chtimes(newer, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	got, ok := LatestStepSentinel(worktree, "implementing")
	if !ok {
		t.Fatalf("expected a sentinel")
	}
	if got.SessionID != "new" {
		t.Errorf("expected newest session id %q, got %q", "new", got.SessionID)
	}
	if got.TranscriptPath != "/new.jsonl" {
		t.Errorf("expected transcript path %q, got %q", "/new.jsonl", got.TranscriptPath)
	}
}

func TestLatestStepSentinel_NoneOrUnparseable(t *testing.T) {
	worktree := t.TempDir()
	if _, ok := LatestStepSentinel(worktree, "implementing"); ok {
		t.Errorf("expected ok=false when no sentinel exists")
	}
	writeSentinel(t, worktree, "implementing-1.json", `not json`)
	if _, ok := LatestStepSentinel(worktree, "implementing"); ok {
		t.Errorf("expected ok=false when sentinel is unparseable")
	}
}

// TestClearStepSentinels_ScopedToStep verifies only the named step's sentinels
// are removed; other steps' markers survive.
func TestClearStepSentinels_ScopedToStep(t *testing.T) {
	worktree := t.TempDir()
	writeSentinel(t, worktree, "implementing-100.json", `{}`)
	writeSentinel(t, worktree, "implementing-200.json", `{}`)
	writeSentinel(t, worktree, "grilling-100.json", `{}`)

	ClearStepSentinels(worktree, "implementing")

	if StepSentinelExists(worktree, "implementing") {
		t.Errorf("expected implementing sentinels to be cleared")
	}
	if !StepSentinelExists(worktree, "grilling") {
		t.Errorf("grilling sentinel must survive a scoped clear of another step")
	}
}

func TestClearStepSentinels_NoDirIsHarmless(t *testing.T) {
	ClearStepSentinels(t.TempDir(), "implementing") // must not panic
}
