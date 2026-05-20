package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// sessionFile represents the JSON structure of a Claude Code session file
// found in ~/.claude/sessions/*.json.
type sessionFile struct {
	PID       int    `json:"pid"`
	SessionID string `json:"sessionId"`
	Cwd       string `json:"cwd"`
	StartedAt int64  `json:"startedAt"`
}

// SessionSnapshot is the set of Claude session IDs registered for a workdir
// at a point in time. Used as the "before" baseline when waiting for a
// freshly-spawned session to appear: anything in the snapshot is a
// pre-existing session and must not be returned as the new one.
type SessionSnapshot map[string]bool

// SnapshotSessionsByWorkdir returns the set of session IDs currently registered
// in ~/.claude/sessions/ whose cwd matches workdir. Call this immediately before
// spawning a new Claude session so the result can be passed to
// FindNewSessionByWorkdir to distinguish the new session from any pre-existing
// ones in the same directory. A read error returns an empty (non-nil) snapshot.
func SnapshotSessionsByWorkdir(workdir string) SessionSnapshot {
	sessionsDir := filepath.Join(os.Getenv("HOME"), ".claude", "sessions")
	snap, _ := snapshotSessions(sessionsDir, workdir)
	return snap
}

// FindNewSessionByWorkdir polls Claude Code session files to find a session
// running in the given working directory whose ID is NOT in `existing`. Polls
// at 500ms intervals up to maxWait. Among new (not-in-snapshot) matches it
// returns the one with the latest startedAt. A nil `existing` is treated as
// an empty set (any match wins) — preserves the legacy "first match" behaviour
// for callers that do not need exclusion.
func FindNewSessionByWorkdir(workdir string, existing SessionSnapshot, maxWait time.Duration) (string, error) {
	deadline := time.Now().Add(maxWait)
	sessionsDir := filepath.Join(os.Getenv("HOME"), ".claude", "sessions")

	for {
		sid, err := findNewMatchingSession(sessionsDir, workdir, existing)
		if err == nil && sid != "" {
			return sid, nil
		}
		if !time.Now().Before(deadline) {
			return "", nil
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// snapshotSessions reads sessionsDir and returns the set of session IDs whose
// cwd matches workdir. Returns an empty (non-nil) snapshot when sessionsDir
// does not exist or cannot be read.
func snapshotSessions(sessionsDir, workdir string) (SessionSnapshot, error) {
	snap := SessionSnapshot{}
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return snap, nil
		}
		return snap, err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(sessionsDir, entry.Name()))
		if err != nil {
			continue
		}
		var sf sessionFile
		if err := json.Unmarshal(data, &sf); err != nil {
			continue
		}
		if sf.Cwd == workdir && sf.SessionID != "" {
			snap[sf.SessionID] = true
		}
	}
	return snap, nil
}

// findNewMatchingSession returns the session ID with the latest startedAt
// whose cwd matches workdir AND whose ID is not in `existing`. A nil
// `existing` excludes nothing.
func findNewMatchingSession(sessionsDir, workdir string, existing SessionSnapshot) (string, error) {
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return "", err
	}

	var bestID string
	var bestStarted int64

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(sessionsDir, entry.Name()))
		if err != nil {
			continue
		}

		var sf sessionFile
		if err := json.Unmarshal(data, &sf); err != nil {
			continue
		}

		if sf.Cwd != workdir || sf.SessionID == "" {
			continue
		}
		if existing[sf.SessionID] {
			continue
		}
		if sf.StartedAt > bestStarted {
			bestID = sf.SessionID
			bestStarted = sf.StartedAt
		}
	}

	return bestID, nil
}
