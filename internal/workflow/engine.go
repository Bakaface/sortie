package workflow

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/aface/ralph-tamer-kit/internal/claude"
	"github.com/aface/ralph-tamer-kit/internal/config"
	"github.com/aface/ralph-tamer-kit/internal/db"
	gitpkg "github.com/aface/ralph-tamer-kit/internal/git"
	"github.com/aface/ralph-tamer-kit/internal/notify"
	"github.com/aface/ralph-tamer-kit/internal/task"
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
func (e *Engine) RunTask(ctx context.Context, t *task.Task) error {
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

		// Spawn Claude process
		exitCode, outputTail, err := e.runClaudeStep(ctx, t, step, resolvedPrompt, env)
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

		// Validate that the step produced meaningful changes (skip for review steps)
		if !step.ApprovalRequired {
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
		if step.ApprovalRequired && i < len(steps)-1 {
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
func (e *Engine) ResumeAfterApproval(ctx context.Context, t *task.Task) error {
	return e.RunTask(ctx, t)
}

func (e *Engine) runClaudeStep(ctx context.Context, t *task.Task, step config.StepConfig, prompt string, envVars map[string]string) (int, string, error) {
	proc := claude.NewProcess(fmt.Sprintf("%d", t.ID), t.WorktreePath, &e.cfg.Claude)

	// Apply step timeout
	timeout := e.cfg.GetStepTimeout(step)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

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
				var outputTail string
				if exitCode != 0 {
					if lines, err := proc.CaptureOutput(20); err == nil && len(lines) > 0 {
						// Take the last 20 lines as context
						outputTail = strings.Join(lines, "\n")
					}
				}
				return exitCode, outputTail, nil
			}
		}
	}
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
