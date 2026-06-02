package tmux

import (
	"testing"
)

// testMonitor creates a monitor with test-friendly defaults (no actual tmux calls).
func testMonitor() *Monitor {
	cfg := DefaultMonitorConfig()
	cfg.StableThreshold = 3
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

// simulateCheck mirrors the monitor's Check logic using pre-provided lines,
// bypassing actual tmux calls.
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
	if state.stableCount >= m.config.StableThreshold {
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

// TestStableContentBecomesIdle verifies the UI-agnostic rule: identical pane
// content across StableThreshold consecutive polls flips the session to idle,
// regardless of what the content actually is (no prompt-glyph dependency).
func TestStableContentBecomesIdle(t *testing.T) {
	m := testMonitor() // StableThreshold = 3

	// Arbitrary static content — note there is no shell/Claude prompt here.
	idleContent := []string{"Done. No errors found.", "some arbitrary line"}

	// First two checks: stableCount < 3, still WIP.
	for i := 0; i < 2; i++ {
		activity, _ := simulateCheck(m, "test-session", idleContent)
		if activity != ActivityWIP {
			t.Errorf("step %d: expected WIP (building stability), got %s", i, activity)
		}
	}

	// Third check (stableCount=3): transition to idle.
	activity, changed := simulateCheck(m, "test-session", idleContent)
	if activity != ActivityIdle {
		t.Errorf("expected idle after stable threshold, got %s", activity)
	}
	if !changed {
		t.Error("expected changed=true on transition to idle")
	}

	// Fourth check: still idle, but changed=false.
	activity, changed = simulateCheck(m, "test-session", idleContent)
	if activity != ActivityIdle {
		t.Errorf("expected still idle, got %s", activity)
	}
	if changed {
		t.Error("expected changed=false when staying idle")
	}
}

// TestCurrentClaudePromptDetectedAsIdle is a regression guard for the bug that
// motivated dropping prompt matching: a static pane showing the current Claude
// Code prompt (❯, not the old ╰─>) must be reported idle purely on stability.
func TestCurrentClaudePromptDetectedAsIdle(t *testing.T) {
	m := testMonitor() // StableThreshold = 3

	pane := []string{
		"────────────────────────",
		"❯ ",
		"────────────────────────",
		"  170k tok | 17% ctx | abc",
	}

	var activity Activity
	for i := 0; i < 3; i++ {
		activity, _ = simulateCheck(m, "test-session", pane)
	}
	if activity != ActivityIdle {
		t.Errorf("expected idle for stable current-prompt pane, got %s", activity)
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

	idleContent := []string{"idle pane content"}

	// Reach idle state.
	for i := 0; i < 4; i++ {
		simulateCheck(m, "test-session", idleContent)
	}
	activity, _ := simulateCheck(m, "test-session", idleContent)
	if activity != ActivityIdle {
		t.Fatalf("expected idle, got %s", activity)
	}

	// New content arrives → should go back to WIP.
	activity, changed := simulateCheck(m, "test-session", []string{"new task starting..."})
	if activity != ActivityWIP {
		t.Errorf("expected WIP after content change, got %s", activity)
	}
	if !changed {
		t.Error("expected changed=true on transition from idle to wip")
	}
}
