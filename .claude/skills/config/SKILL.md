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
2. `~/.config/sortie/config.yaml` (global daemon)
3. `~/.sortie.yml` (global workflow defaults)
4. `./.sortie.yml` (project-specific, wins)

## Key Types

```go
type ProjectConfig struct {
    MaxWorkers       int
    DefaultPriority  string
    Yolo             bool              // Skip Claude permissions
    Git              GitConfig         // BaseBranch, BranchTemplate, OnComplete
    Workflows        WorkflowsConfig   // one-off, tasks, init
    SystemPrompt     string            // Override for generated CLAUDE.md
    Verification     VerificationConfig
    Notifications    NotificationsConfig
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
    Tmux     *bool           // Override workflow-level tmux
    Timeout  time.Duration   // Default 30m
    Human    bool            // Approval gate
    Artifact bool            // Expect output artifact
    Loop     *LoopConfig     // Optional retry loop
}
```

**Loop validation**: goto must reference earlier step, max_iterations >= 1, no human/tmux on looped steps, no overlapping ranges.

## Branch Templates

Default: `"sortie/{{task_id}}-{{task_slug}}"`

Variables: `{{task_id}}`, `{{task_slug}}`, `{{task.title}}`, `{{task.id}}`, `{{task.slug}}`

## Project Detection (detect.go)

`DetectProject()` probes for `package.json`, `go.mod`, `Gemfile`, Python markers, `Cargo.toml`. Returns `DetectedProject{Type, Name, Commands}`. Detects `bun.lockb` and swaps npm -> bun.

## Patterns

- Access workflows via `ListWorkflowNames()`, `GetWorkflow()`, `ListPredefinedTaskNames()`
- `ClaudeConfig.Args()` adds `--dangerously-skip-permissions` if Yolo
- Config validation at parse time; invalid configs return errors
- New fields: add to struct + YAML tag + merge logic + test fixtures
