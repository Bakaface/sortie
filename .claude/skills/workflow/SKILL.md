---
name: workflow
description: >
  Sortie's workflow engine: task execution, template resolution, system prompt injection,
  artifact handling, and merge/conflict resolution. Use when editing files in
  internal/workflow/, working on step execution, prompt templates, artifact I/O,
  loop conditions, summarizer, or on_complete actions.
---

# Workflow Engine

## Execution Flow (RunTask)

1. Create/reuse git worktree (skip if `task.Worktree == false`)
2. **Sync configured paths** via `SyncPathsToWorktree(srcRoot, dstRoot, paths)` — copies `worktree-sync-paths` from project root
3. `EnsureWorkDirs(worktreePath)` — create `.sortie/artifacts/`, `.sortie/images/`, `.sortie/logs/`
4. Copy attached images to `.sortie/images/`
5. For each step (from `task.StepIndex`):
   - Collect artifacts from prior `artifact: true` steps
   - Build `TemplateContext`, resolve prompt via `ResolveTemplate()`
   - Append artifact instruction if `step.Artifact == true`
   - `BuildSystemPrompt()` constructs system prompt string (passed via `--system-prompt` flag)
   - Spawn Claude (direct or tmux mode)
   - Validate meaningful code changes (skip for human/tmux)
   - Verify artifact written (retry if configured)
   - Evaluate loop conditions, check approval gates
6. Run summarizer, execute `on_complete` (commit/merge/none)

## File Map

| File | Purpose |
|------|---------|
| `engine.go` | Core orchestrator: `RunTask()`, `runClaudeStep()`, `runClaudeStepTmux()`, `executeOnComplete()`, `FinalizeTask()`, `ResumeAfterApproval()`, `resolveConflicts()`, `runSummarizer()` |
| `template.go` | `{{placeholder}}` interpolation via `ResolveTemplate()` |
| `system-prompt.go` | `BuildSystemPrompt()` — builds system prompt string for spawned Claude agents |
| `artifact.go` | Directory management, artifact read/write, image copying |
| `sync.go` | `SyncPathsToWorktree(srcRoot, dstRoot string, paths []string) error` — copies configured paths |

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
  artifacts/step_name.md     Step output artifacts
  images/image.png           Attached images
  logs/step_name.log         Per-worktree step logs
  step-prompt-*.txt          Prompt files for tmux steps
  run-step-*.sh              Wrapper scripts for tmux steps
```

Project-level logs: `.sortie/logs/{taskID}/{stepName}.log`

## Artifact & Directory Functions

```go
ArtifactsDir(worktreePath string) string
LogsDir(worktreePath string) string
LogPath(worktreePath, stepName string) string
EnsureWorkDirs(worktreePath string) error
ProjectLogsDir(dataDir string, taskID int64) string
ProjectLogPath(dataDir string, taskID int64, stepName string) string
ReadArtifact(worktreePath, stepName string) (string, error)
CollectArtifacts(worktreePath string, priorStepNames []string) map[string]string
ImagesDir(worktreePath string) string
CopyImagesToWorktree(worktreePath string, imagePaths []string) ([]string, error)
```

## Key Mechanisms

- **Merge**: serialized via `mergeMu`; squash-merge into base, Claude resolves conflicts, up to 3 retries
- **Loops**: evaluate at step end, check `MaxIterations` + `ExitCondition.ArtifactEmpty`, persist iteration to DB
- **Approval gates**: human steps pause at `AwaitingApproval`, tmux steps at `Tmux`
- **Environment**: `SORTIE_TASK_ID`, `SORTIE_STEP`, `SORTIE_WORKTREE`, `SORTIE_ARTIFACTS_DIR`

## Patterns

- Always use `TemplateContext` + `ResolveTemplate()` for prompt interpolation
- Check `fileExistsAndNonEmpty()` before treating an artifact as produced
- Step index persisted to DB after each step for crash recovery
- Tmux steps are fire-and-forget; daemon monitors session state separately
