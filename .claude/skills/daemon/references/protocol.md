# Daemon Protocol Reference

## Message Structure

```go
type Message struct {
    Type    MessageType     `json:"type"`
    Payload json.RawMessage `json:"payload,omitempty"`
}
```

## Client -> Server Commands

| Type | Request Struct | Purpose |
|------|---------------|---------|
| `list_agents` | — | List running agents |
| `list_tasks` | `ListTasksRequest{ProjectID, ProjectName}` | List tasks (optionally filtered) |
| `start_agent` | `StartAgentRequest{TaskID}` | Start agent for task |
| `stop_agent` | `StopAgentRequest{AgentID}` | Stop running agent |
| `create_task` | `CreateTaskRequest{Description, Workflow, Priority, BranchName, ProjectPath, Worktree, Images}` | Create new task |
| `get_task` | `GetTaskRequest{TaskID}` | Get single task |
| `delete_task` | `DeleteTaskRequest{TaskID}` | Delete task + cleanup |
| `retry_task` | `RetryTaskRequest{TaskID}` | Reset task to pending |
| `continue_task` | `ContinueTaskRequest{TaskID, Workflow, Prompt}` | Resume/continue task |
| `finalize_task` | `FinalizeTaskRequest{TaskID}` | Finalize tmux session |
| `get_output` | `GetOutputRequest{AgentID, FromLine}` | Agent output (paginated) |
| `get_logs` | `GetLogsRequest{TaskID, Step, TailLines}` | Step logs |
| `send_input` | `SendInputRequest{AgentID, Input}` | Send input to agent |
| `update_priority` | `UpdatePriorityRequest{TaskID, Priority}` | Change priority |
| `update_field` | `UpdateFieldRequest{TaskID, Field, Value}` | Update title/description/context |
| `subscribe` | — | Subscribe to events |
| `unsubscribe` | — | Unsubscribe |
| `ping` | — | Health check |
| `shutdown` | — | Graceful shutdown |

## Server -> Client Events

| Type | Payload | Purpose |
|------|---------|---------|
| `agent_list` | `[]AgentInfo` | Agent listing response |
| `agent_update` | `AgentInfo` | Agent state change |
| `task_list` | `[]TaskInfo` | Task listing response |
| `task_update` | `TaskInfo` | Task state change |
| `output_chunk` | `OutputChunk` | Agent output lines |
| `ok` | — | Success acknowledgment |
| `error` | `ErrorResponse{Message}` | Error response |
| `pong` | — | Health check response |

## Key Types

```go
type TaskInfo struct {
    // Mirrors task.Task fields + computed metadata
    ID, ProjectID, Title, Description, Slug, Workflow, Status, Priority string
    StepIndex, LoopIteration int
    CurrentStep, BranchName, Branch, WorktreePath string
    Worktree bool
    ExitCode *int
    ErrorMessage, Context string
    BlockedBy []int64
    Images []string
    CreatedAt, StartedAt, CompletedAt, UpdatedAt time.Time
}

type AgentInfo struct {
    ID, TaskID string
    State AgentState  // pending|starting|running|waiting_for_input|completed|failed|stopped
    PID int
    CurrentStep string
    StepIndex int
    StartedAt time.Time
    EndedAt *time.Time
    Error string
}
```
