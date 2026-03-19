---
name: config
description: >
  Sortie's configuration system: .sortie.yml parsing, loading hierarchy, workflow
  definitions, step/loop config, project type detection, and branch template resolution.
  Use when editing files in internal/config/, working on .sortie.yml parsing,
  workflow definitions, project detection, or configuration loading.
---

# Configuration System

## Loading Hierarchy (highest priority last)

1. Built-in defaults
2. `~/.config/sortie/config.yaml` (global daemon — `GlobalConfig`)
3. `~/.sortie.yml` (global workflow defaults)
4. `./.sortie.yml` (project-specific, wins)

```go
Load() (*Config, error)                    // From current directory
LoadForProject(projectDir string) (*Config, error)  // From specific path
```

## Key Types

### ProjectConfig (.sortie.yml)

```go
type ProjectConfig struct {
    MaxWorkers               int
    DefaultPriority          string
    Yolo                     *bool               // Skip Claude permissions (pointer for merge)
    ValidateArtifact         *bool
    Verification             *VerificationConfig
    Git                      GitConfig            // BaseBranch, BranchTemplate, OnComplete
    Workflows                ProjectWorkflows     // tasks, one-off, init
    SystemPrompt             string
    WorktreeSyncPaths        WorktreeSyncPathsConfig // Paths to copy/link into worktrees
    Notifications            *NotificationsConfig
    TmuxNestedAttachBehavior string               // "switch" (default) or "nest"
}
```

### WorkflowConfig

```go
type WorkflowConfig struct {
    Name              string
    Description       string
    Tmux              bool
    Steps             []StepConfig
    SummarizerPrompt  string
    WorktreeSyncPaths WorktreeSyncPathsConfig // Per-workflow sync paths (override project-level)
}
```

### GlobalConfig (~/.config/sortie/config.yaml)

```go
type GlobalConfig struct {
    MaxWorkers               int
    Yolo                     *bool
    ValidateArtifact         *bool
    Verification             *VerificationConfig
    Notifications            NotificationsConfig
    TmuxNestedAttachBehavior string
}
```

### VerificationConfig

```go
type VerificationConfig struct {
    ArtifactRetry    bool
    MaxRetries       int
    VerifySummarizer bool
}
```

## Workflow Categories

```yaml
workflows:
  tasks:    [...]   # User-created tasks with title + description
  one-off:  [...]   # Predefined jobs with built-in descriptions
  init:     [...]   # Initialization pipelines
```

Legacy formats supported: plain list (`workflows: [...]`) and singular (`workflow: { steps: [...] }`).

## StepConfig

```go
type StepConfig struct {
    Name, Prompt, Mode string
    Tmux     *bool          // Override workflow-level tmux
    Timeout  string         // Parsed duration, default 30m (DefaultStepTimeout)
    Human    bool           // Approval gate
    Artifact bool           // Expect output artifact
    Loop     *LoopConfig    // Optional retry loop
}
```

**Loop validation**: goto must reference earlier step, max_iterations >= 1, no human/tmux on looped steps, no overlapping ranges.

## Worktree Sync Paths

```go
type WorktreeSyncPathsConfig struct {
    Copy []string   // Paths to copy into worktrees
    Link []string   // Paths to symlink into worktrees
}

GetWorktreeSyncPaths(wf *WorkflowConfig) WorktreeSyncPathsConfig
```

Supports two modes: `copy` (full recursive copy) and `link` (symlink to source).
Legacy plain-list format (`[]string`) is treated as copy paths for backward compatibility.
Returns workflow-level paths if set, otherwise project-level `WorktreeSyncPaths`.

## Branch Templates

Default: `"sortie/{{task_id}}-{{task_slug}}"`

Variables: `{{task_id}}`, `{{task_slug}}`, `{{task.title}}`, `{{task.id}}`, `{{task.slug}}`

## Config Accessors

```go
GetWorkflow(name string) *WorkflowConfig
GetPredefinedTask(name string) *WorkflowConfig     // From one-off workflows
GetInitWorkflow(name string) *WorkflowConfig
DefaultWorkflow() WorkflowConfig                    // Built-in default workflow
ListWorkflowNames() []string
ListPredefinedTaskNames() []string
ListInitWorkflowNames() []string
GetStepTimeout(step StepConfig) time.Duration       // Parses Timeout string, falls back to 30m
GetWorkflowSteps() []StepConfig                     // Steps from first tasks workflow
```

## Project Detection (detect.go)

`DetectProject()` probes for `package.json`, `go.mod`, `Gemfile`, Python markers, `Cargo.toml`. Returns `DetectedProject{Type, Name, Commands}`. Detects `bun.lockb` and swaps npm -> bun.

## Patterns

- Access workflows via `ListWorkflowNames()`, `GetWorkflow()`, `ListPredefinedTaskNames()`
- `ClaudeConfig.Args()` adds `--dangerously-skip-permissions` if Yolo
- Config validation at parse time; invalid configs return errors
- New fields: add to struct + YAML tag + merge logic + test fixtures
