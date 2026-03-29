# internal/git — Git Operations

Worktree management, merge/rebase, conflict resolution, branch operations. Load `/git-operations` skill before making substantial changes.

## Critical Invariants

- **`MergeBranch()` includes deferred `CleanRepoState()` safety net** — aborts merge and hard-resets on failure to prevent corrupted state
- **`RevertCommits()` reverses in newest-first order** — reverse iteration ensures correct commit lineage
- **Non-worktree mode: no branch creation/deletion** — `on_complete` uses `Commit()`, not merge
