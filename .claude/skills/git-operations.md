# Git Operations

TRIGGER when: editing files in `internal/git/`, working on worktree management, merge strategies, branch operations, conflict resolution, or commit message generation.

## Overview

Two files:
- `operations.go` - Core git commands: commit, merge, rebase, diff, branch management, conflict handling
- `worktree.go` - Worktree creation, removal, listing, cleanup

## Worktree Management

```go
CreateWorktree(repoRoot, worktreePath, branchName, baseBranch) error
RemoveWorktree(repoRoot, worktreePath) error
ListWorktrees(repoRoot) ([]WorktreeInfo, error)   // Filters for "sortie-*" prefix
CleanupWorktrees(repoRoot) error                    // git worktree prune
```

- `CreateWorktree`: runs `git worktree add -b branchName worktreePath baseBranch`; handles "already exists" by falling back to plain checkout
- `RemoveWorktree`: `git worktree remove --force` + `os.RemoveAll()` cleanup

## Core Operations

| Function | What it does |
|----------|-------------|
| `Commit(dir, msg)` | `git add -A` -> check status -> `git commit -m` (no-op if clean) |
| `GetCurrentBranch(dir)` | `rev-parse --abbrev-ref HEAD` |
| `HasChanges(dir)` | `git status --porcelain` |
| `HasMeaningfulChanges(dir, excludeFiles)` | Checks uncommitted + committed changes excluding noise files |
| `MergeBranch(repoRoot, baseBranch, branch, msg)` | Checkout base -> `merge --squash` -> commit (reset on failure) |
| `RebaseBranch(dir, baseBranch)` | `git rebase baseBranch` with auto-abort on failure |
| `DiffStat(dir, baseBranch)` | merge-base -> `git diff --stat` |

## Conflict Resolution

```go
MergeInto(dir, baseBranch)          // git merge baseBranch --no-edit (leaves conflicts in progress)
GetConflictedFiles(dir)             // git diff --name-only --diff-filter=U
CompleteMerge(dir)                  // git add -A -> git commit --no-edit
AbortMerge(dir)                     // git merge --abort
```

Used by workflow engine's `resolveConflicts()` which spawns Claude to fix conflict markers.

## Commit Message Utilities

- `GetLastCommitMessage(dir)` - `git log -1 --pretty=%B`
- `ConventionalCommitFromTitle(title)` - Converts freeform text to conventional commit (feat/fix/docs/etc.)
- `GetSquashCommitMessage(dir, baseBranch)` - Searches branch commits newest-first for conventional format; falls back to first commit subject

## Patterns to Follow

- All git commands run via `exec.Command("git", ...)` with `Dir` set to the working directory
- Error messages include both the git command and stderr output for debuggability
- `HasMeaningfulChanges()` excludes `.claude-output.log` and `CLAUDE.md` from change detection
- `getDefaultBranch()` tries `symbolic-ref refs/remotes/origin/HEAD`, falls back to main/master/HEAD
- Worktree paths live under the Sortie data directory, not inside the repo tree
