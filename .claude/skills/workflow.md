# Workflow Engine

TRIGGER when: editing files in `internal/workflow/`, working on task execution, template resolution, CLAUDE.md generation, artifact handling, step progression, or merge/conflict resolution.

## Overview

The workflow engine orchestrates Claude Code agents through multi-step task workflows in isolated git worktrees. Core files:

- `engine.go` - Main orchestrator, step execution, merge logic
- `template.go` - `{{placeholder}}` variable interpolation
- `claudemd.go` - CLAUDE.md instruction file generation
- `artifact.go` - Directory management, artifact I/O, image handling

## Engine Architecture

```go
type Engine struct {
    cfg      *config.Config
    database *db.DB
    notifier *notify.Notifier
    repoRoot string
    dataDir  string
    mergeMu  sync.Mutex  // Serializes merge operations on shared repoRoot
}
```

### Execution Flow (RunTask)

1. Create/reuse git worktree (or skip if worktree disabled)
2. Copy attached images to `.sortie/images/`
3. For each step (from `task.StepIndex` onwards):
   - Collect artifacts from prior steps marked `artifact: true`
   - Build `TemplateContext` with task, artifact, git, loop variables
   - Resolve prompt templates via `ResolveTemplate()`
   - Append artifact instruction if `step.Artifact == true`
   - Call `InjectClaudeMD()` to write CLAUDE.md in worktree
   - Spawn Claude (direct or tmux mode)
   - Validate meaningful code changes (skip for human/tmux steps)
   - Verify artifact written (with retry if configured)
   - Evaluate loop conditions
   - Check approval gates (human/tmux steps pause here)
4. Run summarizer after all steps
5. Execute `on_complete` action (commit/merge/none)

### Key Methods

| Method | Purpose |
|--------|---------|
| `RunTask()` | Main workflow loop |
| `runClaudeStep()` | Spawn Claude with prompt, collect output |
| `runClaudeStepTmux()` | Fire-and-forget tmux session for interactive work |
| `executeOnComplete()` | Post-workflow: commit, squash-merge, or none |
| `FinalizeTask()` | Complete tmux-continued tasks (summarize + on_complete) |
| `ResumeAfterApproval()` | Re-enter RunTask loop after human approval |
| `resolveConflicts()` | Spawn Claude to fix merge conflicts |
| `runSummarizer()` | Collect artifacts, generate task summary |

## Template System

```go
type TemplateContext struct {
    Task      TaskVars                // ID, Title, Description, Slug, Branch, Images
    Artifacts map[string]string       // step_name -> artifact content
    Git       GitVars                 // BaseBranch, RepoRoot
    Loop      LoopVars                // Iteration, MaxIterations
}
```

**Supported placeholders:**
- `{{task.id}}`, `{{task.title}}`, `{{task.description}}`, `{{task.slug}}`, `{{task.branch}}`
- `{{task.images}}` - newline-joined paths
- `{{git.base_branch}}`, `{{git.repo_root}}`
- `{{loop.iteration}}`, `{{loop.max_iterations}}`
- `{{artifacts.step_name}}` - content from named step's artifact

Pattern: regex `\{\{([a-zA-Z0-9_.]+)\}\}` — unknown keys pass through unchanged.

## CLAUDE.md Injection

`InjectClaudeMD(worktreePath, resolvedPrompt, systemPrompt, imageRelPaths)` writes:

```markdown
[systemPrompt or default autonomous agent prompt]
---
# Task
[resolvedPrompt]
## Attached Images
- image paths
```

Default system prompt instructs autonomous work without user input.

## Artifact & Directory Structure

```
worktree/.sortie/
  artifacts/step_name.md     # Step output artifacts
  images/image.png           # Attached images
  logs/step_name.log         # Per-worktree step logs
  step-prompt-*.txt          # Prompt files for tmux steps
  run-step-*.sh              # Wrapper scripts for tmux steps
```

Project-level logs: `.sortie/logs/{taskID}/{stepName}.log`

Key functions: `ArtifactsDir()`, `LogsDir()`, `ImagesDir()`, `EnsureWorkDirs()`, `ReadArtifact()`, `CollectArtifacts()`, `CopyImagesToWorktree()`

## Environment Variables Set on Claude Process

```
SORTIE_TASK_ID, SORTIE_STEP, SORTIE_WORKTREE, SORTIE_ARTIFACTS_DIR
```

## Merge & Conflict Resolution

1. Commit changes in worktree
2. Squash-merge into base branch (serialized via `mergeMu`)
3. On conflict: list files, spawn Claude with conflict resolution prompt, validate
4. Retry up to 3 times (re-sync from base between attempts)
5. Clean up worktree and branch on success

## Loop Mechanism

- Evaluated at end of each step with `step.Loop` config
- Checks `MaxIterations` and `ExitCondition.ArtifactEmpty`
- Increments `task.LoopIteration`, persists to DB
- Resets to 0 when loop completes

## Patterns to Follow

- Always use `TemplateContext` and `ResolveTemplate()` for prompt interpolation; never hardcode task fields into prompts
- Artifact validation: check `fileExistsAndNonEmpty()` before treating an artifact as produced
- Step index is persisted to DB after each step so recovery works correctly
- The `mergeMu` mutex is critical — merge operations on the shared repo root must be serialized
- Tmux steps are fire-and-forget; the daemon monitors session state separately
