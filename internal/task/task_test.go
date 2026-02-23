package task

import "testing"

func TestStatus_IsActive(t *testing.T) {
	tests := []struct {
		status Status
		want   bool
	}{
		{StatusPending, false},
		{StatusInit, false},
		{StatusRunning, true},
		{StatusAwaitingApproval, true},
		{StatusTmux, true},
		{StatusSummarizing, true},
		{StatusCompleted, false},
		{StatusFailed, false},
		{StatusArtifactMissing, true},
	}

	for _, tt := range tests {
		if got := tt.status.IsActive(); got != tt.want {
			t.Errorf("Status(%q).IsActive() = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		status Status
		want   bool
	}{
		{StatusPending, false},
		{StatusInit, false},
		{StatusRunning, false},
		{StatusAwaitingApproval, false},
		{StatusTmux, false},
		{StatusSummarizing, false},
		{StatusCompleted, true},
		{StatusFailed, true},
		{StatusArtifactMissing, false},
	}

	for _, tt := range tests {
		if got := tt.status.IsTerminal(); got != tt.want {
			t.Errorf("Status(%q).IsTerminal() = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestPriority_Value(t *testing.T) {
	tests := []struct {
		priority Priority
		want     int
	}{
		{PriorityLow, 1},
		{PriorityMedium, 2},
		{PriorityHigh, 3},
		{PriorityUrgent, 4},
		{Priority("unknown"), 2}, // default to medium
	}

	for _, tt := range tests {
		if got := tt.priority.Value(); got != tt.want {
			t.Errorf("Priority(%q).Value() = %d, want %d", tt.priority, got, tt.want)
		}
	}
}

func TestPriority_Ordering(t *testing.T) {
	// Verify that urgent > high > medium > low
	if PriorityUrgent.Value() <= PriorityHigh.Value() {
		t.Error("expected urgent > high")
	}
	if PriorityHigh.Value() <= PriorityMedium.Value() {
		t.Error("expected high > medium")
	}
	if PriorityMedium.Value() <= PriorityLow.Value() {
		t.Error("expected medium > low")
	}
}

func TestIsValidPriority(t *testing.T) {
	valid := []string{"low", "medium", "high", "urgent"}
	for _, s := range valid {
		if !IsValidPriority(s) {
			t.Errorf("expected %q to be a valid priority", s)
		}
	}
	invalid := []string{"", "critical", "none", "URGENT"}
	for _, s := range invalid {
		if IsValidPriority(s) {
			t.Errorf("expected %q to be an invalid priority", s)
		}
	}
}

func TestValidPriorities(t *testing.T) {
	priorities := ValidPriorities()
	if len(priorities) != 4 {
		t.Fatalf("expected 4 priorities, got %d", len(priorities))
	}
	if priorities[0] != PriorityLow || priorities[3] != PriorityUrgent {
		t.Error("priorities should be in ascending order: low, medium, high, urgent")
	}
}

func TestStatusValues(t *testing.T) {
	// Verify the actual string values of status constants
	tests := []struct {
		status Status
		value  string
	}{
		{StatusPending, "pending"},
		{StatusInit, "init"},
		{StatusRunning, "running"},
		{StatusAwaitingApproval, "awaiting-approval"},
		{StatusTmux, "tmux"},
		{StatusSummarizing, "summarizing"},
		{StatusCompleted, "completed"},
		{StatusFailed, "failed"},
		{StatusArtifactMissing, "artifact-missing"},
	}

	for _, tt := range tests {
		if string(tt.status) != tt.value {
			t.Errorf("Status constant has value %q, want %q", tt.status, tt.value)
		}
	}
}
