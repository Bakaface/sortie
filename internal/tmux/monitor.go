package tmux

import (
	"crypto/sha256"
	"fmt"
	"regexp"
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

// MonitorConfig configures the tmux activity monitor.
type MonitorConfig struct {
	PollInterval      time.Duration    // how often to check sessions (default: 2s)
	StableThreshold   int              // consecutive identical hashes needed when pattern matches (default: 3)
	FallbackThreshold int              // hash-only threshold when no pattern configured (default: 6)
	IdlePatterns      []*regexp.Regexp // patterns to match against tail lines (default: Claude Code prompt)
	PatternScanLines  int              // number of lines from bottom to scan for patterns (default: 5)
}

// DefaultMonitorConfig returns sensible defaults for monitoring Claude Code sessions.
func DefaultMonitorConfig() MonitorConfig {
	return MonitorConfig{
		PollInterval:      2 * time.Second,
		StableThreshold:   3,
		FallbackThreshold: 6,
		IdlePatterns: []*regexp.Regexp{
			regexp.MustCompile(`╰─>`),          // Claude Code input prompt
			regexp.MustCompile(`\$\s*$`),        // Shell prompt ending with $
			regexp.MustCompile(`>\s*$`),          // Generic prompt ending with >
		},
		PatternScanLines: 5,
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
	lines, err := session.CapturePane(m.config.PatternScanLines + 50)
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

	// Determine activity
	var activity Activity

	hasPatterns := len(m.config.IdlePatterns) > 0
	patternMatched := hasPatterns && m.matchesIdlePattern(tailLines(lines, m.config.PatternScanLines))

	if hasPatterns && state.stableCount >= m.config.StableThreshold && patternMatched {
		activity = ActivityIdle
	} else if !hasPatterns && state.stableCount >= m.config.FallbackThreshold {
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

// matchesIdlePattern checks if any of the configured idle patterns
// match within the given lines.
func (m *Monitor) matchesIdlePattern(lines []string) bool {
	for _, line := range lines {
		for _, pattern := range m.config.IdlePatterns {
			if pattern.MatchString(line) {
				return true
			}
		}
	}
	return false
}

// hashLines computes a SHA-256 hash of the joined lines.
func hashLines(lines []string) string {
	h := sha256.New()
	h.Write([]byte(strings.Join(lines, "\n")))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// tailLines returns the last n lines from a slice.
func tailLines(lines []string, n int) []string {
	if len(lines) <= n {
		return lines
	}
	return lines[len(lines)-n:]
}
