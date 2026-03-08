---
name: cli
description: >
  Sortie's CLI entry point and command registration: subcommands, flags, project
  config enforcement, and command routing. Use when editing files in cmd/sortie/,
  adding new CLI commands, modifying flags, or changing pre-run validation logic.
---

# CLI Entry Point

`cmd/sortie/main.go` — single-file CLI built on Cobra.

## Command Registration

All subcommands registered in `init()`:

| Command | Flags | Purpose |
|---------|-------|---------|
| `daemon start` | — | Start background daemon |
| `daemon stop` | — | Stop daemon |
| `daemon status` | — | Check daemon status |
| `tui` | `--global/-g` | Launch terminal UI |
| `init` | — | Initialize `.sortie.yml` |
| `tasks` | — | List tasks (CLI) |
| `start` | — | Start daemon (alias) |
| `list` | — | List tasks (alias) |
| `stop` | — | Stop daemon (alias) |
| `retry` | — | Retry failed task |
| `continue` | — | Continue task |
| `logs` | `--tail/-n` | View step logs |
| `cleanup` | — | Clean worktrees |
| `attach` | — | Attach to tmux session |
| `create` | `--priority/-p`, `--branch/-b`, `--workflow/-w`, `--no-worktree` | Create task |
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
1. Loads config via `config.Load()`
2. Skips project check for daemon subcommands (`start`, `stop`, `status`) and `tui`
3. For all other non-exempted commands, requires `.sortie.yml` to exist (returns error if missing)
4. Stores loaded config in Cobra command context

## Patterns

- All commands use the shared `client.Client` for daemon communication
- Task ID arguments parsed as `int64` from positional args
- Commands that modify tasks require a running daemon
- `--no-worktree` flag on `create` sets `Worktree: false` (default true)
