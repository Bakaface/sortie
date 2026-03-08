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
const WorktreePrefix = "sortie-task-"

type Worktree struct {
    Path        string
    Branch      string
    RepoRoot    string
    WorktreeDir string
}

CreateWorktree(repoRoot string, taskID int64, baseBranch, branchName string) (*Worktree, error)
RemoveWorktree(repoRoot, worktreePath string) error
ListWorktrees(repoRoot string) ([]string, error)   // Filters "sortie-*" prefix paths
CleanupWorktrees(repoRoot string) error             // git worktree prune
IsGitRepo(path string) bool
GetRepoRoot(path string) (string, error)
```

Worktree path convention: `<repoRoot>/.sortie/worktrees/<branchName-with-slashes-as-dashes>`

`CreateWorktree` handles "already exists" by falling back to plain checkout.

## Core Operations

| Function | What it does |
|----------|-------------|
| `Commit(dir, msg)` | `add -A` -> status check -> `commit -m` (no-op if clean) |
| `HasMeaningfulChanges(dir, excludeFiles)` | Uncommitted + committed changes excluding noise |
| `MergeBranch(repoRoot, base, branch, msg)` | Checkout base -> `merge --squash` -> commit |
| `RebaseBranch(dir, baseBranch)` | Rebase with auto-abort on failure |
| `DiffStat(dir, baseBranch)` | merge-base -> `diff --stat` |
| `GetCurrentBranch(dir)` | Current branch name |
| `HasChanges(dir)` | Whether working tree has uncommitted changes |

## Conflict Resolution

```go
MergeInto(dir, baseBranch)       // Leaves conflicts in progress
GetConflictedFiles(dir)          // git diff --diff-filter=U
CompleteMerge(dir)               // add -A -> commit --no-edit
AbortMerge(dir)                  // git merge --abort
```

Used by workflow engine's `resolveConflicts()` which spawns Claude to fix conflict markers.

## Branch Operations

```go
DeleteBranch(repoRoot, branch string) error       // Normal delete
ForceDeleteBranch(repoRoot, branch string) error   // Force delete
GetLastCommitMessage(workDir string) (string, error)
```

## Commit Message Utilities

- `ConventionalCommitFromTitle(title)` — freeform text -> conventional commit format
- `GetSquashCommitMessage(repoRoot, baseBranch, branch, fallback)` — search branch commits for conventional format, fallback to first subject

## Patterns

- All git commands via `exec.Command("git", ...)` with `Dir` set
- `HasMeaningfulChanges()` excludes `.claude-output.log` and `CLAUDE.md`
- `getDefaultBranch()`: tries `symbolic-ref`, falls back to main/master/HEAD
