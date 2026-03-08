---
name: daemon
description: >
  Sortie's background daemon: Unix socket server, request handlers, client-server protocol,
  task polling, agent lifecycle, and event broadcasting. Use when editing files in
  internal/daemon/, working on server startup, message handling, task scheduling,
  agent state changes, or pub/sub broadcasting.
---

# Daemon Architecture

## Server Structure

```go
type Server struct {
    cfg         *config.Config
    listener    net.Listener         // Unix socket
    manager     *agent.Manager       // Concurrent agent execution
    database    *db.DB
    projects    map[int64]*projectContext  // Lazy-loaded per-project config+engine
    clients     map[net.Conn]bool    // All connected clients
    subscribers map[net.Conn]bool    // Pub/sub subscribers
}
```

**Startup**: ensure dirs -> PID file -> Unix socket -> register agent callback -> recover orphans -> spawn `acceptLoop()` + `taskPollerLoop()`

## File Map

| File | Purpose |
|------|---------|
| `server.go` | Lifecycle, connection handling, project context caching |
| `handlers.go` | Business logic for all message types |
| `broadcast.go` | Event broadcasting, agent state change handling |
| `protocol.go` | Message types, request/response structs |
| `poller.go` | Background polling for pending tasks |

## Protocol

See [references/protocol.md](references/protocol.md) for all message types and payload structs.

JSON + newline framing, 10MB scanner buffer. `Message{Type, Payload}` structure.

## Handler Patterns

- Handlers receive `(conn, payload)`, respond via `sendMessage()` or `sendError()`
- `handleCreateTask`: creates task + async AI title refinement (30s timeout)
- `handleContinueTask`: complex — resumes approval/tmux, or creates tmux for terminal tasks
- `handleFinalizeTask`: async summarizer + on_complete for tmux sessions

### Adding New Message Types

1. Add constant to `protocol.go`
2. Create request/response structs
3. Add handler in `handlers.go`
4. Wire in `handleMessage()` switch

## Task Polling

`taskPollerLoop()` at configurable interval -> `checkPendingTasks()` -> `startTaskAgent()`:
1. Claim task atomically in DB
2. Get project context (lazy-loaded)
3. Determine work dir (worktree path or repo root)
4. Create runner wrapping `engine.RunTask()`
5. Spawn agent via manager

## Broadcasting

`onAgentStateChange()` fires on every state transition:
- Broadcasts `MsgAgentUpdate` to subscribers
- On terminal states: updates task, fires notifications
- `checkProjectTasksDone()`: fires `AllTasksCompleted` when all project tasks terminal

## Recovery

`recoverOrphanedTasks()`: running/init -> pending, finalizing -> tmux, summarizing -> pending

## Patterns

- Use `getProjectContext()` for project-specific config (lazy-loads and caches)
- Broadcasting happens outside locks to avoid deadlocks
- Agent state change callback fires outside manager mutex
- Database ops should be atomic where possible (e.g., `ClaimTask()`)
