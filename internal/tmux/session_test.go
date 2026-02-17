package tmux

import (
	"testing"
)

func TestNewSession(t *testing.T) {
	s := NewSession("42", "/tmp/work")
	if s.Name != "ralph-tamer-kit-42" {
		t.Errorf("expected name ralph-tamer-kit-42, got %s", s.Name)
	}
	if s.WorkDir != "/tmp/work" {
		t.Errorf("expected workdir /tmp/work, got %s", s.WorkDir)
	}
}

func TestNewStepSession(t *testing.T) {
	s := NewStepSession("42", "implement", "/tmp/work")
	if s.Name != "ralph-tamer-kit-42-implement" {
		t.Errorf("expected name ralph-tamer-kit-42-implement, got %s", s.Name)
	}
}

func TestExtractTaskID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"task only", "ralph-tamer-kit-42", "42"},
		{"task with step", "ralph-tamer-kit-42-implement", "42"},
		{"task with multi-word step", "ralph-tamer-kit-7-code-review", "7"},
		{"no prefix", "other-session", "other-session"},
		{"prefix only", "ralph-tamer-kit-", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractTaskID(tt.input)
			if got != tt.expected {
				t.Errorf("ExtractTaskID(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
