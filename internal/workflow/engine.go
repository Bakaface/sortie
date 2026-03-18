package workflow

import (
	"bufio"
	"bytes"
	"context"
	"errors"
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

const (
	// maxMergeAttempts is the number of times to retry a squash-merge before failing.
	maxMergeAttempts = 3

	// mergeBlockedPollInterval is how often to re-check whether the target branch is clean.
	mergeBlockedPollInterval = 10 * time.Second

	// processExitPollInterval is how often to check whether a Claude subprocess has exited.
	processExitPollInterval = 500 * time.Millisecond
)

type Engine struct {
	cfg      *config.Config
	database *db.DB
	notifier *notify.Notifier
	repoRoot string
	dataDir  string
	mergeMu  *sync.Mutex // serializes merge operations to prevent concurrent git merge conflicts
}

// NewEngine creates a new workflow engine. The mergeMu parameter is an optional
// externally-owned mutex that serializes merge operations for this repo. When
// non-nil, the mutex survives config reloads (the daemon owns it). When nil, a
// fresh mutex is created (for standalone/test usage).
func NewEngine(cfg *config.Config, database *db.DB, notifier *notify.Notifier, repoRoot string, mergeMu ...*sync.Mutex) *Engine {
	var mu *sync.Mutex
	if len(mergeMu) > 0 && mergeMu[0] != nil {
		mu = mergeMu[0]
	} else {
		mu = &sync.Mutex{}
	}
	return &Engine{
		cfg:      cfg,
		database: database,
		notifier: notifier,
		repoRoot: repoRoot,
		dataDir:  filepath.Join(repoRoot, ".sortie"),
		mergeMu:  mu,
	}
}

// findStepIndex returns the index of a step by name, or -1 if not found.
func findStepIndex(steps []config.StepConfig, name string) int {
	for i, s := range steps {
		if s.Name == name {
			return i
		}
	}
	return -1
}

// effectiveBaseBranch returns the base branch for a task, checking the task's
// per-task TargetBranch override first, then falling back to the config's Git.BaseBranch,
// and finally to "main".
func (e *Engine) effectiveBaseBranch(t *task.Task) string {
	if t.TargetBranch != "" {
		return t.TargetBranch
	}
	if e.cfg.Git.BaseBranch != "" {
		return e.cfg.Git.BaseBranch
	}
	return "main"
}

// RunTask executes the full workflow pipeline for a task.
// It creates/reuses the worktree, then loops through steps starting from t.StepIndex.
// outputFn is called with parsed log lines for live streaming (may be nil).
func (e *Engine) RunTask(ctx context.Context, t *task.Task, outputFn func([]string)) error {
	wf := e.cfg.GetWorkflow(t.Workflow)
	steps := wf.Steps

	if t.Worktree {
		if t.CheckoutBranch != "" {
			// Checkout existing branch mode
			t.Branch = t.CheckoutBranch
			if dbErr := e.database.UpdateTaskBranch(t.ID, t.Branch); dbErr != nil {
				log.Printf("Warning: failed to persist branch name for task #%d: %v", t.ID, dbErr)
			}
			if t.WorktreePath == "" {
				worktree, err := gitpkg.CheckoutWorktree(e.repoRoot, t.ID, t.CheckoutBranch)
				if err != nil {
					return fmt.Errorf("failed to checkout worktree: %w", err)
				}
				t.WorktreePath = worktree.Path
				if err := e.database.UpdateTaskWorktreePath(t.ID, worktree.Path); err != nil {
					log.Printf("Warning: failed to update worktree path: %v", err)
				}
			}
		} else {
			// Normal new branch mode
			if t.Branch == "" {
				t.Branch = e.cfg.ResolveBranchForTask(t.ID, t.Title, t.Slug, t.BranchName)
				if dbErr := e.database.UpdateTaskBranch(t.ID, t.Branch); dbErr != nil {
					log.Printf("Warning: failed to persist branch name for task #%d: %v", t.ID, dbErr)
				}
			}
			if t.WorktreePath == "" {
				worktree, err := gitpkg.CreateWorktree(e.repoRoot, t.ID, e.effectiveBaseBranch(t), t.Branch)
				if err != nil {
					return fmt.Errorf("failed to create worktree: %w", err)
				}
				t.WorktreePath = worktree.Path
				if err := e.database.UpdateTaskWorktreePath(t.ID, worktree.Path); err != nil {
					log.Printf("Warning: failed to update worktree path: %v", err)
				}
			}
		}
	} else {
		// No-worktree mode: run in project root directory
		t.WorktreePath = e.repoRoot
	}

	// Sync configured paths from project root into the worktree
	if t.Worktree {
		syncPaths := e.cfg.GetWorktreeSyncPaths(wf)
		if len(syncPaths) > 0 {
			if err := SyncPathsToWorktree(e.repoRoot, t.WorktreePath, syncPaths); err != nil {
				log.Printf("Warning: failed to sync paths to worktree: %v", err)
			}
		}
	}

	// Run worktree setup command if configured
	if t.Worktree {
		if setupCmd := e.cfg.GetWorktreeSetupCommand(wf); setupCmd != "" {
			if err := RunWorktreeSetupCommand(ctx, e.repoRoot, t.WorktreePath, setupCmd); err != nil {
				return fmt.Errorf("worktree setup failed: %w", err)
			}
		}
	}

	// Ensure .sortie directories exist in worktree
	if err := EnsureWorkDirs(t.WorktreePath); err != nil {
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
		loopVars := LoopVars{}
		if step.Loop != nil {
			loopVars.Iteration = t.LoopIteration
			loopVars.MaxIterations = step.Loop.MaxIterations
		} else {
			// Check if any step in the current loop range references this step
			// to populate loop vars for non-loop steps inside a loop
			for _, s := range steps {
				if s.Loop != nil && s.Loop.Goto != "" {
					targetIdx := findStepIndex(steps, s.Loop.Goto)
					if targetIdx >= 0 && targetIdx <= i && i <= findStepIndex(steps, s.Name) {
						loopVars.Iteration = t.LoopIteration
						loopVars.MaxIterations = s.Loop.MaxIterations
						break
					}
				}
			}
		}
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
				BaseBranch:   e.cfg.Git.BaseBranch,
				TargetBranch: e.effectiveBaseBranch(t),
				RepoRoot:     e.repoRoot,
			},
			Loop: loopVars,
		}

		resolvedPrompt := ResolveTemplate(step.Prompt, tmplCtx)

		// Append artifact output instructions if step has artifact: true
		artifactsDir := ArtifactsDir(t.WorktreePath)
		if step.Artifact {
			artifactPath := filepath.Join(artifactsDir, step.Name+".md")
			resolvedPrompt += fmt.Sprintf("\n\n---\n\nIMPORTANT: When you are done, write a summary of what you did to `%s`. Include: files changed, decisions made, and any issues encountered. This artifact is required for subsequent workflow steps.", artifactPath)
		}
		sysPrompt := BuildSystemPrompt(resolvedPrompt, e.cfg.SystemPrompt, imageRelPaths)

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
			exitCode, outputTail, err = e.runClaudeStepTmux(ctx, t, step, resolvedPrompt, env, outputFn, sysPrompt)
		} else {
			exitCode, outputTail, err = e.runClaudeStep(ctx, t, step, resolvedPrompt, env, outputFn, sysPrompt)
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
			return errors.New(errMsg)
		}

		// Validate that the step produced meaningful changes
		// Skip for review steps and tmux steps (agent may still be working)
		if !step.Human && !useTmux {
			noiseFiles := []string{".claude-output.log"}
			hasChanges, err := gitpkg.HasMeaningfulChanges(t.WorktreePath, noiseFiles)
			if err != nil {
				log.Printf("Warning: failed to check for changes in step %q: %v", step.Name, err)
			} else if !hasChanges {
				errMsg := fmt.Sprintf("step %q exited successfully but produced no code changes", step.Name)
				e.database.UpdateTaskExitCode(t.ID, 1, errMsg)
				return errors.New(errMsg)
			}
		}

		// Verify artifact was written for artifact steps
		if step.Artifact && !useTmux {
			artifactPath := filepath.Join(artifactsDir, step.Name+".md")

			// Retry with log context if artifact is missing and retry is enabled
			if !fileExistsAndNonEmpty(artifactPath) && e.cfg.Verification.ArtifactRetry {
				for attempt := 1; attempt <= e.cfg.Verification.MaxRetries; attempt++ {
					log.Printf("Task #%d step %q: artifact missing, retry %d/%d with log context",
						t.ID, step.Name, attempt, e.cfg.Verification.MaxRetries)

					// Read step log for context
					logPath := ProjectLogPath(e.dataDir, t.ID, step.Name)
					logContent := readLogTail(logPath, 500)

					// Build retry prompt
					retryPrompt := buildArtifactRetryPrompt(logContent, artifactPath)

					retrySysPrompt := BuildSystemPrompt(retryPrompt, e.cfg.SystemPrompt, nil)

					retryStep := config.StepConfig{
						Name:    step.Name + "-artifact-retry",
						Timeout: "5m",
					}
					retryEnv := map[string]string{
						"SORTIE_TASK_ID":  fmt.Sprintf("%d", t.ID),
						"SORTIE_STEP":     retryStep.Name,
						"SORTIE_WORKTREE": t.WorktreePath,
					}

					retryExit, _, retryErr := e.runClaudeStep(ctx, t, retryStep, retryPrompt, retryEnv, outputFn, retrySysPrompt)
					if retryErr != nil {
						log.Printf("Warning: artifact retry agent failed: %v", retryErr)
						break
					}
					if retryExit != 0 {
						log.Printf("Warning: artifact retry agent exited with code %d", retryExit)
						break
					}

					if fileExistsAndNonEmpty(artifactPath) {
						log.Printf("Task #%d step %q: artifact recovered on retry %d", t.ID, step.Name, attempt)
						break
					}
				}
			}

			// Final check: if still missing, handle based on config
			if !fileExistsAndNonEmpty(artifactPath) {
				if e.cfg.ValidateArtifact || e.cfg.Verification.ArtifactRetry {
					if err := e.database.UpdateTaskStatus(t.ID, task.StatusArtifactMissing); err != nil {
						log.Printf("Warning: failed to set artifact-missing: %v", err)
					}
					return nil
				}
			}
		}

		// Evaluate loop condition
		if step.Loop != nil {
			shouldLoop := true

			// Check max iterations
			if t.LoopIteration >= step.Loop.MaxIterations {
				shouldLoop = false
			}

			// Check exit condition
			if shouldLoop && step.Loop.ExitCondition != nil {
				if step.Loop.ExitCondition.ArtifactEmpty != "" {
					content, _ := ReadArtifact(t.WorktreePath, step.Loop.ExitCondition.ArtifactEmpty)
					if strings.TrimSpace(content) == "" {
						shouldLoop = false
						log.Printf("Loop exit: artifact %q is empty for task #%d", step.Loop.ExitCondition.ArtifactEmpty, t.ID)
					}
				}
			}

			if shouldLoop {
				targetIdx := findStepIndex(steps, step.Loop.Goto)
				t.LoopIteration++
				if err := e.database.UpdateTaskLoopIteration(t.ID, t.LoopIteration); err != nil {
					log.Printf("Warning: failed to update loop iteration: %v", err)
				}
				if err := e.database.UpdateTaskStep(t.ID, targetIdx, steps[targetIdx].Name); err != nil {
					log.Printf("Warning: failed to update task step for loop: %v", err)
				}
				log.Printf("Task #%d looping back to step %q (iteration %d/%d)", t.ID, step.Loop.Goto, t.LoopIteration, step.Loop.MaxIterations)
				i = targetIdx - 1 // for-loop will increment
				continue
			}

			// Loop done, reset counter
			log.Printf("Task #%d loop completed after %d iterations", t.ID, t.LoopIteration)
			t.LoopIteration = 0
			if err := e.database.UpdateTaskLoopIteration(t.ID, 0); err != nil {
				log.Printf("Warning: failed to reset loop iteration: %v", err)
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

	// Open summarizer log file so TUI can show progress
	summarizerLogPath := ProjectLogPath(e.dataDir, t.ID, "summarizer")
	summarizerLogFile, summarizerLogErr := os.OpenFile(summarizerLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if summarizerLogErr != nil {
		log.Printf("Warning: failed to open summarizer log for task #%d: %v", t.ID, summarizerLogErr)
	}
	summarizerLogFn := func(format string, args ...any) {
		msg := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
		if summarizerLogFile != nil {
			summarizerLogFile.WriteString(msg + "\n")
		}
	}

	if err := e.runSummarizer(ctx, t, wf, summarizerLogFn); err != nil {
		log.Printf("Warning: summarizer failed for task #%d: %v", t.ID, err)
	}

	// Retry summarizer once if verify_summarizer is enabled and context is empty
	if e.cfg.Verification.VerifySummarizer && strings.TrimSpace(t.Context) == "" {
		log.Printf("Task #%d: summarizer produced empty output, retrying", t.ID)
		if err := e.runSummarizer(ctx, t, wf, summarizerLogFn); err != nil {
			log.Printf("Warning: summarizer retry failed for task #%d: %v", t.ID, err)
		}
		if strings.TrimSpace(t.Context) == "" {
			log.Printf("Warning: summarizer still empty after retry for task #%d", t.ID)
		}
	}

	if summarizerLogFile != nil {
		summarizerLogFile.Close()
	}

	// All steps completed — execute on_complete action
	if err := e.executeOnComplete(ctx, t, outputFn, nil); err != nil {
		return fmt.Errorf("on_complete action failed: %w", err)
	}

	return nil
}

// executeOnComplete runs the configured on_complete action after all steps finish.
// logFn is optional; when provided, progress messages are written to it (e.g. finalize log).
func (e *Engine) executeOnComplete(ctx context.Context, t *task.Task, outputFn func([]string), logFn func(string, ...any)) error {
	action := e.cfg.Git.OnComplete

	logf := func(format string, args ...any) {
		if logFn != nil {
			logFn(format, args...)
		}
	}

	switch action {
	case "", "none":
		logf("No on_complete action configured, skipping")
		return nil

	case "commit":
		logf("Committing changes in worktree...")
		if err := gitpkg.Commit(t.WorktreePath, gitpkg.ConventionalCommitFromTitle(t.Title)); err != nil {
			return err
		}
		logf("Commit completed")
		// Track the commit hash
		if commitHash, err := gitpkg.GetLastCommitHash(t.WorktreePath); err == nil {
			if dbErr := e.database.AppendTaskCommit(t.ID, commitHash); dbErr != nil {
				log.Printf("Warning: failed to record commit for task #%d: %v", t.ID, dbErr)
			}
		} else {
			log.Printf("Warning: failed to get commit hash for task #%d: %v", t.ID, err)
		}
		return nil

	case "merge":
		return e.executeMerge(ctx, t, outputFn, logf)

	default:
		log.Printf("Unknown on_complete action: %s", action)
		return nil
	}
}

// executeMerge handles the merge on_complete action: commits changes, squash-merges
// the task branch into the base branch with retry/conflict-resolution, and cleans up.
func (e *Engine) executeMerge(ctx context.Context, t *task.Task, outputFn func([]string), logf func(string, ...any)) error {
	if !t.Worktree {
		// No-worktree mode: treat merge as commit since there's no separate branch
		logf("No-worktree mode: committing changes in project root...")
		if err := gitpkg.Commit(t.WorktreePath, gitpkg.ConventionalCommitFromTitle(t.Title)); err != nil {
			return err
		}
		logf("Commit completed")
		// Track the commit hash
		if commitHash, err := gitpkg.GetLastCommitHash(t.WorktreePath); err == nil {
			if dbErr := e.database.AppendTaskCommit(t.ID, commitHash); dbErr != nil {
				log.Printf("Warning: failed to record commit for task #%d: %v", t.ID, dbErr)
			}
		} else {
			log.Printf("Warning: failed to get commit hash for task #%d: %v", t.ID, err)
		}
		return nil
	}
	if t.Branch == "" {
		return fmt.Errorf("cannot merge: task branch name is empty")
	}
	// Commit any uncommitted changes first (operates on the worktree, safe to do concurrently)
	logf("Committing changes in worktree before merge...")
	if err := gitpkg.Commit(t.WorktreePath, gitpkg.ConventionalCommitFromTitle(t.Title)); err != nil {
		return fmt.Errorf("commit failed: %w", err)
	}
	baseBranch := e.effectiveBaseBranch(t)

	// Wait for the target branch to be clean (no staged or unstaged changes)
	logf("Checking target branch %s for pending changes...", baseBranch)
	if err := e.waitForCleanTarget(ctx, t); err != nil {
		return fmt.Errorf("waiting for clean target branch: %w", err)
	}

	// Pick the best conventional commit message from the branch\'s history
	commitMsg := gitpkg.GetSquashCommitMessage(e.repoRoot, baseBranch, t.Branch, t.Title)
	logf("Squash-merging branch %s into %s...", t.Branch, baseBranch)

	var mergeErr error

	for attempt := 1; attempt <= maxMergeAttempts; attempt++ {
		if attempt > 1 {
			logf("Merge attempt %d/%d...", attempt, maxMergeAttempts)
		}

		// Lock only for the squash-merge on the shared repoRoot
		e.mergeMu.Lock()
		mergeErr = gitpkg.MergeBranch(e.repoRoot, t.Branch, baseBranch, commitMsg)
		e.mergeMu.Unlock()

		if mergeErr == nil {
			logf("Merge successful")
			// Track the merge commit hash
			if commitHash, err := gitpkg.GetLastCommitHash(e.repoRoot); err == nil {
				if dbErr := e.database.AppendTaskCommit(t.ID, commitHash); dbErr != nil {
					log.Printf("Warning: failed to record merge commit for task #%d: %v", t.ID, dbErr)
				}
			} else {
				log.Printf("Warning: failed to get merge commit hash for task #%d: %v", t.ID, err)
			}
			break
		}

		log.Printf("Merge attempt %d/%d failed for task #%d: %v", attempt, maxMergeAttempts, t.ID, mergeErr)
		logf("Merge attempt %d/%d failed: %v", attempt, maxMergeAttempts, mergeErr)

		if attempt == maxMergeAttempts {
			break
		}

		// Update branch outside mutex: merge baseBranch into the worktree branch
		logf("Updating branch from %s...", baseBranch)
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
			logf("Found %d conflicted files, spawning Claude to resolve...", len(conflictFiles))

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
			logf("Conflicts resolved, retrying squash-merge...")
		}
		// If MergeInto succeeded cleanly (no error), the branch is updated — retry the squash-merge
	}

	if mergeErr != nil {
		return fmt.Errorf("merge failed after %d attempts: %w", maxMergeAttempts, mergeErr)
	}

	// Clean up worktree and branch (safe to run concurrently)
	logf("Cleaning up worktree and branch...")
	if err := gitpkg.RemoveWorktree(e.repoRoot, t.WorktreePath); err != nil {
		log.Printf("Warning: failed to remove worktree: %v", err)
	}
	// Prune stale worktree references so git doesn\'t think the branch
	// is still checked out in a (now-removed) worktree.
	if err := gitpkg.CleanupWorktrees(e.repoRoot); err != nil {
		log.Printf("Warning: failed to prune worktrees: %v", err)
	}
	if err := gitpkg.ForceDeleteBranch(e.repoRoot, t.Branch); err != nil {
		log.Printf("Warning: failed to delete branch: %v", err)
	}
	// Clear worktree path in DB
	if err := e.database.ClearWorktreePath(t.ID); err != nil {
		log.Printf("Warning: failed to clear worktree path: %v", err)
	}
	logf("Cleanup completed")
	return nil
}

// waitForCleanTarget polls the repo root until it has no pending changes (staged or unstaged).
// If changes are detected, it sets the task to merge-blocked and retries every 10 seconds.
func (e *Engine) waitForCleanTarget(ctx context.Context, t *task.Task) error {
	dirty, err := gitpkg.HasChanges(e.repoRoot)
	if err != nil {
		return fmt.Errorf("failed to check target branch: %w", err)
	}
	if !dirty {
		return nil
	}

	log.Printf("Task #%d: target branch has pending changes, entering merge-blocked state", t.ID)
	if err := e.database.UpdateTaskStatus(t.ID, task.StatusMergeBlocked); err != nil {
		log.Printf("Warning: failed to set merge-blocked status for task #%d: %v", t.ID, err)
	}

	ticker := time.NewTicker(mergeBlockedPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			dirty, err := gitpkg.HasChanges(e.repoRoot)
			if err != nil {
				log.Printf("Task #%d: failed to check target branch: %v", t.ID, err)
				continue
			}
			if !dirty {
				log.Printf("Task #%d: target branch is clean, proceeding with merge", t.ID)
				return nil
			}
			log.Printf("Task #%d: target branch still has pending changes, retrying in 10s", t.ID)
		}
	}
}

// FinalizeTask runs the summarizer and on_complete action for a task.
// Used when finalizing a tmux-continued task.
func (e *Engine) FinalizeTask(ctx context.Context, t *task.Task) error {
	// Open finalize log file so TUI can show progress
	logDir := ProjectLogsDir(e.dataDir, t.ID)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("Warning: failed to create log dir for task #%d: %v", t.ID, err)
	}
	logPath := ProjectLogPath(e.dataDir, t.ID, "finalize")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("Warning: failed to open finalize log for task #%d: %v", t.ID, err)
	}
	defer func() {
		if logFile != nil {
			logFile.Close()
		}
	}()

	logFn := func(format string, args ...any) {
		msg := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
		log.Printf("Task #%d finalize: %s", t.ID, fmt.Sprintf(format, args...))
		if logFile != nil {
			logFile.WriteString(msg + "\n")
		}
	}

	logFn("=== Finalization started for task #%d: %s ===", t.ID, t.Title)

	// Resolve branch name if not set (may not have been persisted to DB)
	if t.Worktree && t.Branch == "" {
		t.Branch = e.cfg.ResolveBranchForTask(t.ID, t.Title, t.Slug, t.BranchName)
	}

	// Run summarizer to generate task context (same as normal completion)
	wf := e.cfg.GetWorkflow(t.Workflow)
	if wf != nil {
		if err := e.database.UpdateTaskStatus(t.ID, task.StatusSummarizing); err != nil {
			logFn("Warning: failed to set summarizing status: %v", err)
		}
		logFn("Running summarizer...")
		if err := e.runSummarizer(ctx, t, wf, logFn); err != nil {
			logFn("Warning: summarizer failed: %v", err)
		} else {
			logFn("Summarizer completed")
		}
	}

	action := e.cfg.Git.OnComplete
	if action == "" {
		action = "none"
	}
	logFn("Running on_complete action: %s", action)

	if err := e.executeOnComplete(ctx, t, nil, logFn); err != nil {
		logFn("Error: on_complete failed: %v", err)
		return err
	}

	logFn("=== Finalization completed ===")
	return nil
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

	conflictSysPrompt := BuildSystemPrompt(prompt, e.cfg.SystemPrompt, nil)

	step := config.StepConfig{
		Name:    "resolve-conflicts",
		Timeout: "10m",
	}

	env := map[string]string{
		"SORTIE_TASK_ID":  fmt.Sprintf("%d", t.ID),
		"SORTIE_STEP":     step.Name,
		"SORTIE_WORKTREE": t.WorktreePath,
	}

	exitCode, outputTail, err := e.runClaudeStep(ctx, t, step, prompt, env, outputFn, conflictSysPrompt)
	if err != nil {
		return fmt.Errorf("conflict resolution claude process failed: %w", err)
	}
	if exitCode != 0 {
		errMsg := fmt.Sprintf("conflict resolution exited with code %d", exitCode)
		if outputTail != "" {
			errMsg += "\n" + outputTail
		}
		return errors.New(errMsg)
	}

	return nil
}

// AcquireMergeLock acquires the merge mutex for operations that modify the base branch.
func (e *Engine) AcquireMergeLock() {
	e.mergeMu.Lock()
}

// ReleaseMergeLock releases the merge mutex.
func (e *Engine) ReleaseMergeLock() {
	e.mergeMu.Unlock()
}

// ResumeAfterApproval resumes a task from its current step index.
func (e *Engine) ResumeAfterApproval(ctx context.Context, t *task.Task, outputFn func([]string)) error {
	return e.RunTask(ctx, t, outputFn)
}

func (e *Engine) runClaudeStep(ctx context.Context, t *task.Task, step config.StepConfig, prompt string, envVars map[string]string, outputFn func([]string), systemPrompt ...string) (int, string, error) {
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
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return 1, "", fmt.Errorf("failed to open step log: %w", err)
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
		return 1, "", fmt.Errorf("failed to start claude: %w", err)
	}

	// Wait for process to exit
	ticker := time.NewTicker(processExitPollInterval)
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

// readLogTail reads the last n lines from a log file.
// Returns empty string if the file doesn't exist or can't be read.
func readLogTail(path string, maxLines int) string {
	lines, err := readLastLines(path, maxLines)
	if err != nil || len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

// buildArtifactRetryPrompt constructs the prompt for an artifact recovery agent.
func buildArtifactRetryPrompt(logContent, artifactPath string) string {
	var sb strings.Builder
	sb.WriteString("The previous agent session completed a workflow step but did not write the required artifact file.\n\n")
	if logContent != "" {
		sb.WriteString("Below is the session log from that step:\n\n")
		sb.WriteString("```\n")
		sb.WriteString(logContent)
		sb.WriteString("\n```\n\n")
	}
	sb.WriteString(fmt.Sprintf("Based on the session log above, write a summary to `%s`.\n\n", artifactPath))
	sb.WriteString("Include:\n")
	sb.WriteString("- What was done in the step\n")
	sb.WriteString("- Reasoning and decisions made\n")
	sb.WriteString("- Files changed\n")
	sb.WriteString("- Any issues encountered\n\n")
	sb.WriteString("Do NOT make any code changes. Only write the artifact file.")
	return sb.String()
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
		claudeCmd += " --dangerously-skip-permissions"
	}
	if len(systemPrompt) > 0 && systemPrompt[0] != "" {
		// Write system prompt to file to avoid shell quoting issues
		sysPromptFile := filepath.Join(sortieDir, fmt.Sprintf("step-sysprompt-%s.txt", step.Name))
		if err := os.WriteFile(sysPromptFile, []byte(systemPrompt[0]), 0644); err != nil {
			return 1, "", fmt.Errorf("failed to write system prompt file: %w", err)
		}
		claudeCmd += fmt.Sprintf(" --system-prompt \"$(cat %q)\"", sysPromptFile)
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

	log.Printf("Tmux session %q started for task #%d step %q (attach with: sortie attach %s)",
		session.Name, t.ID, step.Name, taskID)

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
		fmt.Sprintf("[%s] Attach with: sortie attach %s", ts, taskIDStr),
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
// logFn is optional; when provided, progress messages are written to it (e.g. finalize log).
func (e *Engine) runSummarizer(ctx context.Context, t *task.Task, wf *config.WorkflowConfig, logFn func(string, ...any)) error {
	logMsg := func(format string, args ...any) {
		log.Printf(format, args...)
		if logFn != nil {
			logFn(format, args...)
		}
	}
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
			logMsg("Warning: failed to get diff stat for task #%d: %v", t.ID, err)
		}
		if diffStat == "" {
			logMsg("No artifacts or changes found for task #%d, skipping summarizer", t.ID)
			return nil
		}
	}

	// Build the prompt and log the summarization approach
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
		var names []string
		for name := range artifacts {
			names = append(names, name)
		}
		logMsg("%s", summarizationDescription(t.ID, true, names, false))
	} else if len(artifacts) > 0 {
		// Build default prompt with all artifacts
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Summarize the progress made on task #%d: %s\n\n", t.ID, t.Title))
		sb.WriteString("Use the context from the following task artifacts:\n\n")
		var artifactNames []string
		for _, name := range stepNames {
			if content, ok := artifacts[name]; ok {
				sb.WriteString(fmt.Sprintf("### %s\n\n%s\n\n", name, content))
				artifactNames = append(artifactNames, name)
			}
		}
		sb.WriteString("Provide a concise but comprehensive summary of what was accomplished, ")
		sb.WriteString("any decisions made, and the current state of the implementation. ")
		sb.WriteString("This summary will be used as context for future work on this task.")
		prompt = sb.String()
		logMsg("%s", summarizationDescription(t.ID, false, artifactNames, false))
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
		logMsg("%s", summarizationDescription(t.ID, false, nil, true))
	}

	logMsg("Running summarizer for task #%d", t.ID)

	// Run Claude synchronously to capture the summary text
	summary, err := e.runClaudeSync(ctx, prompt, t.WorktreePath)
	if err != nil {
		return fmt.Errorf("summarizer claude invocation failed: %w", err)
	}

	summary = strings.TrimSpace(summary)
	if summary == "" {
		logMsg("Summarizer produced empty output for task #%d", t.ID)
		return nil
	}

	// Store the context in the database
	if err := e.database.UpdateTaskContext(t.ID, summary); err != nil {
		return fmt.Errorf("failed to store task context: %w", err)
	}

	t.Context = summary
	logMsg("Summarizer completed for task #%d (%d chars)", t.ID, len(summary))
	return nil
}

// summarizationDescription returns a human-readable description of the summarization
// approach being used for a task, suitable for logging.
func summarizationDescription(taskID int64, hasCustomPrompt bool, artifactNames []string, useDiffStat bool) string {
	if hasCustomPrompt {
		if len(artifactNames) > 0 {
			return fmt.Sprintf("Summarizing task #%d with custom prompt and artifacts: %s", taskID, strings.Join(artifactNames, ", "))
		}
		return fmt.Sprintf("Summarizing task #%d with custom prompt", taskID)
	}
	if len(artifactNames) > 0 {
		return fmt.Sprintf("Summarizing task #%d with artifacts: %s", taskID, strings.Join(artifactNames, ", "))
	}
	if useDiffStat {
		return fmt.Sprintf("Summarizing task #%d via git diff", taskID)
	}
	return fmt.Sprintf("Summarizing task #%d", taskID)
}

// RunWorktreeSetupCommand runs the configured worktree setup command, if any.
// The command is executed with the project root as the working directory.
// {{worktree_path}} in the command string is replaced with the actual worktree path.
func RunWorktreeSetupCommand(ctx context.Context, projectRoot, worktreePath, command string) error {
	if command == "" {
		return nil
	}

	// Replace template variable
	resolved := strings.ReplaceAll(command, "{{worktree_path}}", worktreePath)

	log.Printf("Running worktree setup command: %s", resolved)

	cmd := exec.CommandContext(ctx, "sh", "-c", resolved)
	cmd.Dir = projectRoot

	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		log.Printf("Worktree setup output:\n%s", string(output))
	}
	if err != nil {
		return fmt.Errorf("worktree setup command failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// runClaudeSync runs Claude Code synchronously and captures its stdout output.
// workDir sets the working directory for the Claude process so it can access
// the task's worktree files.
func (e *Engine) runClaudeSync(ctx context.Context, prompt string, workDir string) (string, error) {
	args := []string{"-p", prompt, "--output-format", "text", "--model", "haiku"}
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
