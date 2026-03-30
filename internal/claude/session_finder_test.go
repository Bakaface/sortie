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

func TestFindSessionByWorkdir(t *testing.T) {
	sessionsDir := t.TempDir()

	writeSessionFile(t, sessionsDir, "session1.json", sessionFile{
		PID:       12345,
		SessionID: "sess-abc",
		Cwd:       "/some/path",
		StartedAt: 1234567890,
	})

	got, err := findMatchingSession(sessionsDir, "/some/path")
	if err != nil {
		t.Fatal(err)
	}
	if got != "sess-abc" {
		t.Errorf("expected session ID %q, got %q", "sess-abc", got)
	}
}

func TestFindSessionByWorkdir_NoMatch(t *testing.T) {
	sessionsDir := t.TempDir()

	writeSessionFile(t, sessionsDir, "session1.json", sessionFile{
		PID:       12345,
		SessionID: "sess-abc",
		Cwd:       "/different/path",
		StartedAt: 1234567890,
	})

	got, err := findMatchingSession(sessionsDir, "/some/path")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("expected empty string for no match, got %q", got)
	}
}

func TestFindSessionByWorkdir_MostRecent(t *testing.T) {
	sessionsDir := t.TempDir()

	// Two sessions with same cwd but different startedAt
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

	got, err := findMatchingSession(sessionsDir, "/some/path")
	if err != nil {
		t.Fatal(err)
	}
	if got != "sess-newer" {
		t.Errorf("expected most recent session ID %q, got %q", "sess-newer", got)
	}
}
