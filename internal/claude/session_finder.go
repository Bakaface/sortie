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

// FindSessionByWorkdir polls Claude Code session files to find a session
// running in the given working directory. It checks at 500ms intervals
// up to maxWait. Returns the most recent matching session ID, or empty string.
func FindSessionByWorkdir(workdir string, maxWait time.Duration) (string, error) {
	deadline := time.Now().Add(maxWait)
	sessionsDir := filepath.Join(os.Getenv("HOME"), ".claude", "sessions")

	for time.Now().Before(deadline) {
		sid, err := findMatchingSession(sessionsDir, workdir)
		if err == nil && sid != "" {
			return sid, nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	return "", nil
}

func findMatchingSession(sessionsDir, workdir string) (string, error) {
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

		if sf.Cwd == workdir && sf.SessionID != "" && sf.StartedAt > bestStarted {
			bestID = sf.SessionID
			bestStarted = sf.StartedAt
		}
	}

	return bestID, nil
}
