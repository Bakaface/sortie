package tmux

import (
	"testing"
)

func TestNewSession(t *testing.T) {
	s := NewSession("42", "/tmp/work")
	if s.Name != "sortie-42" {
		t.Errorf("expected name sortie-42, got %s", s.Name)
	}
	if s.WorkDir != "/tmp/work" {
		t.Errorf("expected workdir /tmp/work, got %s", s.WorkDir)
	}
}

func TestNewStepSession(t *testing.T) {
	s := NewStepSession("42", "implement", "/tmp/work")
	if s.Name != "sortie-42-implement" {
		t.Errorf("expected name sortie-42-implement, got %s", s.Name)
	}
}

func TestExtractTaskID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"task only", "sortie-42", "42"},
		{"task with step", "sortie-42-implement", "42"},
		{"task with multi-word step", "sortie-7-code-review", "7"},
		{"no prefix", "other-session", "other-session"},
		{"prefix only", "sortie-", ""},
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
