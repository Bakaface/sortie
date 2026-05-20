package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeSessionFile(t *testing.T, dir, name string, sf sessionFile) {
	t.Helper()
	data, err := json.Marshal(sf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestFindNewMatchingSession_FirstMatch(t *testing.T) {
	sessionsDir := t.TempDir()

	writeSessionFile(t, sessionsDir, "session1.json", sessionFile{
		PID:       12345,
		SessionID: "sess-abc",
		Cwd:       "/some/path",
		StartedAt: 1234567890,
	})

	got, err := findNewMatchingSession(sessionsDir, "/some/path", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "sess-abc" {
		t.Errorf("expected session ID %q, got %q", "sess-abc", got)
	}
}

func TestFindNewMatchingSession_NoMatch(t *testing.T) {
	sessionsDir := t.TempDir()

	writeSessionFile(t, sessionsDir, "session1.json", sessionFile{
		PID:       12345,
		SessionID: "sess-abc",
		Cwd:       "/different/path",
		StartedAt: 1234567890,
	})

	got, err := findNewMatchingSession(sessionsDir, "/some/path", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("expected empty string for no match, got %q", got)
	}
}

func TestFindNewMatchingSession_MostRecent(t *testing.T) {
	sessionsDir := t.TempDir()

	writeSessionFile(t, sessionsDir, "session_older.json", sessionFile{
		PID:       11111,
		SessionID: "sess-older",
		Cwd:       "/some/path",
		StartedAt: 1000000000,
	})
	writeSessionFile(t, sessionsDir, "session_newer.json", sessionFile{
		PID:       22222,
		SessionID: "sess-newer",
		Cwd:       "/some/path",
		StartedAt: 2000000000,
	})

	got, err := findNewMatchingSession(sessionsDir, "/some/path", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "sess-newer" {
		t.Errorf("expected most recent session ID %q, got %q", "sess-newer", got)
	}
}

// TestFindNewMatchingSession_SkipsPreExisting is the regression test for the
// bug where sortie locked onto an unrelated pre-existing Claude session that
// happened to be running in the same worktree (because findMatchingSession
// returned the most-recent matching session by cwd alone, with no notion of
// "the session I just spawned"). With a snapshot of pre-existing IDs, that
// session must be skipped even though it matches the cwd.
func TestFindNewMatchingSession_SkipsPreExisting(t *testing.T) {
	sessionsDir := t.TempDir()

	writeSessionFile(t, sessionsDir, "preexisting.json", sessionFile{
		PID:       11111,
		SessionID: "preexisting-id",
		Cwd:       "/work/dir",
		StartedAt: 1000000000,
	})

	existing := SessionSnapshot{"preexisting-id": true}

	// Before the new session shows up, no new match exists.
	got, err := findNewMatchingSession(sessionsDir, "/work/dir", existing)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("expected no new match yet, got %q", got)
	}

	// New sortie-spawned session appears (later startedAt).
	writeSessionFile(t, sessionsDir, "fresh.json", sessionFile{
		PID:       22222,
		SessionID: "fresh-id",
		Cwd:       "/work/dir",
		StartedAt: 2000000000,
	})

	got, err = findNewMatchingSession(sessionsDir, "/work/dir", existing)
	if err != nil {
		t.Fatal(err)
	}
	if got != "fresh-id" {
		t.Errorf("expected new session %q, got %q", "fresh-id", got)
	}
}

// TestFindNewMatchingSession_SkipsPreExistingEvenWhenNewer guards against the
// race where the user's stale session was somehow recorded with a later
// startedAt than the freshly-spawned one — the snapshot must still exclude
// the pre-existing ID regardless of timestamp ordering.
func TestFindNewMatchingSession_SkipsPreExistingEvenWhenNewer(t *testing.T) {
	sessionsDir := t.TempDir()

	writeSessionFile(t, sessionsDir, "stale.json", sessionFile{
		SessionID: "stale-id",
		Cwd:       "/work/dir",
		StartedAt: 9999999999,
	})
	writeSessionFile(t, sessionsDir, "fresh.json", sessionFile{
		SessionID: "fresh-id",
		Cwd:       "/work/dir",
		StartedAt: 1000000000,
	})

	existing := SessionSnapshot{"stale-id": true}

	got, err := findNewMatchingSession(sessionsDir, "/work/dir", existing)
	if err != nil {
		t.Fatal(err)
	}
	if got != "fresh-id" {
		t.Errorf("expected fresh-id (stale excluded), got %q", got)
	}
}

func TestSnapshotSessions(t *testing.T) {
	sessionsDir := t.TempDir()

	writeSessionFile(t, sessionsDir, "match1.json", sessionFile{
		SessionID: "id-1",
		Cwd:       "/work/dir",
		StartedAt: 100,
	})
	writeSessionFile(t, sessionsDir, "match2.json", sessionFile{
		SessionID: "id-2",
		Cwd:       "/work/dir",
		StartedAt: 200,
	})
	writeSessionFile(t, sessionsDir, "other.json", sessionFile{
		SessionID: "id-3",
		Cwd:       "/other/dir",
		StartedAt: 300,
	})

	snap, err := snapshotSessions(sessionsDir, "/work/dir")
	if err != nil {
		t.Fatal(err)
	}
	if len(snap) != 2 || !snap["id-1"] || !snap["id-2"] {
		t.Errorf("expected snapshot {id-1, id-2}, got %v", snap)
	}
}

func TestSnapshotSessions_MissingDir(t *testing.T) {
	snap, err := snapshotSessions(filepath.Join(t.TempDir(), "does-not-exist"), "/work/dir")
	if err != nil {
		t.Fatalf("expected nil error for missing dir, got %v", err)
	}
	if snap == nil || len(snap) != 0 {
		t.Errorf("expected empty non-nil snapshot, got %v", snap)
	}
}
