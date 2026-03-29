---
name: cli
description: >
  Sortie's CLI entry point and command registration: subcommands, flags, project
  config enforcement, and command routing. Use when editing files in cmd/sortie/,
  adding new CLI commands, modifying flags, or changing pre-run validation logic.
---

# CLI Entry Point

Cobra-based CLI split across multiple files in `cmd/sortie/`.

| File | Contents |
|------|----------|
| `main.go` | Root command, `PersistentPreRunE`, `noProjectRequired` map, `init()` registration |
| `daemon.go` | `daemon start`, `daemon stop`, `daemon status` subcommands |
| `task_crud.go` | `create`, `edit`, `delete` commands |
| `tasks.go` | `tasks`, `start`, `stop`, `list`/`agents`, `retry`, `revert`, `continue`, `logs`, `cleanup`, `attach`, `detach`, `attach-branch` commands |
| `tui.go` | `tui` command, `resolveProjectMode()` helper |
| `init.go` | `init` command (scaffolds `.sortie.yml`) |
| `helpers.go` | `taskTableRow`, `printTaskTable()`, `truncateStr()`, `completeTaskIDs()` shell completion |

## Command Registration

All subcommands registered in `init()`:

| Command | Flags | Purpose |
|---------|-------|---------|
| `daemon start` | `--foreground/-f` | Start background daemon |
| `daemon stop` | — | Stop daemon |
| `daemon status` | — | Check daemon status |
| `tui` | `--global/-g` | Launch terminal UI |
| `init` | — | Initialize `.sortie.yml` |
| `tasks [id]` | — | List all tasks or show task detail |
| `start` | — | Start agent for task |
| `agents` / `list` | — | List running agents |
| `stop` | — | Stop running task |
| `retry` | — | Retry failed task |
| `revert` | — | Revert all commits made by a task |
| `continue` | — | Continue task (awaiting-approval, completed, or failed) |
| `logs` | `--tail/-n` | View step logs |
| `cleanup` | — | Remove worktrees for completed/failed tasks |
| `attach` | — | Attach to tmux session |
| `detach` | — | Detach worktree branch so it can be checked out elsewhere |
| `attach-branch` | — | Reattach branch to worktree after detach |
| `create` | `--priority/-p`, `--branch/-b`, `--workflow/-w`, `--no-worktree`, `--target`, `--checkout` | Create task |
| `edit` | `--title/-t`, `--description/-d`, `--context/-c`, `--priority/-p` | Edit task fields |
| `delete` | `--yes/-y` | Delete task |

## Project Config Enforcement

### noProjectRequired Map

```go
var noProjectRequired = map[string]bool{
    "init": true, "help": true, "completion": true,
    "__complete": true, "__completeNoDesc": true,
    "start": true, "stop": true, "status": true,
}
```

### PersistentPreRunE

Runs before every command:
1. Loads config via `config.Load()` into package-level `var cfg *config.Config`
2. Skips project check for daemon subcommands (`start`, `stop`, `status`) and `tui`
3. For all other non-exempted commands, requires `.sortie.yml` to exist (returns error if missing)

## Patterns

- Most commands use `client.Client` for daemon communication
- `cleanup` and `tui` access the database directly via `db.Open()`, bypassing the daemon
- `tasks` falls back to direct DB access (`listTasksFromDB()`) when the daemon is not running
- `cleanup` modifies task state (clears worktree paths) without requiring the daemon
- Task ID arguments parsed as `int64` from positional args
- `--no-worktree` flag on `create` sets `Worktree: false` (default true)
- `--target` overrides `git.base_branch` for the task's target branch
- `--checkout` checks out an existing branch instead of creating a new one (mutually exclusive with `--branch`)
