package tmux

import (
	"testing"
)

func TestSessionPrefix(t *testing.T) {
	got := SessionPrefix("myproject")
	if got != "myproject-" {
		t.Errorf("expected myproject-, got %s", got)
	}
}

func TestNewSession(t *testing.T) {
	s := NewSession("sortie", "42", "/tmp/work")
	if s.Name != "sortie-42" {
		t.Errorf("expected name sortie-42, got %s", s.Name)
	}
	if s.WorkDir != "/tmp/work" {
		t.Errorf("expected workdir /tmp/work, got %s", s.WorkDir)
	}
}

func TestExtractTaskID(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		input       string
		expected    string
	}{
		{"basic", "sortie", "sortie-42", "42"},
		{"no prefix match", "sortie", "other-42", "other-42"},
		{"prefix only", "sortie", "sortie-", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractTaskID(tt.projectName, tt.input)
			if got != tt.expected {
				t.Errorf("ExtractTaskID(%q, %q) = %q, want %q", tt.projectName, tt.input, got, tt.expected)
			}
		})
	}
}
