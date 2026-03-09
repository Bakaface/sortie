package tmux

import (
	"regexp"
	"testing"
)

// testMonitor creates a monitor with test-friendly defaults (no actual tmux calls).
func testMonitor() *Monitor {
	cfg := DefaultMonitorConfig()
	cfg.StableThreshold = 3
	cfg.FallbackThreshold = 6
	cfg.PatternScanLines = 5
	return NewMonitor(cfg)
}

func TestHashLines(t *testing.T) {
	lines1 := []string{"hello", "world"}
	lines2 := []string{"hello", "world"}
	lines3 := []string{"hello", "different"}

	h1 := hashLines(lines1)
	h2 := hashLines(lines2)
	h3 := hashLines(lines3)

	if h1 != h2 {
		t.Error("identical lines should produce identical hashes")
	}
	if h1 == h3 {
		t.Error("different lines should produce different hashes")
	}
}

func TestTailLines(t *testing.T) {
	lines := []string{"a", "b", "c", "d", "e"}

	tail := tailLines(lines, 3)
	if len(tail) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(tail))
	}
	if tail[0] != "c" || tail[1] != "d" || tail[2] != "e" {
		t.Errorf("unexpected tail: %v", tail)
	}

	// When n >= len, return all
	all := tailLines(lines, 10)
	if len(all) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(all))
	}
}

func TestMatchesIdlePattern(t *testing.T) {
	m := testMonitor()

	tests := []struct {
		name    string
		lines   []string
		matches bool
	}{
		{"claude code prompt", []string{"some output", "╰─> "}, true},
		{"shell prompt", []string{"user@host:~$ "}, true},
		{"generic prompt", []string{"some> "}, true},
		{"no prompt", []string{"compiling...", "running tests"}, false},
		{"empty lines", []string{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.matchesIdlePattern(tt.lines); got != tt.matches {
				t.Errorf("matchesIdlePattern() = %v, want %v", got, tt.matches)
			}
		})
	}
}

// simulateCheck simulates the monitor's Check logic using pre-provided lines,
// bypassing actual tmux calls. It duplicates the Check logic but with
// direct line input for testability.
func simulateCheck(m *Monitor, sessionName string, lines []string) (Activity, bool) {
	if len(lines) == 0 {
		return ActivityUnknown, false
	}

	hash := hashLines(lines)

	state, ok := m.sessions[sessionName]
	if !ok {
		state = &sessionState{lastActivity: ActivityUnknown}
		m.sessions[sessionName] = state
	}

	if hash == state.lastHash {
		state.stableCount++
	} else {
		state.stableCount = 1
		state.lastHash = hash
	}

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

func TestChangingContentIsWIP(t *testing.T) {
	m := testMonitor()

	content := [][]string{
		{"compiling main.go..."},
		{"compiling utils.go..."},
		{"linking..."},
	}

	for i, lines := range content {
		activity, _ := simulateCheck(m, "test-session", lines)
		if activity != ActivityWIP {
			t.Errorf("step %d: expected WIP, got %s", i, activity)
		}
	}
}

func TestStableContentWithPatternBecomesIdle(t *testing.T) {
	m := testMonitor()

	idleContent := []string{"some output", "╰─> "}

	// First two checks: stableCount < 3, should be WIP
	for i := 0; i < 2; i++ {
		activity, _ := simulateCheck(m, "test-session", idleContent)
		if activity != ActivityWIP {
			t.Errorf("step %d: expected WIP (building stability), got %s", i, activity)
		}
	}

	// Third check (stableCount=3): transition to idle
	activity, changed := simulateCheck(m, "test-session", idleContent)
	if activity != ActivityIdle {
		t.Errorf("expected idle after stable threshold, got %s", activity)
	}
	if !changed {
		t.Error("expected changed=true on transition to idle")
	}

	// Fourth check: still idle, but changed=false
	activity, changed = simulateCheck(m, "test-session", idleContent)
	if activity != ActivityIdle {
		t.Errorf("expected still idle, got %s", activity)
	}
	if changed {
		t.Error("expected changed=false when staying idle")
	}
}

func TestStableContentWithoutPatternNeedsFallbackThreshold(t *testing.T) {
	m := testMonitor()

	// Content that doesn't match any idle pattern
	noPromptContent := []string{"Done. No errors found."}

	var activity Activity
	for i := 0; i < 5; i++ {
		activity, _ = simulateCheck(m, "test-session", noPromptContent)
		// StableThreshold=3 but pattern doesn't match, so needs FallbackThreshold=6
		if activity != ActivityWIP {
			t.Errorf("step %d: expected WIP (no pattern match, below fallback), got %s", i, activity)
		}
	}

	// At step 5 (stableCount=6), fallback should kick in
	activity, _ = simulateCheck(m, "test-session", noPromptContent)
	// Wait, with patterns configured, fallback doesn't apply. Let's test with no patterns.

	// Test with no patterns configured
	cfg := DefaultMonitorConfig()
	cfg.IdlePatterns = nil
	cfg.FallbackThreshold = 4
	m2 := NewMonitor(cfg)

	staticContent := []string{"Done. No errors found."}
	for i := 0; i < 3; i++ {
		activity, _ = simulateCheck(m2, "test-session", staticContent)
		if activity != ActivityWIP {
			t.Errorf("step %d: expected WIP, got %s", i, activity)
		}
	}
	// 4th identical check should trigger fallback
	activity, _ = simulateCheck(m2, "test-session", staticContent)
	if activity != ActivityIdle {
		t.Errorf("expected idle after fallback threshold, got %s", activity)
	}
}

func TestPatternInEarlyLinesNotTail(t *testing.T) {
	m := testMonitor()
	m.config.PatternScanLines = 2

	// Pattern in early lines, not in the last 2
	content := []string{"╰─> ", "compiling...", "still compiling...", "more output", "building..."}

	var activity Activity
	for i := 0; i < 5; i++ {
		activity, _ = simulateCheck(m, "test-session", content)
	}

	// Pattern is only in early lines, not tail; should stay WIP even if stable
	if activity != ActivityWIP {
		t.Errorf("expected WIP when pattern not in tail, got %s", activity)
	}
}

func TestSessionRemoval(t *testing.T) {
	m := testMonitor()

	simulateCheck(m, "test-session", []string{"hello"})
	if _, ok := m.sessions["test-session"]; !ok {
		t.Error("expected session to be tracked")
	}

	m.Remove("test-session")
	if _, ok := m.sessions["test-session"]; ok {
		t.Error("expected session to be removed")
	}
}

func TestTransitionBackToWIP(t *testing.T) {
	m := testMonitor()

	idleContent := []string{"╰─> "}

	// Reach idle state
	for i := 0; i < 4; i++ {
		simulateCheck(m, "test-session", idleContent)
	}
	activity, _ := simulateCheck(m, "test-session", idleContent)
	if activity != ActivityIdle {
		t.Fatalf("expected idle, got %s", activity)
	}

	// New content arrives → should go back to WIP
	activity, changed := simulateCheck(m, "test-session", []string{"new task starting..."})
	if activity != ActivityWIP {
		t.Errorf("expected WIP after content change, got %s", activity)
	}
	if !changed {
		t.Error("expected changed=true on transition from idle to wip")
	}
}

func TestCustomPatterns(t *testing.T) {
	cfg := DefaultMonitorConfig()
	cfg.IdlePatterns = []*regexp.Regexp{
		regexp.MustCompile(`CUSTOM_PROMPT>`),
	}
	m := NewMonitor(cfg)

	content := []string{"CUSTOM_PROMPT> "}
	for i := 0; i < 4; i++ {
		simulateCheck(m, "test-session", content)
	}

	activity, _ := simulateCheck(m, "test-session", content)
	if activity != ActivityIdle {
		t.Errorf("expected idle with custom pattern, got %s", activity)
	}
}
