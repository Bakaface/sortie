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
	got := buildTmuxClaudeCmd("/tmp/stub-claude", false, "", nil)
	if !strings.Contains(got, "/tmp/stub-claude") {
		t.Errorf("expected configured path in command, got %q", got)
	}
	if strings.Contains(got, "--dangerously-skip-permissions") {
		t.Errorf("expected no yolo flag when yolo=false, got %q", got)
	}
}

func TestBuildTmuxClaudeCmd_FallsBackToClaude(t *testing.T) {
	got := buildTmuxClaudeCmd("", false, "", nil)
	if got != `"claude"` {
		t.Errorf("expected fallback %q, got %q", `"claude"`, got)
	}
}

func TestBuildTmuxClaudeCmd_YoloAddsFlag(t *testing.T) {
	got := buildTmuxClaudeCmd("/tmp/stub", true, "", nil)
	if !strings.Contains(got, "--dangerously-skip-permissions") {
		t.Errorf("expected --dangerously-skip-permissions, got %q", got)
	}
}

// TestBuildTmuxClaudeCmd_IncludesDefaultArgs verifies configured default_args
// (e.g. --plugin-dir for the sortie plugin) reach interactive tmux steps, so
// the chat launches with sortie's MCP tools available. Regression guard for
// tmux/resume steps silently dropping the plugin and losing update_step_context.
func TestBuildTmuxClaudeCmd_IncludesDefaultArgs(t *testing.T) {
	got := buildTmuxClaudeCmd("/tmp/stub", true, "", []string{"--plugin-dir", "/path with spaces/plugin"})
	if !strings.Contains(got, `"--plugin-dir" "/path with spaces/plugin"`) {
		t.Errorf("expected quoted --plugin-dir default args in command, got %q", got)
	}
}

// TestBuildTmuxClaudeCmd_SettingsFlagWiresStopHook verifies that the worktree
// settings.json path is appended as `--settings <path>` so the Stop hook is
// loaded additively, without redirecting the entire Claude config dir (which
// would hide OAuth/onboarding state). Regression guard for the
// CLAUDE_CONFIG_DIR re-auth bug.
func TestBuildTmuxClaudeCmd_SettingsFlagWiresStopHook(t *testing.T) {
	got := buildTmuxClaudeCmd("/tmp/stub", false, "/wt/.sortie/claude-settings/settings.json", nil)
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
	claudeCmd := buildTmuxClaudeCmd("/path with spaces/claude", true, settingsFile, []string{"--plugin-dir", "/plugins/sortie"})
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
