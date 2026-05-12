package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteClaudeScript_HonorsClaudeBin(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "run.sh")

	if err := writeClaudeScript(script, "/tmp/stub-claude", false, ""); err != nil {
		t.Fatalf("writeClaudeScript failed: %v", err)
	}

	data, err := os.ReadFile(script)
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	got := string(data)

	if !strings.Contains(got, "/tmp/stub-claude") {
		t.Errorf("expected script to contain configured claude path /tmp/stub-claude\nscript:\n%s", got)
	}
	if !strings.Contains(got, "exec bash") {
		t.Errorf("expected script to drop to bash\nscript:\n%s", got)
	}

	// Script must be syntactically valid bash
	if err := exec.Command("bash", "-n", script).Run(); err != nil {
		t.Errorf("bash -n rejected generated script: %v\nscript:\n%s", err, got)
	}
}

func TestWriteClaudeScript_FallsBackToClaude(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "run.sh")

	if err := writeClaudeScript(script, "", false, ""); err != nil {
		t.Fatalf("writeClaudeScript failed: %v", err)
	}

	data, _ := os.ReadFile(script)
	got := string(data)

	if !strings.Contains(got, `"claude"`) {
		t.Errorf("expected fallback to literal claude\nscript:\n%s", got)
	}
}

func TestWriteClaudeScript_YoloAddsFlag(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "run.sh")

	if err := writeClaudeScript(script, "/tmp/stub", true, ""); err != nil {
		t.Fatalf("writeClaudeScript failed: %v", err)
	}

	data, _ := os.ReadFile(script)
	got := string(data)

	if !strings.Contains(got, "--dangerously-skip-permissions") {
		t.Errorf("expected --dangerously-skip-permissions when yolo\nscript:\n%s", got)
	}
}

func TestWriteClaudeScript_QuotesPathWithSpaces(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "run.sh")

	if err := writeClaudeScript(script, "/path with spaces/claude", false, ""); err != nil {
		t.Fatalf("writeClaudeScript failed: %v", err)
	}

	if err := exec.Command("bash", "-n", script).Run(); err != nil {
		data, _ := os.ReadFile(script)
		t.Errorf("bash -n rejected script with spaced path: %v\nscript:\n%s", err, string(data))
	}
}
