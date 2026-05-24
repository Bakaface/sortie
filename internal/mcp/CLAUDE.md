# internal/mcp — MCP Server

Model Context Protocol server that lets Claude Code (or any MCP client) talk to a running
sortie daemon over its Unix socket. Exposes a **safe-by-default** tool surface — no
destructive operations.

## Critical Invariants

- **Safe surface only**: `create_task`, `get_task`, `list_workflows`, `update_step_context`.
  Do not add `delete_task`, `stop_agent`, or `retry_task` here without an explicit design
  decision. `update_step_context` is admitted because the daemon enforces that it can only
  write to the calling task's currently-active step (see `handleUpdateActiveStepContext`).
- **Project resolution is per-call**: `resolveProjectPath()` in `project.go` falls back to
  `git rev-parse --show-toplevel` against the caller's cwd when `project_path` is omitted,
  matching how the TUI resolves projects so tasks land on the same project row.
- **Daemon connection held for the MCP process lifetime**: `Serve()` connects once at start
  (fails fast if the daemon isn't running) and reuses that `*client.Client`.

## File Map

| File | Purpose |
|------|---------|
| `server.go` | `Serve(cfg)` entry point, tool registration, MCP-result error helper |
| `project.go` | `resolveProjectPath()` — explicit → cwd → git toplevel |
| `tool_create_task.go` | `create_task` tool definition + handler |
| `tool_get_task.go` | `get_task` tool definition + handler |
| `tool_list_workflows.go` | `list_workflows` tool definition + handler |
| `tool_update_step_context.go` | `update_step_context` tool definition + handler |

## Conventions

- Tool-level errors return `(mcp.NewToolResultError(...), nil)` — never propagate as transport
  errors. Use `resultErr()` from `server.go`.
- Keep each tool's argument schema co-located with its handler.
