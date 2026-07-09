package workflow

import (
	"testing"

	"github.com/Bakaface/sortie/internal/config"
	"github.com/Bakaface/sortie/internal/task"
)

// TestDecideInitialStepContext_Precedence is a table-driven test for the pure
// "manual > last_message > none" precedence decision made the instant a
// headless step finishes (see the STEP-CONTEXT LIFECYCLE doc comment in
// stepcontext.go). This is precedence tiers 1-2 of the chain; tier 3
// (summarize_chat) is gated separately by decideSummarizeChat below.
func TestDecideInitialStepContext_Precedence(t *testing.T) {
	tests := []struct {
		name             string
		hasManualContext bool
		manualContext    string
		strategy         string
		resultText       string
		wantSource       stepContextSource
		wantValue        string
		wantHasValue     bool
	}{
		{
			name:             "manual set wins over last_message",
			hasManualContext: true,
			manualContext:    "manually folded artifact",
			strategy:         config.SummarizationStrategySummarizeChat,
			resultText:       "claude result text",
			wantSource:       stepContextSourceManual,
			wantValue:        "manually folded artifact",
			wantHasValue:     true,
		},
		{
			name:             "manual set wins even over strategy none",
			hasManualContext: true,
			manualContext:    "manually folded artifact",
			strategy:         config.SummarizationStrategyNone,
			resultText:       "",
			wantSource:       stepContextSourceManual,
			wantValue:        "manually folded artifact",
			wantHasValue:     true,
		},
		{
			name:             "no manual, non-empty resultText is captured as last_message",
			hasManualContext: false,
			strategy:         config.SummarizationStrategySummarizeChat,
			resultText:       "claude result text",
			wantSource:       stepContextSourceLastMessage,
			wantValue:        "claude result text",
			wantHasValue:     true,
		},
		{
			name:             "no manual, no output, summarize_chat configured: no initial value",
			hasManualContext: false,
			strategy:         config.SummarizationStrategySummarizeChat,
			resultText:       "",
			wantSource:       stepContextSourceNone,
			wantValue:        "",
			wantHasValue:     false,
		},
		{
			name:             "no manual, resultText present but strategy is none: no initial value",
			hasManualContext: false,
			strategy:         config.SummarizationStrategyNone,
			resultText:       "claude result text",
			wantSource:       stepContextSourceNone,
			wantValue:        "",
			wantHasValue:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source, value, hasValue := decideInitialStepContext(tt.hasManualContext, tt.manualContext, tt.strategy, tt.resultText)
			if source != tt.wantSource {
				t.Errorf("source = %v, want %v", source, tt.wantSource)
			}
			if value != tt.wantValue {
				t.Errorf("value = %q, want %q", value, tt.wantValue)
			}
			if hasValue != tt.wantHasValue {
				t.Errorf("hasValue = %v, want %v", hasValue, tt.wantHasValue)
			}
		})
	}
}

// TestDecideSummarizeChat_Precedence covers precedence tier 3: whether a
// summarize_chat pass should even be attempted. A manual override (tier 1)
// always blocks it, regardless of strategy.
func TestDecideSummarizeChat_Precedence(t *testing.T) {
	tests := []struct {
		name             string
		hasManualContext bool
		strategy         string
		want             bool
	}{
		{"manual override blocks summarize_chat", true, config.SummarizationStrategySummarizeChat, false},
		{"no manual + summarize_chat strategy: attempt it", false, config.SummarizationStrategySummarizeChat, true},
		{"no manual + last_message strategy: do not attempt", false, config.SummarizationStrategyLastMessage, false},
		{"no manual + none strategy: do not attempt", false, config.SummarizationStrategyNone, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decideSummarizeChat(tt.hasManualContext, tt.strategy); got != tt.want {
				t.Errorf("decideSummarizeChat(%v, %q) = %v, want %v", tt.hasManualContext, tt.strategy, got, tt.want)
			}
		})
	}
}

// TestReadManualOverride_RowStatusRouting proves readManualOverride reads the
// row-status-correct source: the RUNNING row when pausedTmux is false (a
// headless step still executing), and the COMPLETED row when pausedTmux is
// true (a tmux/human step already paused at its approval gate). This is the
// read-side half of the ROW-STATUS ROUTING invariant in stepcontext.go.
func TestReadManualOverride_RowStatusRouting(t *testing.T) {
	t.Run("pausedTmux=false reads the running row, not the completed row", func(t *testing.T) {
		store := newFakeTaskStore()
		store.runningStepContexts[1] = map[string]string{"implementing": "manual mid-step write"}
		store.stepContexts[1] = map[string]string{"implementing": "stale completed-row value"}
		e := &Engine{database: store}

		value, has, err := e.readManualOverride(1, "implementing", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !has || value != "manual mid-step write" {
			t.Errorf("got (%q, %v), want (%q, true)", value, has, "manual mid-step write")
		}
	})

	t.Run("pausedTmux=true reads the completed row, not the running row", func(t *testing.T) {
		store := newFakeTaskStore()
		store.runningStepContexts[1] = map[string]string{"grilling": "stale running-row value"}
		store.stepContexts[1] = map[string]string{"grilling": "manually folded chat"}
		e := &Engine{database: store}

		value, has, err := e.readManualOverride(1, "grilling", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !has || value != "manually folded chat" {
			t.Errorf("got (%q, %v), want (%q, true)", value, has, "manually folded chat")
		}
	})

	t.Run("blank value reports has=false", func(t *testing.T) {
		store := newFakeTaskStore()
		e := &Engine{database: store}

		if _, has, err := e.readManualOverride(1, "implementing", false); err != nil || has {
			t.Errorf("got has=%v err=%v, want has=false err=nil for an unset row", has, err)
		}
	})
}

// TestPublishManualStepContext_RowStatusRouting proves PublishManualStepContext
// routes a write to exactly one of the two row-status-specific DB writers,
// selected by pausedTmux — the write-side half of the ROW-STATUS ROUTING
// invariant, and the sole place callers (the daemon's
// handleUpdateActiveStepContext) need to consult.
func TestPublishManualStepContext_RowStatusRouting(t *testing.T) {
	t.Run("pausedTmux=false calls the running-row writer only", func(t *testing.T) {
		store := newFakeTaskStore()
		e := &Engine{database: store}

		rows, err := e.PublishManualStepContext(1, "implementing", "canonical artifact", false, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rows != 1 {
			t.Errorf("rows = %d, want 1", rows)
		}
		if len(store.updateRunningCalls) != 1 || len(store.updatePausedCalls) != 0 {
			t.Errorf("expected exactly one running-row write and zero paused-row writes, got running=%d paused=%d",
				len(store.updateRunningCalls), len(store.updatePausedCalls))
		}
	})

	t.Run("pausedTmux=true calls the paused-row writer only", func(t *testing.T) {
		store := newFakeTaskStore()
		e := &Engine{database: store}

		rows, err := e.PublishManualStepContext(1, "grilling", "folded chat", false, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rows != 1 {
			t.Errorf("rows = %d, want 1", rows)
		}
		if len(store.updatePausedCalls) != 1 || len(store.updateRunningCalls) != 0 {
			t.Errorf("expected exactly one paused-row write and zero running-row writes, got running=%d paused=%d",
				len(store.updateRunningCalls), len(store.updatePausedCalls))
		}
	})
}

// TestResolveActiveStep covers the three cases ResolveActiveStep must
// distinguish: a running agent step, a tmux/human step paused at its
// approval gate, and a task with no resolvable active step.
func TestResolveActiveStep(t *testing.T) {
	wf := config.WorkflowConfig{
		Name: "wf",
		Steps: []config.StepConfig{
			{Name: "planning"},
			{Name: "grilling", Human: true},
		},
	}
	cfg := &config.Config{Workflows: []config.WorkflowConfig{wf}}
	e := &Engine{database: newFakeTaskStore(), cfg: newEngineConfig(cfg)}

	t.Run("running agent step: CurrentStep wins, not paused", func(t *testing.T) {
		tk := &task.Task{Workflow: "wf", CurrentStep: "planning", Status: task.StatusRunning}
		name, pausedTmux := e.ResolveActiveStep(tk)
		if name != "planning" || pausedTmux {
			t.Errorf("got (%q, %v), want (%q, false)", name, pausedTmux, "planning")
		}
	})

	t.Run("tmux step paused at approval gate: PausedStep wins, pausedTmux=true", func(t *testing.T) {
		// CurrentStep is cleared and StepIndex bumped past the paused step, per
		// the cursor invariant in cursor.go (index 2 resolves PausedStep to
		// Steps[1] == "grilling").
		tk := &task.Task{Workflow: "wf", CurrentStep: "", StepIndex: 2, Status: task.StatusTmux}
		name, pausedTmux := e.ResolveActiveStep(tk)
		if name != "grilling" || !pausedTmux {
			t.Errorf("got (%q, %v), want (%q, true)", name, pausedTmux, "grilling")
		}
	})

	t.Run("idle task: no active step", func(t *testing.T) {
		tk := &task.Task{Workflow: "wf", CurrentStep: "", Status: task.StatusPending}
		name, pausedTmux := e.ResolveActiveStep(tk)
		if name != "" || pausedTmux {
			t.Errorf("got (%q, %v), want (\"\", false)", name, pausedTmux)
		}
	})
}

// TestRecordTmuxStepSentinelSession_CorrectsFromSentinel proves the sentinel
// path (audit item: tmux_monitor's captureSentinelSession) writes through the
// Engine's taskStore rather than a daemon-level DB call, and that it corrects
// a stale recorded session id.
func TestRecordTmuxStepSentinelSession_CorrectsFromSentinel(t *testing.T) {
	worktree := t.TempDir()
	writeSentinel(t, worktree, "grilling-1234567890.json", `{"session_id":"new-session"}`)

	store := newFakeTaskStore()
	e := &Engine{database: store}
	tk := &task.Task{ID: 1, WorktreePath: worktree}

	// No prior recorded session.
	e.RecordTmuxStepSentinelSession(tk, "grilling")
	got, err := store.GetChatByStep(1, "grilling")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.SessionID != "new-session" {
		t.Fatalf("got %+v, want session id %q", got, "new-session")
	}

	// A second call with the same sentinel content must not error or drop the
	// recorded session (it's a no-op: already correct).
	e.RecordTmuxStepSentinelSession(tk, "grilling")
	got2, _ := store.GetChatByStep(1, "grilling")
	if got2.SessionID != "new-session" {
		t.Errorf("session id changed on idempotent re-call: %q", got2.SessionID)
	}
}
