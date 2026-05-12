package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildClaudeCommand(t *testing.T) {
	tests := []struct {
		name            string
		yolo            bool
		resumeSessionID string
		want            string
	}{
		{
			name: "default",
			want: `"claude"`,
		},
		{
			name: "yolo",
			yolo: true,
			want: `"claude" --dangerously-skip-permissions`,
		},
		{
			name:            "resume",
			resumeSessionID: "abc-123",
			want:            `"claude" --resume abc-123`,
		},
		{
			name:            "yolo and resume",
			yolo:            true,
			resumeSessionID: "abc-123",
			want:            `"claude" --dangerously-skip-permissions --resume abc-123`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildClaudeCommand("", tt.yolo, tt.resumeSessionID)
			if got != tt.want {
				t.Errorf("buildClaudeCommand(%q, %v, %q) = %q, want %q",
					"", tt.yolo, tt.resumeSessionID, got, tt.want)
			}
		})
	}
}

func TestWriteClaudeScript_IncludesResumeFlag(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "run.sh")

	if err := writeClaudeScript(scriptPath, "", false, "sess-xyz"); err != nil {
		t.Fatalf("writeClaudeScript failed: %v", err)
	}

	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("failed to read script: %v", err)
	}

	contents := string(data)
	if !strings.Contains(contents, "--resume sess-xyz") {
		t.Errorf("expected script to contain '--resume sess-xyz', got:\n%s", contents)
	}
}

func TestWriteClaudeScript_NoResumeWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "run.sh")

	if err := writeClaudeScript(scriptPath, "", false, ""); err != nil {
		t.Fatalf("writeClaudeScript failed: %v", err)
	}

	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("failed to read script: %v", err)
	}

	if strings.Contains(string(data), "--resume") {
		t.Errorf("expected script to omit --resume when no session id, got:\n%s", string(data))
	}
}
