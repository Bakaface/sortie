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
- **Template resolution** (`internal/workflow/template.go`) interpolates task variables and prior step contexts into prompts
- **Git worktrees** isolate each task so parallel agents don't conflict

## Development

```bash
go build ./...        # Build
go test ./...         # Test
go build -o sortie ./cmd/sortie  # Build binary
```

## Domain Context

Each package directory contains a `CLAUDE.md` with critical invariants and a pointer to the
corresponding domain skill. These auto-load when you read/edit files in that directory.

For substantial changes, load the full domain skill (referenced in each subdirectory's `CLAUDE.md`)
to get file maps, struct definitions, protocol details, and deeper conventions.

## Verifying Non-Interactive Output

The TUI cannot be run interactively from this harness. When modifying any `View()` method or layout code, you MUST verify rendering correctness programmatically — do not rely on reasoning about the math alone.

Write a standalone Go program (or test) that:
1. Constructs the component with realistic data
2. Calls `View()` or the relevant render function
3. Splits the output into lines and checks `lipgloss.Width()` on each line
4. Asserts structural properties (uniform width, border alignment, expected content)

Run this program via Bash to confirm correctness before telling the user it's fixed.

## Code Style

- Follow existing patterns in the codebase
- Keep changes minimal and focused
- Use BubbleTea/Lip Gloss conventions for TUI code (see `/bubbletea` skill)
