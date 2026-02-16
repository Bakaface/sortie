package workflow

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aface/ralph-tamer-kit/internal/claude"
	"github.com/aface/ralph-tamer-kit/internal/config"
	"github.com/aface/ralph-tamer-kit/internal/db"
	gitpkg "github.com/aface/ralph-tamer-kit/internal/git"
	"github.com/aface/ralph-tamer-kit/internal/notify"
	"github.com/aface/ralph-tamer-kit/internal/task"
	"github.com/aface/ralph-tamer-kit/internal/tmux"
)

type Engine struct {
	cfg      *config.Config
	database *db.DB
	notifier *notify.Notifier
	repoRoot string
}

func NewEngine(cfg *config.Config, database *db.DB, notifier *notify.Notifier, repoRoot string) *Engine {
	return &Engine{
		cfg:      cfg,
		database: database,
		notifier: notifier,
		repoRoot: repoRoot,
	}
}

// RunTask executes the full workflow pipeline for a task.
// It creates/reuses the worktree, then loops through steps starting from t.StepIndex.
// outputFn is called with parsed log lines for live streaming (may be nil).
func (e *Engine) RunTask(ctx context.Context, t *task.Task, outputFn func([]string)) error {
	wf := e.cfg.GetWorkflow(t.Workflow)
	steps := wf.Steps

	// Resolve branch name if not set
	if t.Branch == "" {
		t.Branch = e.cfg.ResolveBranchName(t.ID, t.Slug)
	}

	// Create worktree if not already set
	if t.WorktreePath == "" {
		worktree, err := gitpkg.CreateWorktree(e.repoRoot, t.ID, e.cfg.Git.BaseBranch, t.Branch)
		if err != nil {
			return fmt.Errorf("failed to create worktree: %w", err)
		}
		t.WorktreePath = worktree.Path
		if err := e.database.UpdateTaskWorktreePath(t.ID, worktree.Path); err != nil {
			log.Printf("Warning: failed to update worktree path: %v", err)
		}
	}

	// Ensure .rtk directories exist in worktree
	if err := EnsureRTKDirs(t.WorktreePath); err != nil {
		return fmt.Errorf("failed to create rtk dirs: %w", err)
	}

	// Loop through steps from current index
	for i := t.StepIndex; i < len(steps); i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		step := steps[i]

		// Update step tracking
		if err := e.database.UpdateTaskStep(t.ID, i, step.Name); err != nil {
			log.Printf("Warning: failed to update task step: %v", err)
		}

		// Collect artifacts from prior steps
		var priorStepNames []string
		for j := 0; j < i; j++ {
			priorStepNames = append(priorStepNames, steps[j].Name)
		}
		artifacts := CollectArtifacts(t.WorktreePath, priorStepNames)

		// Build template context and resolve prompt
		tmplCtx := &TemplateContext{
			Task: TaskVars{
				ID:          t.ID,
				Title:       t.Title,
				Description: t.Description,
				Slug:        t.Slug,
				Branch:      t.Branch,
			},
			Artifacts: artifacts,
			Git: GitVars{
				BaseBranch: e.cfg.Git.BaseBranch,
				RepoRoot:   e.repoRoot,
			},
		}

		resolvedPrompt := ResolveTemplate(step.Prompt, tmplCtx)

		// Inject CLAUDE.md into worktree
		artifactsDir := ArtifactsDir(t.WorktreePath)
		if err := InjectClaudeMD(t.WorktreePath, resolvedPrompt, step.Name, artifactsDir); err != nil {
			return fmt.Errorf("failed to inject CLAUDE.md: %w", err)
		}

		// Set environment variables
		env := map[string]string{
			"RTK_TASK_ID":       fmt.Sprintf("%d", t.ID),
			"RTK_STEP":          step.Name,
			"RTK_WORKTREE":      t.WorktreePath,
			"RTK_ARTIFACTS_DIR": artifactsDir,
		}

		// Spawn Claude process (tmux or direct)
		useTmux := step.UseTmux(wf.Tmux)
		var exitCode int
		var outputTail string
		var err error
		if useTmux {
			exitCode, outputTail, err = e.runClaudeStepTmux(ctx, t, step, resolvedPrompt, env)
		} else {
			exitCode, outputTail, err = e.runClaudeStep(ctx, t, step, resolvedPrompt, env, outputFn)
		}
		if err != nil {
			e.database.UpdateTaskExitCode(t.ID, 1, err.Error())
			return fmt.Errorf("step %q failed: %w", step.Name, err)
		}

		if exitCode != 0 {
			errMsg := fmt.Sprintf("step %q exited with code %d", step.Name, exitCode)
			if outputTail != "" {
				errMsg += "\n" + outputTail
			}
			e.database.UpdateTaskExitCode(t.ID, exitCode, errMsg)
			return fmt.Errorf("%s", errMsg)
		}

		// Validate that the step produced meaningful changes
		// Skip for review steps and tmux steps (agent may still be working)
		if !step.ApprovalRequired && !useTmux {
			noiseFiles := []string{".claude-output.log", "CLAUDE.md"}
			hasChanges, err := gitpkg.HasMeaningfulChanges(t.WorktreePath, noiseFiles)
			if err != nil {
				log.Printf("Warning: failed to check for changes in step %q: %v", step.Name, err)
			} else if !hasChanges {
				errMsg := fmt.Sprintf("step %q exited successfully but produced no code changes", step.Name)
				e.database.UpdateTaskExitCode(t.ID, 1, errMsg)
				return fmt.Errorf("%s", errMsg)
			}
		}

		// Check if approval required before continuing
		// Tmux steps always require approval (agent runs interactively, user approves when done)
		needsApproval := step.ApprovalRequired || useTmux
		if needsApproval && i < len(steps)-1 {
			// Set to awaiting_approval, the daemon will pause this task
			if err := e.database.UpdateTaskStep(t.ID, i+1, ""); err != nil {
				log.Printf("Warning: failed to update task step: %v", err)
			}
			if err := e.database.UpdateTaskStatus(t.ID, task.StatusAwaitingApproval); err != nil {
				log.Printf("Warning: failed to set awaiting_approval: %v", err)
			}
			return nil
		}
	}

	// Run summarizer to generate task context
	if err := e.runSummarizer(ctx, t, wf); err != nil {
		log.Printf("Warning: summarizer failed for task #%d: %v", t.ID, err)
	}

	// All steps completed — execute on_complete action
	if err := e.executeOnComplete(t); err != nil {
		log.Printf("Warning: on_complete action failed for task #%d: %v", t.ID, err)
	}

	return nil
}

// executeOnComplete runs the configured on_complete action after all steps finish.
func (e *Engine) executeOnComplete(t *task.Task) error {
	action := e.cfg.Git.OnComplete
	switch action {
	case "", "none":
		return nil

	case "commit":
		return gitpkg.Commit(t.WorktreePath, "rtk: "+t.Title)

	case "merge":
		// Commit any uncommitted changes first
		if err := gitpkg.Commit(t.WorktreePath, "rtk: "+t.Title); err != nil {
			return fmt.Errorf("commit failed: %w", err)
		}
		baseBranch := e.cfg.Git.BaseBranch
		if baseBranch == "" {
			baseBranch = "main"
		}
		// Merge happens from the main repo, not the worktree
		if err := gitpkg.MergeBranch(e.repoRoot, t.Branch, baseBranch); err != nil {
			return fmt.Errorf("merge failed: %w", err)
		}
		// Clean up worktree and branch
		if err := gitpkg.RemoveWorktree(e.repoRoot, t.WorktreePath); err != nil {
			log.Printf("Warning: failed to remove worktree: %v", err)
		}
		if err := gitpkg.DeleteBranch(e.repoRoot, t.Branch); err != nil {
			log.Printf("Warning: failed to delete branch: %v", err)
		}
		// Clear worktree path in DB
		if err := e.database.ClearWorktreePath(t.ID); err != nil {
			log.Printf("Warning: failed to clear worktree path: %v", err)
		}
		return nil

	default:
		log.Printf("Unknown on_complete action: %s", action)
		return nil
	}
}

// ResumeAfterApproval resumes a task from its current step index.
func (e *Engine) ResumeAfterApproval(ctx context.Context, t *task.Task, outputFn func([]string)) error {
	return e.RunTask(ctx, t, outputFn)
}

func (e *Engine) runClaudeStep(ctx context.Context, t *task.Task, step config.StepConfig, prompt string, envVars map[string]string, outputFn func([]string)) (int, string, error) {
	proc := claude.NewProcess(fmt.Sprintf("%d", t.ID), t.WorktreePath, &e.cfg.Claude)

	// Apply step timeout
	timeout := e.cfg.GetStepTimeout(step)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Open per-step log file
	logPath := LogPath(t.WorktreePath, step.Name)
	logFile, err := os.Create(logPath)
	if err != nil {
		return 1, "", fmt.Errorf("failed to create step log: %w", err)
	}
	defer logFile.Close()

	// Write step header
	header := fmt.Sprintf("[%s] === Step: %s (task #%d) ===\n",
		time.Now().Format("15:04:05"), step.Name, t.ID)
	logFile.WriteString(header)

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

	if err := proc.StartWithPrompt(prompt); err != nil {
		return 1, "", fmt.Errorf("failed to start claude: %w", err)
	}

	// Wait for process to exit
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			proc.Stop()
			return 1, "", ctx.Err()
		case <-ticker.C:
			if proc.HasExited() {
				exitCode := proc.ExitCode()

				// Write step footer
				footer := fmt.Sprintf("[%s] === Step %s finished (exit=%d) ===\n",
					time.Now().Format("15:04:05"), step.Name, exitCode)
				logMu.Lock()
				logFile.WriteString(footer)
				logMu.Unlock()

				var outputTail string
				if exitCode != 0 {
					// Read last 20 lines from the per-step log (not raw JSON)
					if lines, err := readLastLines(logPath, 20); err == nil && len(lines) > 0 {
						outputTail = strings.Join(lines, "\n")
					}
				}
				return exitCode, outputTail, nil
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

// runClaudeStepTmux starts a Claude session in a detached tmux session and returns
// immediately. The tmux session persists for the user to attach and interact with.
// The workflow engine treats tmux steps as approval_required, so the task will pause
// at awaiting_approval until the user manually approves.
func (e *Engine) runClaudeStepTmux(ctx context.Context, t *task.Task, step config.StepConfig, prompt string, envVars map[string]string) (int, string, error) {
	if !tmux.IsAvailable() {
		return 1, "", fmt.Errorf("tmux is not installed or not in PATH (required for tmux mode)")
	}

	taskID := fmt.Sprintf("%d", t.ID)
	session := tmux.NewStepSession(taskID, step.Name, t.WorktreePath)

	// Kill stale session if exists (handles retries)
	if session.Exists() {
		session.Kill()
	}

	rtkDir := filepath.Join(t.WorktreePath, ".rtk")
	promptFile := filepath.Join(rtkDir, fmt.Sprintf("step-prompt-%s.txt", step.Name))
	scriptFile := filepath.Join(rtkDir, fmt.Sprintf("run-step-%s.sh", step.Name))
	logPath := LogPath(t.WorktreePath, step.Name)

	// Write prompt to file (avoids shell quoting issues)
	if err := os.WriteFile(promptFile, []byte(prompt), 0644); err != nil {
		return 1, "", fmt.Errorf("failed to write prompt file: %w", err)
	}

	// Build env exports for the wrapper script
	var envExports strings.Builder
	for k, v := range envVars {
		envExports.WriteString(fmt.Sprintf("export %s=%q\n", k, v))
	}

	// Write wrapper script: run Claude interactively, then drop to bash for inspection
	script := fmt.Sprintf(`#!/bin/bash
%sPROMPT=$(cat %q)
claude --dangerously-skip-permissions "$PROMPT"
exec bash
`, envExports.String(), promptFile)

	if err := os.WriteFile(scriptFile, []byte(script), 0755); err != nil {
		return 1, "", fmt.Errorf("failed to write wrapper script: %w", err)
	}

	// Create detached tmux session running the wrapper script
	if err := session.Create("bash", scriptFile); err != nil {
		return 1, "", fmt.Errorf("failed to create tmux session: %w", err)
	}

	// Tee pane output to log file (non-fatal if fails)
	if err := session.PipePane(logPath); err != nil {
		log.Printf("Warning: failed to pipe pane output to log: %v", err)
	}

	log.Printf("Tmux session %q started for task #%d step %q (attach with: rtk attach %s %s)",
		session.Name, t.ID, step.Name, taskID, step.Name)

	// Fire-and-forget: return immediately, workflow will pause at approval gate
	return 0, "", nil
}

// runSummarizer generates a summary of all artifacts and stores it as the task's context.
func (e *Engine) runSummarizer(ctx context.Context, t *task.Task, wf *config.WorkflowConfig) error {
	// Collect all step names
	var stepNames []string
	for _, s := range wf.Steps {
		stepNames = append(stepNames, s.Name)
	}

	// Collect all artifacts
	artifacts := CollectArtifacts(t.WorktreePath, stepNames)
	if len(artifacts) == 0 {
		log.Printf("No artifacts found for task #%d, skipping summarizer", t.ID)
		return nil
	}

	// Build the prompt
	var prompt string
	if wf.SummarizerPrompt != "" {
		// Use the configured summarizer prompt with template resolution
		tmplCtx := &TemplateContext{
			Task: TaskVars{
				ID:          t.ID,
				Title:       t.Title,
				Description: t.Description,
				Slug:        t.Slug,
				Branch:      t.Branch,
			},
			Artifacts: artifacts,
			Git: GitVars{
				BaseBranch: e.cfg.Git.BaseBranch,
				RepoRoot:   e.repoRoot,
			},
		}
		prompt = ResolveTemplate(wf.SummarizerPrompt, tmplCtx)
	} else {
		// Build default prompt with all artifacts
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Summarize the progress made on task #%d: %s\n\n", t.ID, t.Title))
		sb.WriteString("Use the context from the following task artifacts:\n\n")
		for _, name := range stepNames {
			if content, ok := artifacts[name]; ok {
				sb.WriteString(fmt.Sprintf("### %s\n\n%s\n\n", name, content))
			}
		}
		sb.WriteString("Provide a concise but comprehensive summary of what was accomplished, ")
		sb.WriteString("any decisions made, and the current state of the implementation. ")
		sb.WriteString("This summary will be used as context for future work on this task.")
		prompt = sb.String()
	}

	log.Printf("Running summarizer for task #%d", t.ID)

	// Run Claude synchronously to capture the summary text
	summary, err := e.runClaudeSync(ctx, prompt)
	if err != nil {
		return fmt.Errorf("summarizer claude invocation failed: %w", err)
	}

	summary = strings.TrimSpace(summary)
	if summary == "" {
		log.Printf("Summarizer produced empty output for task #%d", t.ID)
		return nil
	}

	// Store the context in the database
	if err := e.database.UpdateTaskContext(t.ID, summary); err != nil {
		return fmt.Errorf("failed to store task context: %w", err)
	}

	t.Context = summary
	log.Printf("Summarizer completed for task #%d (%d chars)", t.ID, len(summary))
	return nil
}

// runClaudeSync runs Claude Code synchronously and captures its stdout output.
func (e *Engine) runClaudeSync(ctx context.Context, prompt string) (string, error) {
	args := []string{"-p", prompt, "--output-format", "text"}
	args = append(args, e.cfg.Claude.DefaultArgs...)

	cmd := exec.CommandContext(ctx, e.cfg.Claude.Command, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("claude command failed: %w (stderr: %s)", err, stderr.String())
	}

	return stdout.String(), nil
}
