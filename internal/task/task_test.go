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
		{StatusCompleted, false},
		{StatusFailed, false},
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
		{StatusCompleted, true},
		{StatusFailed, true},
	}

	for _, tt := range tests {
		if got := tt.status.IsTerminal(); got != tt.want {
			t.Errorf("Status(%q).IsTerminal() = %v, want %v", tt.status, got, tt.want)
		}
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
		{StatusCompleted, "completed"},
		{StatusFailed, "failed"},
	}

	for _, tt := range tests {
		if string(tt.status) != tt.value {
			t.Errorf("Status constant has value %q, want %q", tt.status, tt.value)
		}
	}
}
