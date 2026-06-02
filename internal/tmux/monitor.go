package tmux

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
)

// Activity represents the detected activity state of a tmux session.
type Activity string

const (
	ActivityIdle    Activity = "idle"
	ActivityWIP     Activity = "wip"
	ActivityUnknown Activity = "unknown"
)

// captureScrollbackLines is how many lines of pane content (visible + recent
// scrollback) the monitor hashes each poll. Enough to capture meaningful
// change while a turn is in flight without hashing the entire history.
const captureScrollbackLines = 50

// MonitorConfig configures the tmux activity monitor.
type MonitorConfig struct {
	PollInterval    time.Duration // how often to check sessions (default: 2s)
	StableThreshold int           // consecutive identical pane captures before declaring idle (default: 6 ≈ 12s)
}

// DefaultMonitorConfig returns sensible defaults for monitoring Claude Code sessions.
func DefaultMonitorConfig() MonitorConfig {
	return MonitorConfig{
		PollInterval:    2 * time.Second,
		StableThreshold: 6,
	}
}

// sessionState tracks the polling state for a single tmux session.
type sessionState struct {
	lastHash     string
	stableCount  int
	lastActivity Activity
}

// Monitor detects idle/wip state of tmux sessions by combining
// content hash stability with idle pattern matching.
type Monitor struct {
	config   MonitorConfig
	sessions map[string]*sessionState
}

// NewMonitor creates a new tmux activity monitor.
func NewMonitor(cfg MonitorConfig) *Monitor {
	return &Monitor{
		config:   cfg,
		sessions: make(map[string]*sessionState),
	}
}

// Check captures the pane content for the given session and determines
// its activity state. Returns the activity and whether it changed from
// the previous check.
func (m *Monitor) Check(session *Session) (Activity, bool) {
	lines, err := session.CapturePane(captureScrollbackLines)
	if err != nil {
		return ActivityUnknown, false
	}

	if len(lines) == 0 {
		return ActivityUnknown, false
	}

	// Compute hash of all captured content
	hash := hashLines(lines)

	state, ok := m.sessions[session.Name]
	if !ok {
		state = &sessionState{lastActivity: ActivityUnknown}
		m.sessions[session.Name] = state
	}

	// Track hash stability
	if hash == state.lastHash {
		state.stableCount++
	} else {
		state.stableCount = 1
		state.lastHash = hash
	}

	// Idle once the pane content has held identical across StableThreshold
	// consecutive polls. This is deliberately UI-agnostic: while a Claude
	// agent is working it continuously repaints the pane (spinner frames,
	// elapsed timer, token counter), so a static capture means the turn has
	// ended. We do NOT scrape for a prompt glyph — that coupled idle
	// detection to Claude Code's UI and silently broke when the prompt
	// changed (╰─> → ❯), leaving idle panes stuck reporting "wip".
	var activity Activity
	if state.stableCount >= m.config.StableThreshold {
		activity = ActivityIdle
	} else {
		activity = ActivityWIP
	}

	changed := activity != state.lastActivity
	state.lastActivity = activity

	return activity, changed
}

// Remove cleans up tracking state for a session that has ended.
func (m *Monitor) Remove(sessionName string) {
	delete(m.sessions, sessionName)
}

// Sessions returns the set of tracked session names (for cleanup).
func (m *Monitor) Sessions() map[string]*sessionState {
	return m.sessions
}

// hashLines computes a SHA-256 hash of the joined lines.
func hashLines(lines []string) string {
	h := sha256.New()
	h.Write([]byte(strings.Join(lines, "\n")))
	return fmt.Sprintf("%x", h.Sum(nil))
}
