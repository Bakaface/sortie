package workflow

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aface/sortie/internal/config"
	gitpkg "github.com/aface/sortie/internal/git"
	"github.com/aface/sortie/internal/task"
)

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

	// Pick the best conventional commit message from the branch's history
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

	exitCode, _, outputTail, err := e.runClaudeStep(ctx, t, step, prompt, env, outputFn, conflictSysPrompt)
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

// cleanupMergedWorktree removes the worktree and branch after a successful merge.
func (e *Engine) cleanupMergedWorktree(t *task.Task, logf func(string, ...any)) {
	logf("Cleaning up worktree and branch...")
	if err := gitpkg.RemoveWorktree(e.repoRoot, t.WorktreePath); err != nil {
		log.Printf("Warning: failed to remove worktree: %v", err)
	}
	// Prune stale worktree references so git doesn't think the branch
	// is still checked out in a (now-removed) worktree.
	if err := gitpkg.CleanupWorktrees(e.repoRoot); err != nil {
		log.Printf("Warning: failed to prune worktrees: %v", err)
	}
	// Only delete branches that sortie created; preserve user-provided branches
	if t.CheckoutBranch == "" {
		if err := gitpkg.ForceDeleteBranch(e.repoRoot, t.Branch); err != nil {
			log.Printf("Warning: failed to delete branch: %v", err)
		}
	}
	// Clear worktree path in DB
	if err := e.database.ClearWorktreePath(t.ID); err != nil {
		log.Printf("Warning: failed to clear worktree path: %v", err)
	}
	logf("Cleanup completed")
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

// AcquireMergeLock acquires the merge mutex for operations that modify the base branch.
func (e *Engine) AcquireMergeLock() {
	e.mergeMu.Lock()
}

// ReleaseMergeLock releases the merge mutex.
func (e *Engine) ReleaseMergeLock() {
	e.mergeMu.Unlock()
}
