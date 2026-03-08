# Configuration System

TRIGGER when: editing files in `internal/config/`, working on `.sortie.yml` parsing, project type detection, workflow definitions, or configuration loading.

## Loading Hierarchy (highest priority last)

1. Built-in defaults
2. `~/.config/sortie/config.yaml` (global daemon config)
3. `~/.sortie.yml` (global workflow defaults)
4. `./.sortie.yml` (project-specific, highest priority)

## Key Types

```go
type Config struct {
    Daemon       DaemonConfig
    Claude       ClaudeConfig
    Project      ProjectConfig       // Merged from all sources
    DataDir      string
}

type ProjectConfig struct {
    MaxWorkers       int
    DefaultPriority  string
    Yolo             bool             // Skip Claude permissions
    Git              GitConfig        // BaseBranch, BranchTemplate, OnComplete
    Workflows        WorkflowsConfig  // one-off, tasks, init
    SystemPrompt     string           // Override for generated CLAUDE.md
    Verification     VerificationConfig
    Notifications    NotificationsConfig
    ValidateArtifact bool
}
```

## Workflow Configuration

Three workflow categories in `.sortie.yml`:

```yaml
workflows:
  tasks:        # User-created tasks with title + description
    - name: implement
      steps: [...]
  one-off:      # Predefined jobs with built-in descriptions
    - name: refactor
      description: "..."
      steps: [...]
  init:         # Initialization pipelines
    - name: from-prd
      steps: [...]
```

**Legacy format support:** plain list (`workflows: [...]`) treated as task workflows. Ancient format (`workflow: { steps: [...] }`) also supported.

### StepConfig

```go
type StepConfig struct {
    Name     string
    Prompt   string          // Templated prompt text
    Mode     string          // "automatic" etc.
    Tmux     *bool           // Override workflow-level tmux setting
    Timeout  time.Duration   // Default 30m
    Human    bool            // Approval gate
    Artifact bool            // Expect output artifact
    Loop     *LoopConfig     // Optional loop/retry
}

type LoopConfig struct {
    Goto          string     // Target step name (must reference earlier step)
    MaxIterations int        // >= 1
    ExitCondition struct {
        ArtifactEmpty string // Step name whose empty artifact triggers exit
    }
}
```

**Loop validation rules:** goto must reference earlier step (no forward jumps), steps with loops can't have human/tmux, no overlapping loop ranges.

## Branch Template Resolution

Template: `"sortie/{{task_id}}-{{task_slug}}"` (default)

Variables: `{{task_id}}`, `{{task_slug}}`, `{{task.title}}`, `{{task.id}}`, `{{task.slug}}`

Methods: `ResolveBranchName()`, `ResolveBranchTemplate()`

## Project Detection (detect.go)

`DetectProject()` probes for: `package.json`, `go.mod`, `Gemfile`, Python markers, `Cargo.toml`

Returns `DetectedProject` with Type (node/go/ruby/python/rust/unknown), Name, Commands (test/lint/build). Special: detects `bun.lockb` and replaces npm with bun commands.

## Patterns to Follow

- Access workflows via `ListWorkflowNames()`, `GetWorkflow()`, `ListPredefinedTaskNames()`, `GetPredefinedTask()`, `ListInitWorkflowNames()`, `GetInitWorkflow()`
- `ClaudeConfig.Args()` adds `--dangerously-skip-permissions` if Yolo=true
- Use `cfg.EnsureDirs()` on startup to create data directories
- Config validation happens at parse time; invalid configs return errors, not panics
- When adding new config fields: add to the struct, add YAML tag, handle in merge logic, add to test fixtures
