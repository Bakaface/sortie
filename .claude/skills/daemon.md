# Daemon Architecture

TRIGGER when: editing files in `internal/daemon/`, working on the background server, request handlers, client-server protocol, task polling, broadcasting, or agent lifecycle management.

## Overview

The daemon is the background server that schedules tasks, manages agents, and communicates with the TUI/CLI via a Unix socket. Files:

- `server.go` - Server lifecycle, connection handling, project context caching
- `handlers.go` - Business logic for all 18+ message types
- `broadcast.go` - Event broadcasting to subscribed clients
- `protocol.go` - Message types, request/response structs, JSON encoding
- `poller.go` - Background polling for pending tasks

## Server Architecture

```go
type Server struct {
    cfg       *config.Config
    listener  net.Listener        // Unix socket
    manager   *agent.Manager      // Concurrent agent execution
    database  *db.DB
    notifier  *notify.Notifier
    projectsMu sync.RWMutex
    projects   map[int64]*projectContext  // Lazy-loaded per-project config+engine
    mu         sync.RWMutex
    clients    map[net.Conn]bool   // All connected clients
    subscribers map[net.Conn]bool  // Pub/sub subscribers
    ctx        context.Context
    cancel     context.CancelFunc
}
```

### Startup Flow
1. Ensure directories, write PID file
2. Create Unix socket at `cfg.Daemon.SocketPath`
3. Register agent state change callback
4. Recover stale/orphaned tasks from unclean shutdown
5. Spawn `acceptLoop()` + `taskPollerLoop()` goroutines

### Project Context (lazy-loaded)
```go
type projectContext struct {
    cfg      *config.Config      // Project .sortie.yml
    engine   *workflow.Engine
    repoRoot string
}
```
Cached per project ID in `projects` map. Created on first task for that project.

## Protocol

**Framing:** JSON + newline, line-based scanning (10MB buffer)

```go
type Message struct {
    Type    MessageType     `json:"type"`
    Payload json.RawMessage `json:"payload,omitempty"`
}
```

**Client -> Server commands:**
`list_agents`, `list_tasks`, `start_agent`, `stop_agent`, `subscribe`, `unsubscribe`, `send_input`, `get_output`, `get_task`, `retry_task`, `get_logs`, `create_task`, `continue_task`, `finalize_task`, `delete_task`, `update_priority`, `update_field`, `ping`, `shutdown`

**Server -> Client events:**
`agent_list`, `agent_update`, `task_list`, `task_update`, `output_chunk`, `ok`, `error`, `pong`

## Handler Patterns

Key handlers in `handlers.go`:

| Handler | Behavior |
|---------|----------|
| `handleCreateTask` | Creates task + async AI title refinement in background |
| `handleContinueTask` | Complex: resumes approval/tmux tasks, or creates tmux session for terminal tasks |
| `handleFinalizeTask` | Async: runs summarizer + on_complete for tmux sessions |
| `handleRetryTask` | Kills tmux, resets status to pending |
| `handleDeleteTask` | Removes task, kills agents, cleans worktree/logs |
| `handleGetOutput` | Returns agent output lines with pagination (fromLine) |
| `handleGetLogs` | Returns step-specific or all logs with tail support |

### Important Details
- `handleCreateTask`: branch resolution uses `ResolveBranchName()` with template variables
- `handleContinueTask`: creates worktree if task doesn't have one, writes CLAUDE.md, creates tmux session
- Title generation runs async with 30s timeout using Claude haiku model
- No-worktree tasks run in project root directory

## Broadcasting

`onAgentStateChange()` fires on every agent state transition:
- Broadcasts `MsgAgentUpdate` to all subscribers
- On terminal states (completed/failed): updates task status, kills tmux, fires notifications
- `checkProjectTasksDone()`: fires `AllTasksCompleted` notification when all project tasks are terminal

## Task Polling

`taskPollerLoop()` runs at configurable interval:
1. Calls `checkPendingTasks()`
2. Fetches claimable tasks from DB (respects priority + dependency ordering)
3. For each unclaimed task: `startTaskAgent()` spawns an agent

`startTaskAgent()`:
1. Claims task atomically in DB (prevents duplicates)
2. Gets project context and workflow engine
3. Determines work directory (worktree path or repo root)
4. Creates runner function wrapping `engine.RunTask()`
5. Spawns agent via manager

## Recovery on Startup

`recoverOrphanedTasks()` handles unclean shutdown:
- Running/init -> reset to pending
- Finalizing -> reset to tmux
- Summarizing/merge-blocked -> reset to pending
- Awaiting-approval/tmux/artifact-missing -> log (user must continue)

## Patterns to Follow

- All handlers receive `(conn, payload)` and send responses via `sendMessage()` or `sendError()`
- Use `getProjectContext()` for project-specific config; it lazy-loads and caches
- Broadcasting happens outside of locks to avoid deadlocks
- Agent state change callback fires outside manager mutex
- Database operations should be atomic where possible (e.g., `ClaimTask()`)
- When adding new message types: add to `protocol.go` constants, create request/response structs, add handler in `handlers.go`, wire in `handleMessage()` switch
