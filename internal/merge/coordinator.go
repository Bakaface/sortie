// Package merge is the deep module that owns the per-repo merge invariant.
//
// "All merges into the same repo must serialize, and on any failure the repo's
// working tree state must be cleaned." Before this module existed that
// invariant was spread across three packages — the daemon held the per-repo
// mutex map, the workflow engine drove the retry loop, and internal/git had a
// deferred CleanRepoState on the merge primitive. Each piece was shallow
// because it relied on the others to behave correctly.
//
// This package consolidates them. The daemon hands out per-repo Locks via a
// Locks registry that survives config reloads; an Engine wraps a Lock plus a
// few engine-local callbacks (conflict resolution, DB writes) into a
// Coordinator; callers invoke Coordinator.Finalize and get the whole pipeline.
//
// The external interface is intentionally tiny:
//
//	coord.Finalize(ctx, task, baseBranch, log)
//
// Everything else — locking, conflict retry, target-branch wait,
// cleanup-on-failure — is locality inside this package.
package merge

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	gitpkg "github.com/Bakaface/sortie/internal/git"
	"github.com/Bakaface/sortie/internal/task"
)

// Action is the on_complete behaviour the coordinator should run.
//
// "" and "none" are equivalent and produce a no-op. "commit" commits any
// pending changes in the task worktree. "merge" runs the full merge pipeline.
type Action = string

const (
	ActionNone   Action = "none"
	ActionCommit Action = "commit"
	ActionMerge  Action = "merge"
)

// Config holds the per-coordinator knobs. These are stable across tasks; the
// per-task action and branch are passed to Finalize.
type Config struct {
	// OnComplete is the configured on_complete action ("merge" | "commit" | "none" | "").
	OnComplete Action

	// MaxAttempts caps the merge-retry loop. Defaults to 3 when zero.
	MaxAttempts int

	// BlockedPollInterval is how often to re-check whether the target branch is
	// clean while waiting in merge-blocked state. Defaults to 10s when zero.
	BlockedPollInterval time.Duration
}

// LogFunc is the per-call log sink used by Finalize. nil is treated as discard.
type LogFunc func(format string, args ...any)

// ConflictResolver is invoked when an in-worktree update against the base
// branch produces conflicts. Implementations typically spawn a Claude agent
// against the worktree to fix the conflict markers and stage the result.
//
// Returning nil means the conflicts are resolved and the merge commit can
// proceed. Returning an error aborts the retry loop.
type ConflictResolver func(ctx context.Context, t *task.Task, conflictFiles []string) error

// StatusSetter records a status change for a task. It is invoked outside the
// per-repo lock so it can safely take its own database mutexes.
type StatusSetter func(taskID int64, status task.Status) error

// CommitRecorder appends a commit hash to a task's history.
type CommitRecorder func(taskID int64, commitHash string) error

// Coordinator owns the merge pipeline for one repository: it serializes via a
// shared Lock, waits for the target branch to be clean, runs the merge with
// conflict-retry, and guarantees CleanRepoState on any pipeline failure.
//
// One Coordinator instance exists per Engine; the Lock is shared across all
// Engines pointing at the same repoRoot (so config reloads do not break
// serialization).
type Coordinator struct {
	repoRoot string
	lock     *Lock
	cfg      Config

	resolveConflicts ConflictResolver
	setStatus        StatusSetter
	recordCommit     CommitRecorder
}

// NewCoordinator wires a Lock plus its engine-local callbacks into a
// Coordinator. Any callback may be nil — a nil ConflictResolver means
// conflicts abort the merge instead of triggering resolution; nil
// StatusSetter / CommitRecorder mean those side effects are skipped.
func NewCoordinator(
	repoRoot string,
	lock *Lock,
	cfg Config,
	resolveConflicts ConflictResolver,
	setStatus StatusSetter,
	recordCommit CommitRecorder,
) *Coordinator {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.BlockedPollInterval <= 0 {
		cfg.BlockedPollInterval = 10 * time.Second
	}
	if lock == nil {
		lock = &Lock{}
	}
	return &Coordinator{
		repoRoot:         repoRoot,
		lock:             lock,
		cfg:              cfg,
		resolveConflicts: resolveConflicts,
		setStatus:        setStatus,
		recordCommit:     recordCommit,
	}
}

// Lock exposes the per-repo lock for ad-hoc operations that must serialize
// against merges (currently: revert and worktree teardown). Prefer Finalize.
func (c *Coordinator) Lock() *Lock { return c.lock }

// RepoRoot returns the absolute path this Coordinator merges into.
func (c *Coordinator) RepoRoot() string { return c.repoRoot }

// Finalize runs the configured on_complete action for the task and guarantees
// that on any failure during a merge the repository's working tree is restored
// to a clean state.
//
// Locality of the merge invariant:
//   - Serialization: the per-repo Lock is held only across the base-repo merge
//     operation itself. Worktree-side work (commits, conflict resolution) runs
//     outside the lock so two tasks can prep their worktrees in parallel.
//   - Cleanup: CleanRepoState fires whenever the merge branch of this function
//     returns an error — regardless of whether the failure came from
//     MergeBranch, MergeInto, CompleteMerge, or the conflict resolver.
//
// For Action == "" / "none", or for non-worktree tasks, Finalize is a no-op.
// For Action == "merge" on a non-worktree task, Finalize falls back to a
// commit (because there is no task branch to merge from).
func (c *Coordinator) Finalize(ctx context.Context, t *task.Task, baseBranch string, logFn LogFunc) error {
	logf := normalizeLog(logFn)

	action := c.cfg.OnComplete
	switch action {
	case "", ActionNone:
		logf("No on_complete action configured, skipping")
		return nil
	}

	if !t.Worktree {
		// Non-worktree mode shares the user's working tree, so auto-committing
		// would scoop up unrelated changes and merge has no separate branch.
		// Leave the working tree alone.
		logf("No-worktree mode: leaving working tree as-is, skipping on_complete=%q", action)
		return nil
	}

	switch action {
	case ActionCommit:
		return c.commit(t, logf)
	case ActionMerge:
		return c.merge(ctx, t, baseBranch, logf)
	default:
		logf("Unknown on_complete action: %s", action)
		return nil
	}
}

// commit handles on_complete=commit by committing pending changes in the
// worktree. The base repo is not touched, so no locking or cleanup is needed.
func (c *Coordinator) commit(t *task.Task, logf LogFunc) error {
	logf("Committing changes in worktree...")
	if err := gitpkg.Commit(t.WorktreePath, gitpkg.ConventionalCommitFromTitle(t.Title)); err != nil {
		return err
	}
	logf("Commit completed")
	c.recordWorktreeCommit(t)
	return nil
}

// merge runs the merge pipeline with cleanup-on-failure as a top-level
// guarantee. The deferred CleanRepoState fires for any non-nil return inside
// this method — including failures that happen *after* MergeBranch has
// committed and *before* the post-merge bookkeeping completes.
func (c *Coordinator) merge(ctx context.Context, t *task.Task, baseBranch string, logf LogFunc) (retErr error) {
	if t.Branch == "" {
		return errors.New("cannot merge: task branch name is empty")
	}

	// Top-level safety net: if any branch of the merge pipeline returns an
	// error, abort any in-progress merge and hard-reset the base repo so a
	// later task does not see staged conflict markers on main. This is the
	// single owner of the cleanup invariant — internal/git's MergeBranch keeps
	// its own deferred CleanRepoState as defence-in-depth, but the
	// authoritative cleanup lives here so failures *outside* MergeBranch
	// (conflict resolution, target-clean wait, etc.) are covered too.
	defer func() {
		if retErr == nil {
			return
		}
		if err := gitpkg.CleanRepoState(c.repoRoot); err != nil {
			log.Printf("merge.Coordinator: failed to clean repo state for %s after merge failure: %v", c.repoRoot, err)
			logf("Warning: cleanup after merge failure failed: %v", err)
		}
	}()

	// Commit any uncommitted worktree changes. Safe to do outside the per-repo
	// lock — this writes only to the worktree's index.
	logf("Committing changes in worktree before merge...")
	if err := gitpkg.Commit(t.WorktreePath, gitpkg.ConventionalCommitFromTitle(t.Title)); err != nil {
		return fmt.Errorf("commit failed: %w", err)
	}

	// Wait for the target branch to be clean. Done outside the lock so other
	// tasks can finish their own merges (which is what clears the dirty state).
	logf("Checking target branch %s for pending changes...", baseBranch)
	if err := c.waitForCleanTarget(ctx, t); err != nil {
		return fmt.Errorf("waiting for clean target branch: %w", err)
	}

	commitMsg := gitpkg.ConventionalCommitFromTitle(t.Title)
	logf("Merging branch %s into %s...", t.Branch, baseBranch)

	var mergeErr error
	for attempt := 1; attempt <= c.cfg.MaxAttempts; attempt++ {
		if attempt > 1 {
			logf("Merge attempt %d/%d...", attempt, c.cfg.MaxAttempts)
		}

		mergeErr = c.lockedMerge(t.Branch, baseBranch, commitMsg)
		if mergeErr == nil {
			logf("Merge successful")
			c.recordBaseCommit(t)
			return nil
		}

		log.Printf("Merge attempt %d/%d failed for task #%d: %v", attempt, c.cfg.MaxAttempts, t.ID, mergeErr)
		logf("Merge attempt %d/%d failed: %v", attempt, c.cfg.MaxAttempts, mergeErr)

		if attempt == c.cfg.MaxAttempts {
			break
		}

		// Conflict path: update the worktree branch from base outside the lock,
		// resolving any conflicts via the injected resolver, then retry the
		// base-repo merge.
		if err := c.refreshFromBase(ctx, t, baseBranch, logf); err != nil {
			return err
		}
	}

	return fmt.Errorf("merge failed after %d attempts: %w", c.cfg.MaxAttempts, mergeErr)
}

// lockedMerge is the only place the per-repo Lock is held. Keep it tiny —
// every line that runs under the lock contributes to head-of-line blocking
// across all tasks in this repo.
func (c *Coordinator) lockedMerge(branch, baseBranch, commitMsg string) error {
	c.lock.Acquire()
	defer c.lock.Release()
	return gitpkg.MergeBranch(c.repoRoot, branch, baseBranch, commitMsg)
}

// refreshFromBase merges baseBranch into the task's worktree branch, resolving
// any conflicts via the injected ConflictResolver. Returns nil on success.
//
// Runs outside the per-repo lock — the worktree has its own index and other
// tasks' merges into base do not need to block on this.
func (c *Coordinator) refreshFromBase(ctx context.Context, t *task.Task, baseBranch string, logf LogFunc) error {
	logf("Updating branch from %s...", baseBranch)
	if err := gitpkg.MergeInto(t.WorktreePath, baseBranch); err == nil {
		// Clean merge of base into worktree — no conflicts, ready to retry the
		// base-repo merge as-is.
		return nil
	}

	// Conflict resolution path.
	conflictFiles, cfErr := gitpkg.GetConflictedFiles(t.WorktreePath)
	if cfErr != nil {
		gitpkg.AbortMerge(t.WorktreePath)
		return fmt.Errorf("could not list conflicts: %w", cfErr)
	}

	if len(conflictFiles) == 0 {
		gitpkg.AbortMerge(t.WorktreePath)
		return errors.New("merge into worktree failed with no conflicted files")
	}

	if c.resolveConflicts == nil {
		gitpkg.AbortMerge(t.WorktreePath)
		return fmt.Errorf("merge produced %d conflicted files and no resolver is configured", len(conflictFiles))
	}

	log.Printf("Task #%d has %d conflicted files, invoking resolver", t.ID, len(conflictFiles))
	logf("Found %d conflicted files, invoking resolver...", len(conflictFiles))

	if err := c.resolveConflicts(ctx, t, conflictFiles); err != nil {
		gitpkg.AbortMerge(t.WorktreePath)
		return fmt.Errorf("merge conflict resolution failed: %w", err)
	}

	remaining, err := gitpkg.GetConflictedFiles(t.WorktreePath)
	if err != nil {
		gitpkg.AbortMerge(t.WorktreePath)
		return fmt.Errorf("failed to verify conflict resolution: %w", err)
	}
	if len(remaining) > 0 {
		gitpkg.AbortMerge(t.WorktreePath)
		return fmt.Errorf("resolver left %d files still conflicted: %v", len(remaining), remaining)
	}

	if err := gitpkg.CompleteMerge(t.WorktreePath); err != nil {
		gitpkg.AbortMerge(t.WorktreePath)
		return fmt.Errorf("failed to complete merge after conflict resolution: %w", err)
	}

	log.Printf("Task #%d conflicts resolved, retrying merge", t.ID)
	logf("Conflicts resolved, retrying merge...")
	return nil
}

// waitForCleanTarget polls the repo root until it has no pending changes.
// While waiting it transitions the task into the merge-blocked state via the
// injected StatusSetter so the UI can show why work is stalled. Runs outside
// any lock — the goal is to let other tasks finish their merges.
func (c *Coordinator) waitForCleanTarget(ctx context.Context, t *task.Task) error {
	dirty, err := gitpkg.HasChanges(c.repoRoot)
	if err != nil {
		return fmt.Errorf("failed to check target branch: %w", err)
	}
	if !dirty {
		return nil
	}

	log.Printf("Task #%d: target branch has pending changes, entering merge-blocked state", t.ID)
	if c.setStatus != nil {
		if err := c.setStatus(t.ID, task.StatusMergeBlocked); err != nil {
			log.Printf("Warning: failed to set merge-blocked status for task #%d: %v", t.ID, err)
		}
	}

	ticker := time.NewTicker(c.cfg.BlockedPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			dirty, err := gitpkg.HasChanges(c.repoRoot)
			if err != nil {
				log.Printf("Task #%d: failed to check target branch: %v", t.ID, err)
				continue
			}
			if !dirty {
				log.Printf("Task #%d: target branch is clean, proceeding with merge", t.ID)
				return nil
			}
			log.Printf("Task #%d: target branch still has pending changes, retrying", t.ID)
		}
	}
}

// recordBaseCommit appends the new HEAD of the base repo (the fast-forward
// target, or the merge commit when fast-forward wasn't possible) to the
// task's commit history.
func (c *Coordinator) recordBaseCommit(t *task.Task) {
	if c.recordCommit == nil {
		return
	}
	hash, err := gitpkg.GetLastCommitHash(c.repoRoot)
	if err != nil {
		log.Printf("Warning: failed to get base HEAD hash for task #%d: %v", t.ID, err)
		return
	}
	if err := c.recordCommit(t.ID, hash); err != nil {
		log.Printf("Warning: failed to record base commit for task #%d: %v", t.ID, err)
	}
}

// recordWorktreeCommit appends the just-created worktree commit (HEAD of the
// worktree) to the task's commit history.
func (c *Coordinator) recordWorktreeCommit(t *task.Task) {
	if c.recordCommit == nil {
		return
	}
	hash, err := gitpkg.GetLastCommitHash(t.WorktreePath)
	if err != nil {
		log.Printf("Warning: failed to get commit hash for task #%d: %v", t.ID, err)
		return
	}
	if err := c.recordCommit(t.ID, hash); err != nil {
		log.Printf("Warning: failed to record commit for task #%d: %v", t.ID, err)
	}
}

func normalizeLog(fn LogFunc) LogFunc {
	if fn != nil {
		return fn
	}
	return func(string, ...any) {}
}

// Lock is the per-repo merge serializer. It wraps sync.Mutex so the surface is
// explicit ("Acquire merge" / "Release merge") rather than the more generic
// Lock/Unlock.
type Lock struct {
	mu sync.Mutex
}

// Acquire blocks until the caller can be the exclusive merger for this repo.
func (l *Lock) Acquire() { l.mu.Lock() }

// Release returns the merge lock so the next waiter can proceed.
func (l *Lock) Release() { l.mu.Unlock() }

// WithLock runs fn while holding the merge lock. Prefer this over
// Acquire/Release in callers that just want to do a small protected block, so
// the lock cannot leak on a panic.
func (l *Lock) WithLock(fn func()) {
	l.Acquire()
	defer l.Release()
	fn()
}

// Locks is the daemon-owned registry that hands out one Lock per repo root.
// Locks survive engine reconstruction (config reloads), so concurrent tasks
// always agree on a single serializer.
type Locks struct {
	mu    sync.Mutex
	locks map[string]*Lock
}

// NewLocks returns an empty registry.
func NewLocks() *Locks {
	return &Locks{locks: make(map[string]*Lock)}
}

// For returns the Lock for the given repo root, creating one on first use.
// Two calls with the same repoRoot always return the same *Lock.
func (l *Locks) For(repoRoot string) *Lock {
	l.mu.Lock()
	defer l.mu.Unlock()
	lk, ok := l.locks[repoRoot]
	if !ok {
		lk = &Lock{}
		l.locks[repoRoot] = lk
	}
	return lk
}
