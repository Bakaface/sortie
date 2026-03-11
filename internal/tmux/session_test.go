package tmux

import (
	"testing"
)

func TestSanitizeSessionName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"no dots", "myproject", "myproject"},
		{"leading dot", ".docs", "_docs"},
		{"multiple dots", "my.project.name", "my_project_name"},
		{"only dot", ".", "_"},
		{"no change needed", "sortie", "sortie"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeSessionName(tt.input)
			if got != tt.expected {
				t.Errorf("sanitizeSessionName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSessionPrefix(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"plain project", "myproject", "myproject-"},
		{"dot-prefixed project", ".docs", "_docs-"},
		{"project with dots", "my.app", "my_app-"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SessionPrefix(tt.input)
			if got != tt.expected {
				t.Errorf("SessionPrefix(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
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

func TestNewSessionDotPrefix(t *testing.T) {
	s := NewSession(".docs", "7", "/tmp/work")
	if s.Name != "_docs-7" {
		t.Errorf("expected name _docs-7, got %s", s.Name)
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
		{"dot-prefixed project", ".docs", "_docs-42", "42"},
		{"dot-prefixed no match", ".docs", ".docs-42", ".docs-42"},
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
