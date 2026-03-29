---
name: workflow
description: >
  Sortie's workflow engine: task execution, template resolution, system prompt injection,
  step context capture, and merge/conflict resolution. Use when editing files in
  internal/workflow/, working on step execution, prompt templates, step context I/O,
  loop conditions, summarizer, or on_complete actions.
---

# Workflow Engine

## Execution Flow (RunTask)

1. Create/reuse git worktree (skip if `task.Worktree == false`)
2. **Sync configured paths** via `SyncPathsToWorktree(srcRoot, dstRoot string, paths config.WorktreeSyncPathsConfig)` — copies/links `worktree-sync-paths` from project root
3. Run `RunWorktreeSetupCommand()` if configured (worktree-only)
4. `EnsureWorkDirs(worktreePath)` — create `.sortie/logs/`
5. Copy attached images to `.sortie/images/`
6. For each step (from `task.StepIndex`):
   - Collect step contexts from prior steps (fetched from `task_steps` DB table)
   - Build `TemplateContext`, resolve prompt via `ResolveTemplate()`
   - `BuildSystemPrompt()` constructs system prompt string (passed via `--system-prompt` flag)
   - Spawn Claude (direct or tmux mode)
   - Capture `result` event from Claude's NDJSON output stream as step context
   - Store step context in `task_steps` DB table
   - Validate meaningful code changes (skip for human/tmux)
   - Evaluate loop conditions, check approval gates
7. Execute `on_complete` (commit/merge/none), run summarizer, clean up worktree (if merge)

## File Map

| File | Purpose |
|------|---------|
| `engine.go` | Core orchestrator: `Engine` struct, `NewEngine()`, `RunTask()`, `ResumeAfterApproval()` |
| `step.go` | Claude step execution: `runClaudeStep()`, `runClaudeStepTmux()`, `writeTmuxLogMessage()` |
| `merge.go` | Merge operations: `executeOnComplete()`, `executeMerge()`, `resolveConflicts()`, `cleanupMergedWorktree()`, `waitForCleanTarget()`, `AcquireMergeLock()`/`ReleaseMergeLock()` |
| `summarizer.go` | Summarization + finalization: `FinalizeTask()`, `runSummarizer()`, `summarizeChatLog()`, `RunWorktreeSetupCommand()`, `runClaudeSync()` |
| `template.go` | `{{placeholder}}` interpolation via `ResolveTemplate()` |
| `system-prompt.go` | `BuildSystemPrompt()` — builds system prompt string for spawned Claude agents |
| `artifact.go` | Directory management, image copying |
| `sync.go` | `SyncPathsToWorktree(srcRoot, dstRoot string, paths config.WorktreeSyncPathsConfig) error` — copies/links configured paths |

## Template System

See [references/templates.md](references/templates.md) for supported placeholders and context struct.

## System Prompt Injection

`BuildSystemPrompt(resolvedPrompt, systemPrompt, imageRelPaths)` returns a string containing:

```
[systemPrompt or default autonomous agent prompt]
---
# Task
[resolvedPrompt]
## Attached Images
- image paths
```

This string is passed to Claude via the `--system-prompt` flag rather than written to a file,
preventing task-specific instructions from leaking into git history.

## Directory Structure

```
worktree/.sortie/
  images/image.png           Attached images
  logs/step_name.log         Per-worktree step logs
  step-prompt-*.txt          Prompt files for tmux steps
  run-step-*.sh              Wrapper scripts for tmux steps
```

Project-level logs: `.sortie/logs/{taskID}/{stepName}.log`

## Directory Functions

```go
LogsDir(worktreePath string) string
LogPath(worktreePath, stepName string) string
EnsureWorkDirs(worktreePath string) error
ProjectLogsDir(dataDir string, taskID int64) string
ProjectLogPath(dataDir string, taskID int64, stepName string) string
ImagesDir(worktreePath string) string
CopyImagesToWorktree(worktreePath string, imagePaths []string) ([]string, error)
```

## Non-Worktree Mode

When `task.Worktree == false`:
- Worktree creation and branch resolution are skipped; `WorktreePath` is set to project root
- Path syncing (`SyncPathsToWorktree`) is skipped
- `on_complete: merge` falls back to a simple commit (no branch to merge)
- Worktree/branch cleanup on delete is skipped
- The summarizer uses `git diff --stat` against `base_branch` for context (may be empty if changes were already committed)

## Finalization (FinalizeTask)

`FinalizeTask()` handles tmux task completion:
1. Runs `executeOnComplete` (commit/merge/none) — merges first to unblock user
2. Sets `StatusSummarizing`, runs summarizer
3. Cleans up worktree via `cleanupMergedWorktree` (if merge was performed)
4. Called from `handleFinalizeTask` → `runFinalization` (async)

## Key Mechanisms

- **Merge**: serialized via `mergeMu`; squash-merge into base, Claude resolves conflicts, up to 3 retries
- **Loops**: evaluate at step end, check `MaxIterations` + `ExitCondition.StepContextEmpty`, persist iteration to DB
- **Approval gates**: human steps pause at `AwaitingApproval`, tmux steps at `Tmux`
- **Summarization strategy**: per-step `summarization_strategy` controls how step context is captured. `last_message` (default) stores Claude's result event text. `summarize_chat` stores last_message immediately, then spawns a background goroutine that runs `summarizeChatLog()` (haiku model) against the full step log and overwrites the context via `UpdateTaskStepContext()` when done.
- **Environment**: `SORTIE_TASK_ID`, `SORTIE_STEP`, `SORTIE_WORKTREE`

## Patterns

- Always use `TemplateContext` + `ResolveTemplate()` for prompt interpolation
- Step context captured from Claude's NDJSON `result` event and stored in `task_steps` table
- Step index persisted to DB after each step for crash recovery
- Tmux steps are fire-and-forget; daemon monitors session state separately
