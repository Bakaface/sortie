package claude

import (
	"sync"
	"testing"
	"time"

	"github.com/aface/ralph-tamer-kit/internal/config"
)

func TestProcessOutputFunc(t *testing.T) {
	// This test requires the claude CLI to be installed and ANTHROPIC_API_KEY set.
	// It's an integration test that verifies the full pipeline:
	// Claude process → stdout pipe → scanner → parser → OutputFunc
	cfg := &config.ClaudeConfig{
		Command:     "claude",
		DefaultArgs: []string{"--dangerously-skip-permissions"},
	}

	workDir := t.TempDir()
	proc := NewProcess("test", workDir, cfg)

	var mu sync.Mutex
	var capturedLines []string
	proc.OutputFunc = func(lines []string) {
		mu.Lock()
		capturedLines = append(capturedLines, lines...)
		mu.Unlock()
	}

	if err := proc.StartWithPrompt("Reply with exactly: HELLO_TEST_OUTPUT"); err != nil {
		t.Fatalf("failed to start: %v", err)
	}

	// Wait for process to exit (max 60s)
	deadline := time.After(60 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			proc.Stop()
			t.Fatal("timed out waiting for claude to exit")
		case <-ticker.C:
			if proc.HasExited() {
				goto done
			}
		}
	}

done:
	mu.Lock()
	defer mu.Unlock()

	t.Logf("exit code: %d", proc.ExitCode())
	t.Logf("captured %d lines:", len(capturedLines))
	for _, l := range capturedLines {
		t.Logf("  %s", l)
	}

	if len(capturedLines) == 0 {
		t.Fatal("OutputFunc was never called — no parsed lines from claude output")
	}

	// Should have at least an assistant turn marker and some text
	foundTurn := false
	foundText := false
	for _, l := range capturedLines {
		if contains(l, "Assistant turn") {
			foundTurn = true
		}
		if contains(l, "HELLO_TEST_OUTPUT") {
			foundText = true
		}
	}
	if !foundTurn {
		t.Error("expected assistant turn marker in output")
	}
	if !foundText {
		t.Error("expected HELLO_TEST_OUTPUT in output")
	}
}
