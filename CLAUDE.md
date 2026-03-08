# Sortie

A daemon that orchestrates Claude Code agents to work through tasks in parallel.
Each task runs in an isolated git worktree through a configurable multi-step workflow.

## Architecture

```
cmd/sortie/          CLI entry points (daemon, tui, task CRUD)
internal/
  config/            .sortie.yml parsing, project type detection
  daemon/            Background daemon that schedules and runs tasks
  workflow/          Task execution engine, system prompt injection, template resolution
  claude/            Claude Code process spawning and output parsing
  tui/               BubbleTea terminal UI for monitoring and approval
  db/                SQLite persistence for tasks and projects
  git/               Git worktree and merge operations
  agent/             Agent state management
  task/              Task lifecycle and state transitions
  tmux/              Tmux session management for interactive tasks
  notify/            Notification support
  client/            Client for daemon communication
claude-code-plugin/  Claude Code plugin with sortie-config skill
```

## Key Concepts

- **Workflow steps** are defined in `.sortie.yml` with templated prompts (`{{task.id}}`, `{{task.title}}`, etc.)
- **`BuildSystemPrompt()`** in `internal/workflow/system-prompt.go` constructs the system prompt passed to spawned Claude agents via `--system-prompt`
- **Template resolution** (`internal/workflow/template.go`) interpolates task variables and prior step artifacts into prompts
- **Git worktrees** isolate each task so parallel agents don't conflict

## Development

```bash
go build ./...        # Build
go test ./...         # Test
go build -o sortie ./cmd/sortie  # Build binary
```

## Code Style

- Follow existing patterns in the codebase
- Keep changes minimal and focused
- Use BubbleTea/Lip Gloss conventions for TUI code (see `/bubbletea` skill)
