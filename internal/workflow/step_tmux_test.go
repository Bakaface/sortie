package workflow

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildTmuxClaudeCmd_HonorsCommand(t *testing.T) {
	got := buildTmuxClaudeCmd("/tmp/stub-claude", false, "")
	if !strings.Contains(got, "/tmp/stub-claude") {
		t.Errorf("expected configured path in command, got %q", got)
	}
	if strings.Contains(got, "--dangerously-skip-permissions") {
		t.Errorf("expected no yolo flag when yolo=false, got %q", got)
	}
}

func TestBuildTmuxClaudeCmd_FallsBackToClaude(t *testing.T) {
	got := buildTmuxClaudeCmd("", false, "")
	if got != `"claude"` {
		t.Errorf("expected fallback %q, got %q", `"claude"`, got)
	}
}

func TestBuildTmuxClaudeCmd_YoloAddsFlag(t *testing.T) {
	got := buildTmuxClaudeCmd("/tmp/stub", true, "")
	if !strings.Contains(got, "--dangerously-skip-permissions") {
		t.Errorf("expected --dangerously-skip-permissions, got %q", got)
	}
}

// TestBuildTmuxClaudeCmd_SettingsFlagWiresStopHook verifies that the worktree
// settings.json path is appended as `--settings <path>` so the Stop hook is
// loaded additively, without redirecting the entire Claude config dir (which
// would hide OAuth/onboarding state). Regression guard for the
// CLAUDE_CONFIG_DIR re-auth bug.
func TestBuildTmuxClaudeCmd_SettingsFlagWiresStopHook(t *testing.T) {
	got := buildTmuxClaudeCmd("/tmp/stub", false, "/wt/.sortie/claude-settings/settings.json")
	want := `--settings "/wt/.sortie/claude-settings/settings.json"`
	if !strings.Contains(got, want) {
		t.Errorf("expected %q in command, got %q", want, got)
	}
}

// Verify the command fragment composes into a syntactically valid bash script
// when concatenated the same way runClaudeStepTmux does.
func TestBuildTmuxClaudeCmd_ScriptIsValidBash(t *testing.T) {
	dir := t.TempDir()
	scriptFile := filepath.Join(dir, "run.sh")
	promptFile := filepath.Join(dir, "prompt.txt")
	if err := os.WriteFile(promptFile, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	settingsFile := filepath.Join(dir, "settings with spaces.json")
	if err := os.WriteFile(settingsFile, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	claudeCmd := buildTmuxClaudeCmd("/path with spaces/claude", true, settingsFile)
	sysPromptFile := filepath.Join(dir, "sys.txt")
	if err := os.WriteFile(sysPromptFile, []byte("sys"), 0644); err != nil {
		t.Fatal(err)
	}
	claudeCmd += fmt.Sprintf(" --system-prompt \"$(cat %q)\"", sysPromptFile)

	script := fmt.Sprintf(`#!/bin/bash
PROMPT=$(cat %q)
%s "$PROMPT"
exec bash
`, promptFile, claudeCmd)

	if err := os.WriteFile(scriptFile, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	if err := exec.Command("bash", "-n", scriptFile).Run(); err != nil {
		t.Errorf("bash -n rejected generated script: %v\nscript:\n%s", err, script)
	}
}
