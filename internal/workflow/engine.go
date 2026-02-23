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

	"github.com/aface/sortie/internal/claude"
	"github.com/aface/sortie/internal/config"
	"github.com/aface/sortie/internal/db"
	gitpkg "github.com/aface/sortie/internal/git"
	"github.com/aface/sortie/internal/notify"
	"github.com/aface/sortie/internal/task"
	"github.com/aface/sortie/internal/tmux"
)

type Engine struct {
	cfg      *config.Config
	database *db.DB
	notifier *notify.Notifier
	repoRoot string
	dataDir  string
	mergeMu  sync.Mutex // serializes merge operations to prevent concurrent git merge conflicts
}

func NewEngine(cfg *config.Config, database *db.DB, notifier *notify.Notifier, repoRoot string) *Engine {
	return &Engine{
		cfg:      cfg,
		database: database,
		notifier: notifier,
		repoRoot: repoRoot,
		dataDir:  filepath.Join(repoRoot, ".sortie"),
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

	// Ensure .sortie directories exist in worktree
	if err := EnsureRTKDirs(t.WorktreePath); err != nil {
		return fmt.Errorf("failed to create sortie dirs: %w", err)
	}

	// Copy attached images to worktree
	var imageRelPaths []string
	if len(t.Images) > 0 {
		var err error
		imageRelPaths, err = CopyImagesToWorktree(t.WorktreePath, t.Images)
		if err != nil {
			log.Printf("Warning: failed to copy images to worktree: %v", err)
		}
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

		// Collect artifacts from prior steps (only those with artifact: true)
		var priorStepNames []string
		for j := 0; j < i; j++ {
			if steps[j].Artifact {
				priorStepNames = append(priorStepNames, steps[j].Name)
			}
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
				Images:      imageRelPaths,
			},
			Artifacts: artifacts,
			Git: GitVars{
				BaseBranch: e.cfg.Git.BaseBranch,
				RepoRoot:   e.repoRoot,
			},
		}

		resolvedPrompt := ResolveTemplate(step.Prompt, tmplCtx)

		// Append artifact output instructions if step has artifact: true
		artifactsDir := ArtifactsDir(t.WorktreePath)
		if step.Artifact {
			artifactPath := filepath.Join(artifactsDir, step.Name+".md")
			resolvedPrompt += fmt.Sprintf("\n\n---\n\nIMPORTANT: When you are done, write a summary of what you did to `%s`. Include: files changed, decisions made, and any issues encountered. This artifact is required for subsequent workflow steps.", artifactPath)
		}
		if err := InjectClaudeMD(t.WorktreePath, resolvedPrompt, imageRelPaths); err != nil {
			return fmt.Errorf("failed to inject CLAUDE.md: %w", err)
		}

		// Set environment variables
		env := map[string]string{
			"SORTIE_TASK_ID":       fmt.Sprintf("%d", t.ID),
			"SORTIE_STEP":          step.Name,
			"SORTIE_WORKTREE":      t.WorktreePath,
			"SORTIE_ARTIFACTS_DIR": artifactsDir,
		}

		// Spawn Claude process (tmux or direct)
		useTmux := step.UseTmux(wf.Tmux)
		var exitCode int
		var outputTail string
		var err error
		if useTmux {
			exitCode, outputTail, err = e.runClaudeStepTmux(ctx, t, step, resolvedPrompt, env, outputFn)
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
		if !step.Human && !useTmux {
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

		// Validate artifact file was written if validate_artifact is enabled
		if step.Artifact && e.cfg.ValidateArtifact {
			artifactPath := filepath.Join(artifactsDir, step.Name+".md")
			if _, err := os.Stat(artifactPath); os.IsNotExist(err) {
				if err := e.database.UpdateTaskStatus(t.ID, task.StatusArtifactMissing); err != nil {
					log.Printf("Warning: failed to set artifact-missing: %v", err)
				}
				return nil
			}
		}

		// Check if approval required before continuing
		needsApproval := step.Human || useTmux
		if needsApproval {
			// Set status to pause the task. The daemon will wait for user action.
			if err := e.database.UpdateTaskStep(t.ID, i+1, ""); err != nil {
				log.Printf("Warning: failed to update task step: %v", err)
			}
			pauseStatus := task.StatusAwaitingApproval
			if useTmux {
				pauseStatus = task.StatusTmux
			}
			if err := e.database.UpdateTaskStatus(t.ID, pauseStatus); err != nil {
				log.Printf("Warning: failed to set %s: %v", pauseStatus, err)
			}
			return nil
		}
	}

	// Run summarizer to generate task context
	if err := e.database.UpdateTaskStatus(t.ID, task.StatusSummarizing); err != nil {
		log.Printf("Warning: failed to set summarizing status for task #%d: %v", t.ID, err)
	}
	if err := e.runSummarizer(ctx, t, wf); err != nil {
		log.Printf("Warning: summarizer failed for task #%d: %v", t.ID, err)
	}

	// All steps completed — execute on_complete action
	if err := e.executeOnComplete(ctx, t, outputFn); err != nil {
		return fmt.Errorf("on_complete action failed: %w", err)
	}

	return nil
}

// executeOnComplete runs the configured on_complete action after all steps finish.
func (e *Engine) executeOnComplete(ctx context.Context, t *task.Task, outputFn func([]string)) error {
	action := e.cfg.Git.OnComplete
	switch action {
	case "", "none":
		return nil

	case "commit":
		return gitpkg.Commit(t.WorktreePath, "sortie: "+t.Title)

	case "merge":
		// Commit any uncommitted changes first (operates on the worktree, safe to do concurrently)
		if err := gitpkg.Commit(t.WorktreePath, "sortie: "+t.Title); err != nil {
			return fmt.Errorf("commit failed: %w", err)
		}
		baseBranch := e.cfg.Git.BaseBranch
		if baseBranch == "" {
			baseBranch = "main"
		}

		const maxMergeAttempts = 3
		var mergeErr error

		for attempt := 1; attempt <= maxMergeAttempts; attempt++ {
			// Lock only for the squash-merge on the shared repoRoot
			e.mergeMu.Lock()
			mergeErr = gitpkg.MergeBranch(e.repoRoot, t.Branch, baseBranch)
			e.mergeMu.Unlock()

			if mergeErr == nil {
				break
			}

			log.Printf("Merge attempt %d/%d failed for task #%d: %v", attempt, maxMergeAttempts, t.ID, mergeErr)

			if attempt == maxMergeAttempts {
				break
			}

			// Update branch outside mutex: merge baseBranch into the worktree branch
			if err := gitpkg.MergeInto(t.WorktreePath, baseBranch); err != nil {
				// Merge produced conflicts — try to resolve with Claude
				conflictFiles, cfErr := gitpkg.GetConflictedFiles(t.WorktreePath)
				if cfErr != nil {
					gitpkg.AbortMerge(t.WorktreePath)
					return fmt.Errorf("merge failed and could not list conflicts: merge: %w; conflict-check: %v", mergeErr, cfErr)
				}

				if len(conflictFiles) == 0 {
					// Merge failed but no conflicted files — something unexpected happened
					gitpkg.AbortMerge(t.WorktreePath)
					return fmt.Errorf("merge into worktree failed with no conflicted files: %w", err)
				}

				log.Printf("Task #%d has %d conflicted files, spawning Claude to resolve", t.ID, len(conflictFiles))

				if resolveErr := e.resolveConflicts(ctx, t, conflictFiles, outputFn); resolveErr != nil {
					gitpkg.AbortMerge(t.WorktreePath)
					return fmt.Errorf("merge conflict resolution failed: %w", resolveErr)
				}

				// Verify all conflicts are resolved
				remaining, cfErr := gitpkg.GetConflictedFiles(t.WorktreePath)
				if cfErr != nil {
					gitpkg.AbortMerge(t.WorktreePath)
					return fmt.Errorf("failed to verify conflict resolution: %w", cfErr)
				}
				if len(remaining) > 0 {
					gitpkg.AbortMerge(t.WorktreePath)
					return fmt.Errorf("claude failed to resolve all conflicts, %d files still conflicted: %v", len(remaining), remaining)
				}

				// Complete the merge commit
				if err := gitpkg.CompleteMerge(t.WorktreePath); err != nil {
					gitpkg.AbortMerge(t.WorktreePath)
					return fmt.Errorf("failed to complete merge after conflict resolution: %w", err)
				}

				log.Printf("Task #%d conflicts resolved, retrying squash-merge", t.ID)
			}
			// If MergeInto succeeded cleanly (no error), the branch is updated — retry the squash-merge
		}

		if mergeErr != nil {
			return fmt.Errorf("merge failed after %d attempts: %w", maxMergeAttempts, mergeErr)
		}

		// Clean up worktree and branch (safe to run concurrently)
		if err := gitpkg.RemoveWorktree(e.repoRoot, t.WorktreePath); err != nil {
			log.Printf("Warning: failed to remove worktree: %v", err)
		}
		if err := gitpkg.ForceDeleteBranch(e.repoRoot, t.Branch); err != nil {
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

// FinalizeTask runs the on_complete action (commit/merge/cleanup) for a task.
// Used when finalizing a tmux-continued task.
func (e *Engine) FinalizeTask(ctx context.Context, t *task.Task) error {
	return e.executeOnComplete(ctx, t, nil)
}

// resolveConflicts spawns a Claude Code agent to resolve merge conflicts in the worktree.
func (e *Engine) resolveConflicts(ctx context.Context, t *task.Task, conflictFiles []string, outputFn func([]string)) error {
	var sb strings.Builder
	sb.WriteString("You are resolving merge conflicts in an automated merge pipeline.\n\n")
	sb.WriteString(fmt.Sprintf("The branch `%s` is being merged into `%s`, and the following files have conflicts:\n\n", e.cfg.Git.BaseBranch, t.Branch))
	for _, f := range conflictFiles {
		sb.WriteString(fmt.Sprintf("- `%s`\n", f))
	}
	sb.WriteString("\nYour job:\n")
	sb.WriteString("1. Open each conflicted file and resolve all `<<<<<<<`, `=======`, `>>>>>>>` conflict markers\n")
	sb.WriteString("2. Choose the correct resolution by understanding both sides of the conflict\n")
	sb.WriteString("3. Run `git add <file>` on each resolved file\n")
	sb.WriteString("4. Do NOT run `git commit` — the merge commit will be created automatically\n")
	sb.WriteString("5. Do NOT modify any files that are not conflicted\n")
	sb.WriteString("6. Verify the code compiles after resolving conflicts (run `go build ./...` or equivalent)\n")
	prompt := sb.String()

	if err := InjectClaudeMD(t.WorktreePath, prompt, nil); err != nil {
		return fmt.Errorf("failed to inject CLAUDE.md for conflict resolution: %w", err)
	}

	step := config.StepConfig{
		Name:    "resolve-conflicts",
		Timeout: "10m",
	}

	env := map[string]string{
		"SORTIE_TASK_ID":  fmt.Sprintf("%d", t.ID),
		"SORTIE_STEP":     step.Name,
		"SORTIE_WORKTREE": t.WorktreePath,
	}

	exitCode, outputTail, err := e.runClaudeStep(ctx, t, step, prompt, env, outputFn)
	if err != nil {
		return fmt.Errorf("conflict resolution claude process failed: %w", err)
	}
	if exitCode != 0 {
		errMsg := fmt.Sprintf("conflict resolution exited with code %d", exitCode)
		if outputTail != "" {
			errMsg += "\n" + outputTail
		}
		return fmt.Errorf("%s", errMsg)
	}

	return nil
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

	// Open per-step log file in project data dir
	logPath := ProjectLogPath(e.dataDir, t.ID, step.Name)
	if err := os.MkdirAll(ProjectLogsDir(e.dataDir, t.ID), 0755); err != nil {
		return 1, "", fmt.Errorf("failed to create log dir: %w", err)
	}
	logFile, err := os.Create(logPath)
	if err != nil {
		return 1, "", fmt.Errorf("failed to create step log: %w", err)
	}
	defer logFile.Close()

	// Write step header and prompt to log file and outputFn
	header := fmt.Sprintf("[%s] === Step: %s (task #%d) ===",
		time.Now().Format("15:04:05"), step.Name, t.ID)
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

// runClaudeStepTmux starts a Claude session in a detached tmux session and returns
// immediately. The tmux session persists for the user to attach and interact with.
// The workflow engine treats tmux steps as human steps, so the task will pause
// at tmux status until the user manually approves.
func (e *Engine) runClaudeStepTmux(ctx context.Context, t *task.Task, step config.StepConfig, prompt string, envVars map[string]string, outputFn func([]string)) (int, string, error) {
	if !tmux.IsAvailable() {
		return 1, "", fmt.Errorf("tmux is not installed or not in PATH (required for tmux mode)")
	}

	taskID := fmt.Sprintf("%d", t.ID)
	session := tmux.NewStepSession(taskID, step.Name, t.WorktreePath)

	// Kill stale session if exists (handles retries)
	if session.Exists() {
		session.Kill()
	}

	sortieDir := filepath.Join(t.WorktreePath, ".sortie")
	promptFile := filepath.Join(sortieDir, fmt.Sprintf("step-prompt-%s.txt", step.Name))
	scriptFile := filepath.Join(sortieDir, fmt.Sprintf("run-step-%s.sh", step.Name))
	logPath := ProjectLogPath(e.dataDir, t.ID, step.Name)
	if err := os.MkdirAll(ProjectLogsDir(e.dataDir, t.ID), 0755); err != nil {
		return 1, "", fmt.Errorf("failed to create log dir: %w", err)
	}

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
	claudeCmd := "claude"
	if e.cfg.Claude.Yolo {
		claudeCmd = "claude --dangerously-skip-permissions"
	}
	script := fmt.Sprintf(`#!/bin/bash
%sPROMPT=$(cat %q)
%s "$PROMPT"
exec bash
`, envExports.String(), promptFile, claudeCmd)

	if err := os.WriteFile(scriptFile, []byte(script), 0755); err != nil {
		return 1, "", fmt.Errorf("failed to write wrapper script: %w", err)
	}

	// Create detached tmux session running the wrapper script
	if err := session.Create("bash", scriptFile); err != nil {
		return 1, "", fmt.Errorf("failed to create tmux session: %w", err)
	}

	// Write a clean log message instead of piping raw TUI output via pipe-pane
	logLines := writeTmuxLogMessage(logPath, t.ID, step.Name, session.Name, taskID)
	if outputFn != nil {
		outputFn(logLines)
	}

	log.Printf("Tmux session %q started for task #%d step %q (attach with: sortie attach %s %s)",
		session.Name, t.ID, step.Name, taskID, step.Name)

	// Fire-and-forget: return immediately, workflow will pause at approval gate
	return 0, "", nil
}

// writeTmuxLogMessage writes a clean status message to the per-step log file for tmux
// steps, replacing the raw TUI output that pipe-pane would capture.
func writeTmuxLogMessage(logPath string, taskID int64, stepName, sessionName, taskIDStr string) []string {
	ts := time.Now().Format("15:04:05")
	lines := []string{
		fmt.Sprintf("[%s] === Step: %s (task #%d) ===", ts, stepName, taskID),
		fmt.Sprintf("[%s] Tmux session %q initiated", ts, sessionName),
		fmt.Sprintf("[%s] Attach with: sortie attach %s %s", ts, taskIDStr, stepName),
	}

	logFile, err := os.Create(logPath)
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

// runSummarizer generates a summary of all artifacts and stores it as the task's context.
func (e *Engine) runSummarizer(ctx context.Context, t *task.Task, wf *config.WorkflowConfig) error {
	// Collect step names that produce artifacts
	var stepNames []string
	for _, s := range wf.Steps {
		if s.Artifact {
			stepNames = append(stepNames, s.Name)
		}
	}

	// Collect all artifacts
	artifacts := CollectArtifacts(t.WorktreePath, stepNames)

	// Get git diff stat as fallback context when no artifacts are available
	var diffStat string
	if len(artifacts) == 0 {
		baseBranch := e.cfg.Git.BaseBranch
		if baseBranch == "" {
			baseBranch = "main"
		}
		var err error
		diffStat, err = gitpkg.DiffStat(t.WorktreePath, baseBranch)
		if err != nil {
			log.Printf("Warning: failed to get diff stat for task #%d: %v", t.ID, err)
		}
		if diffStat == "" {
			log.Printf("No artifacts or changes found for task #%d, skipping summarizer", t.ID)
			return nil
		}
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
	} else if len(artifacts) > 0 {
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
	} else {
		// No artifacts — use git diff stat and instruct Claude to read the actual changes
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Summarize the progress made on task #%d: %s\n\n", t.ID, t.Title))
		sb.WriteString("The task description was:\n")
		sb.WriteString(t.Description)
		sb.WriteString("\n\nThe following files were changed:\n\n```\n")
		sb.WriteString(diffStat)
		sb.WriteString("\n```\n\n")
		sb.WriteString("Read the changed files listed above and review the actual code to understand what was implemented. ")
		sb.WriteString("Do NOT guess or assume — base your summary on the actual file contents and git changes in this repository. ")
		sb.WriteString("Provide a concise summary of what was accomplished. ")
		sb.WriteString("This summary will be used as context for future work on this task.")
		prompt = sb.String()
	}

	log.Printf("Running summarizer for task #%d", t.ID)

	// Run Claude synchronously to capture the summary text
	summary, err := e.runClaudeSync(ctx, prompt, t.WorktreePath)
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
// workDir sets the working directory for the Claude process so it can access
// the task's worktree files.
func (e *Engine) runClaudeSync(ctx context.Context, prompt string, workDir string) (string, error) {
	args := []string{"-p", prompt, "--output-format", "text"}
	args = append(args, e.cfg.Claude.Args()...)

	cmd := exec.CommandContext(ctx, e.cfg.Claude.Command, args...)
	if workDir != "" {
		cmd.Dir = workDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("claude command failed: %w (stderr: %s)", err, stderr.String())
	}

	return stdout.String(), nil
}
