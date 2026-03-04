# Sortie

A daemon that orchestrates [Claude Code](https://docs.anthropic.com/en/docs/claude-code) agents to work through tasks in parallel. Each task runs in an isolated git worktree through a configurable multi-step workflow, with real-time monitoring via a terminal UI.

## Features

- **Parallel execution** — run multiple Claude Code agents concurrently (configurable worker limit)
- **Multi-step workflows** — chain planning, implementation, review, and approval steps with artifact passing between them
- **Git worktree isolation** — each task gets its own worktree and branch, no conflicts between parallel tasks
- **Approval gates** — pause workflows at any step for human review
- **Review loops** — iterate between implementation and review steps until quality checks pass
- **TUI** — monitor tasks, view live logs, approve/reject from the terminal
- **Auto-merge** — optionally commit or merge completed work back to the base branch

## Quick Start

```bash
# Build
go build -o sortie ./cmd/sortie

# Initialize in your project
cd /path/to/your/repo
sortie init

# Add tasks, then start the daemon
sortie daemon start

# Monitor with the TUI
sortie tui
```

## Workflow Configuration

Workflows are defined in `.sortie.yml` at your project root:

```yaml
max_workers: 3
git:
  base_branch: main
  on_complete: merge

workflows:
  tasks:
    - name: sensible
      steps:
        - name: implementing
          prompt: |
            Implement task #{{task.id}}: {{task.title}}
            {{task.description}}
          artifact: true
          timeout: 30m

        - name: reviewing
          prompt: |
            Review the implementation.
            {{artifacts.implementing}}
          timeout: 20m
```

Template variables: `{{task.id}}`, `{{task.title}}`, `{{task.description}}`, `{{task.branch}}`, `{{git.base_branch}}`, `{{artifacts.<step>}}`.

## Requirements

- Go 1.24+
- git
- [Claude Code](https://docs.anthropic.com/en/docs/claude-code) CLI (`claude`) in PATH
