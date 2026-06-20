package workflow

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Bakaface/sortie/internal/claude"
	"github.com/Bakaface/sortie/internal/config"
	"github.com/Bakaface/sortie/internal/task"
	"github.com/Bakaface/sortie/internal/tmux"
)

// buildTmuxClaudeCmd returns the claude command-line fragment used inside the
// generated tmux wrapper script. The binary path is shell-quoted via %q so paths
// with spaces don't break the script.
//
// settingsPath, if non-empty, is appended as `--settings <path>` so the
// worktree-scoped Stop hook (see InstallStopHook) is layered on top of the
// user's global config. We deliberately do NOT set CLAUDE_CONFIG_DIR here:
// that env var is a full redirection of the entire Claude Code config
// directory, which would hide ~/.claude/.credentials.json and ~/.claude.json
// (OAuth, onboarding state, per-project trust acceptance) from the spawned
// agent and force re-onboarding on every launch.
func buildTmuxClaudeCmd(claudeBin string, yolo bool, settingsPath string, defaultArgs []string) string {
	if claudeBin == "" {
		claudeBin = "claude"
	}
	cmd := fmt.Sprintf("%q", claudeBin)
	if yolo {
		cmd += " --dangerously-skip-permissions"
	}
	// Configured default_args (e.g. --plugin-dir for the sortie plugin) must
	// apply to interactive tmux steps too, not just headless ones — otherwise
	// the chat launches without sortie's MCP tools (update_step_context, etc.).
	for _, a := range defaultArgs {
		cmd += fmt.Sprintf(" %q", a)
	}
	if settingsPath != "" {
		cmd += fmt.Sprintf(" --settings %q", settingsPath)
	}
	return cmd
}

func (e *Engine) runClaudeStep(ctx context.Context, t *task.Task, step config.StepConfig, prompt string, envVars map[string]string, outputFn func([]string), systemPrompt ...string) (int, string, string, string, error) {
	proc := claude.NewProcess(fmt.Sprintf("%d", t.ID), t.WorktreePath, &e.cfg.Claude)

	// Apply step timeout
	timeout := e.cfg.GetStepTimeout(step)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Open the unified task log file. Every step and the finalization phase append
	// into this single file so the on-disk order matches the chronological order
	// of events.
	logPath := ProjectLogPath(e.dataDir, t.ID)
	if err := os.MkdirAll(ProjectLogsDir(e.dataDir, t.ID), 0755); err != nil {
		return 1, "", "", "", fmt.Errorf("failed to create log dir: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return 1, "", "", "", fmt.Errorf("failed to open task log: %w", err)
	}
	defer logFile.Close()

	// Write step header and prompt to log file and outputFn
	iterSuffix := ""
	if t.LoopIteration > 0 {
		iterSuffix = fmt.Sprintf(" [iteration %d]", t.LoopIteration)
	}
	header := fmt.Sprintf("[%s] === Step: %s (task #%d)%s ===",
		time.Now().Format("15:04:05"), step.Name, t.ID, iterSuffix)
	promptHeader := fmt.Sprintf("[%s] Prompt:", time.Now().Format("15:04:05"))
	var promptLines []string
	promptLines = append(promptLines, header)
	promptLines = append(promptLines, promptHeader)
	for _, line := range strings.Split(prompt, "\n") {
		promptLines = append(promptLines, fmt.Sprintf("[%s]   %s", time.Now().Format("15:04:05"), line))
	}
	promptLines = append(promptLines, "")

	for _, line := range promptLines {
		logFile.WriteString(line + "\n")
	}
	if outputFn != nil {
		outputFn(promptLines)
	}

	// Compose OutputFunc: write to log file AND call the agent's outputFn
	var logMu sync.Mutex
	proc.OutputFunc = func(lines []string) {
		logMu.Lock()
		for _, line := range lines {
			logFile.WriteString(line + "\n")
		}
		logMu.Unlock()

		if outputFn != nil {
			outputFn(lines)
		}
	}

	// Set environment on the child process (not the daemon's global env)
	proc.SetEnv(envVars)

	if err := proc.StartWithPrompt(prompt, systemPrompt...); err != nil {
		return 1, "", "", "", fmt.Errorf("failed to start claude: %w", err)
	}

	// Wait for process to exit
	ticker := time.NewTicker(processExitPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			proc.Stop()
			return 1, "", "", "", ctx.Err()
		case <-ticker.C:
			if proc.HasExited() {
				exitCode := proc.ExitCode()
				resultText := proc.ResultText()

				// Write step footer
				footer := fmt.Sprintf("[%s] === Step %s finished (exit=%d) ===",
					time.Now().Format("15:04:05"), step.Name, exitCode)
				logMu.Lock()
				logFile.WriteString(footer + "\n")
				logMu.Unlock()
				if outputFn != nil {
					outputFn([]string{footer})
				}

				var outputTail string
				if exitCode != 0 {
					// Read last 20 lines from the per-step log (not raw JSON)
					if lines, err := readLastLines(logPath, 20); err == nil && len(lines) > 0 {
						outputTail = strings.Join(lines, "\n")
					}
				}
				sessionID := proc.SessionID()
				return exitCode, resultText, sessionID, outputTail, nil
			}
		}
	}
}

// readLastLines reads the last n lines from a file.
func readLastLines(path string, n int) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024) // 1MB buffer for large NDJSON lines
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}

// readLogTail reads the last n lines from a log file.
// Returns empty string if the file doesn't exist or can't be read.
func readLogTail(path string, maxLines int) string {
	lines, err := readLastLines(path, maxLines)
	if err != nil || len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

// runClaudeStepTmux starts a Claude session in a detached tmux session and returns
// immediately. The tmux session persists for the user to attach and interact with.
// The workflow engine treats tmux steps as human steps, so the task will pause
// at tmux status until the user manually approves.
func (e *Engine) runClaudeStepTmux(ctx context.Context, t *task.Task, step config.StepConfig, prompt string, envVars map[string]string, outputFn func([]string), systemPrompt ...string) (int, string, error) {
	if !tmux.IsAvailable() {
		return 1, "", fmt.Errorf("tmux is not installed or not in PATH (required for tmux mode)")
	}

	taskID := fmt.Sprintf("%d", t.ID)
	session := tmux.NewSession(e.cfg.Project.Name, taskID, t.WorktreePath)

	// Kill stale session if exists (handles retries)
	if session.Exists() {
		session.Kill()
	}

	sortieDir := filepath.Join(t.WorktreePath, ".sortie")
	promptFile := filepath.Join(sortieDir, fmt.Sprintf("step-prompt-%s.txt", step.Name))
	scriptFile := filepath.Join(sortieDir, fmt.Sprintf("run-step-%s.sh", step.Name))
	logPath := ProjectLogPath(e.dataDir, t.ID)
	if err := os.MkdirAll(ProjectLogsDir(e.dataDir, t.ID), 0755); err != nil {
		return 1, "", fmt.Errorf("failed to create log dir: %w", err)
	}

	// Install the Claude Code Stop hook so the daemon can detect turn-end
	// events and auto-advance the workflow. The daemon falls back to the
	// hash-stability monitor if the hook never fires (e.g. user disabled
	// hooks via managed-settings policy). Failure here is non-fatal.
	if err := InstallStopHook(t.WorktreePath, step.Name); err != nil {
		log.Printf("Warning: failed to install Claude Stop hook for task #%d step %q: %v", t.ID, step.Name, err)
	}

	// Clear any Stop-hook sentinels left from a previous pass of THIS step.
	// Without this, a stale turn-end marker (e.g. from a per-step retry, or one
	// that survived a daemon restart) would let the monitor auto-advance the
	// freshly-launched session before its agent does any work — handing the next
	// step an empty context. Scoped to the step name so concurrent or earlier
	// steps in the same worktree are untouched.
	ClearStepSentinels(t.WorktreePath, step.Name)

	// Write prompt to file (avoids shell quoting issues)
	if err := os.WriteFile(promptFile, []byte(prompt), 0644); err != nil {
		return 1, "", fmt.Errorf("failed to write prompt file: %w", err)
	}

	// Build env exports for the wrapper script. We do NOT inject
	// CLAUDE_CONFIG_DIR — that env var is a full config-dir redirection that
	// would hide the user's OAuth/onboarding state and trigger re-auth prompts.
	// The Stop-hook settings.json is wired in via `--settings` on the claude
	// command line instead (see buildTmuxClaudeCmd).
	var envExports strings.Builder
	for k, v := range envVars {
		envExports.WriteString(fmt.Sprintf("export %s=%q\n", k, v))
	}

	// Write wrapper script: run Claude interactively, then drop to bash for inspection.
	// Honor cfg.Claude.Command so e2e tests / custom installs route through a stub.
	sortieSettingsFile := filepath.Join(SortieSettingsDir(t.WorktreePath), "settings.json")
	claudeCmd := buildTmuxClaudeCmd(e.cfg.Claude.Command, e.cfg.Claude.Yolo, sortieSettingsFile, e.cfg.Claude.DefaultArgs)
	if len(systemPrompt) > 0 && systemPrompt[0] != "" {
		// Write system prompt to file to avoid shell quoting issues
		sysPromptFile := filepath.Join(sortieDir, fmt.Sprintf("step-sysprompt-%s.txt", step.Name))
		if err := os.WriteFile(sysPromptFile, []byte(systemPrompt[0]), 0644); err != nil {
			return 1, "", fmt.Errorf("failed to write system prompt file: %w", err)
		}
		claudeCmd += fmt.Sprintf(" --system-prompt \"$(cat %q)\"", sysPromptFile)
	}
	var script string
	if strings.TrimSpace(prompt) == "" {
		// Empty prompt: launch Claude as a blank interactive session
		script = fmt.Sprintf("#!/bin/bash\n%s%s\nexec bash\n", envExports.String(), claudeCmd)
	} else {
		script = fmt.Sprintf(`#!/bin/bash
%sPROMPT=$(cat %q)
%s "$PROMPT"
exec bash
`, envExports.String(), promptFile, claudeCmd)
	}

	if err := os.WriteFile(scriptFile, []byte(script), 0755); err != nil {
		return 1, "", fmt.Errorf("failed to write wrapper script: %w", err)
	}

	// Snapshot pre-existing Claude sessions in this workdir BEFORE spawning so
	// the async session-ID poller below can distinguish the one we are about to
	// launch from any unrelated chat the user already has open in the same
	// directory (e.g. an interactive `claude` running in the worktree).
	preExistingSessions := claude.SnapshotSessionsByWorkdir(t.WorktreePath)

	// If the setup command contains {{run_agent}} or {{claude_command}}, the user
	// controls which window/pane runs the agent — create a bare session instead
	// of auto-starting Claude in window 0.
	setupCmd := e.cfg.TmuxSetupCommand
	if tmux.SetupCommandControlsAgent(setupCmd) {
		// Create bare session (just a shell), setup command will place the agent
		if err := session.Create(""); err != nil {
			return 1, "", fmt.Errorf("failed to create tmux session: %w", err)
		}
	} else {
		// Default: create session running the wrapper script in window 0
		if err := session.Create("bash", scriptFile); err != nil {
			return 1, "", fmt.Errorf("failed to create tmux session: %w", err)
		}
	}

	// Run tmux setup command if configured (e.g. create additional windows/panes)
	if setupCmd != "" {
		vars := &tmux.SetupVars{
			ClaudeCommand: claudeCmd,
			RunAgent:      scriptFile,
		}
		if err := session.RunSetupCommand(setupCmd, vars); err != nil {
			log.Printf("Warning: tmux setup command failed: %v", err)
		}
	}

	// Write a clean log message instead of piping raw TUI output via pipe-pane
	logLines := writeTmuxLogMessage(logPath, t.ID, step.Name, session.Name, taskID)
	if outputFn != nil {
		outputFn(logLines)
	}

	log.Printf("Tmux session %q started for task #%d step %q (attach with: sortie attach %s)",
		session.Name, t.ID, step.Name, taskID)

	// Async: discover the freshly-spawned Claude session ID and record it.
	// Filtering against preExistingSessions prevents locking onto an unrelated
	// pre-existing chat in the same worktree.
	go func() {
		sid, _ := claude.FindNewSessionByWorkdir(t.WorktreePath, preExistingSessions, 15*time.Second)
		if sid != "" {
			if err := e.database.UpsertChat(t.ID, step.Name, sid, session.Name); err != nil {
				log.Printf("Warning: failed to upsert chat for tmux task #%d step %q: %v", t.ID, step.Name, err)
			}
		}
	}()

	// Fire-and-forget: return immediately, workflow will pause at approval gate
	return 0, "", nil
}

// writeTmuxLogMessage appends a clean status message to the unified task log for tmux
// steps, replacing the raw TUI output that pipe-pane would capture. Append (rather than
// truncate) so restarts and retries of the tmux step preserve prior history alongside
// the new session marker.
func writeTmuxLogMessage(logPath string, taskID int64, stepName, sessionName, taskIDStr string) []string {
	ts := time.Now().Format("15:04:05")
	lines := []string{
		fmt.Sprintf("[%s] === Step: %s (task #%d) ===", ts, stepName, taskID),
		fmt.Sprintf("[%s] Tmux session %q initiated", ts, sessionName),
		fmt.Sprintf("[%s] Attach with: sortie attach %s", ts, taskIDStr),
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("Warning: failed to write tmux log message: %v", err)
		return lines
	}
	defer logFile.Close()

	for _, line := range lines {
		logFile.WriteString(line + "\n")
	}

	return lines
}
