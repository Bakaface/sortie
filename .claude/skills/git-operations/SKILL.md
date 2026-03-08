---
name: git-operations
description: >
  Sortie's git operations: worktree management, merge/rebase strategies, conflict
  resolution, branch operations, and commit message generation. Use when editing
  files in internal/git/, working on worktree creation/removal, merge logic,
  conflict handling, or conventional commit utilities.
---

# Git Operations

Two files: `operations.go` (core git commands) and `worktree.go` (worktree lifecycle).

## Worktree Management

```go
CreateWorktree(repoRoot, worktreePath, branchName, baseBranch)  // git worktree add -b
RemoveWorktree(repoRoot, worktreePath)                          // force remove + os.RemoveAll
ListWorktrees(repoRoot)                                         // filters "sortie-*" prefix
CleanupWorktrees(repoRoot)                                      // git worktree prune
```

`CreateWorktree` handles "already exists" by falling back to plain checkout.

## Core Operations

| Function | What it does |
|----------|-------------|
| `Commit(dir, msg)` | `add -A` -> status check -> `commit -m` (no-op if clean) |
| `HasMeaningfulChanges(dir, excludeFiles)` | Uncommitted + committed changes excluding noise |
| `MergeBranch(repoRoot, base, branch, msg)` | Checkout base -> `merge --squash` -> commit |
| `RebaseBranch(dir, baseBranch)` | Rebase with auto-abort on failure |
| `DiffStat(dir, baseBranch)` | merge-base -> `diff --stat` |

## Conflict Resolution

```go
MergeInto(dir, baseBranch)       // Leaves conflicts in progress
GetConflictedFiles(dir)          // git diff --diff-filter=U
CompleteMerge(dir)               // add -A -> commit --no-edit
AbortMerge(dir)                  // git merge --abort
```

Used by workflow engine's `resolveConflicts()` which spawns Claude to fix conflict markers.

## Commit Message Utilities

- `ConventionalCommitFromTitle(title)` — freeform text -> conventional commit format
- `GetSquashCommitMessage(dir, baseBranch)` — search branch commits for conventional format, fallback to first subject

## Patterns

- All git commands via `exec.Command("git", ...)` with `Dir` set
- `HasMeaningfulChanges()` excludes `.claude-output.log` and `CLAUDE.md`
- `getDefaultBranch()`: tries `symbolic-ref`, falls back to main/master/HEAD
