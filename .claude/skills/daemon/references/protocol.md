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
type AgentInfo struct {
    ID          string     `json:"id"`
    TaskID      int64      `json:"task_id"`
    Description string     `json:"description"`
    WorkDir     string     `json:"work_dir"`
    State       AgentState `json:"state"`   // pending|starting|running|waiting_for_input|completed|failed|stopped
    StartedAt   time.Time  `json:"started_at"`
    Error       string     `json:"error,omitempty"`
}

type TaskInfo struct {
    ID            int64      `json:"id"`
    ProjectID     int64      `json:"project_id"`
    ProjectName   string     `json:"project_name,omitempty"`
    ProjectPath   string     `json:"project_path,omitempty"`
    Title         string     `json:"title"`
    Description   string     `json:"description"`
    Slug          string     `json:"slug"`
    Workflow      string     `json:"workflow,omitempty"`
    Status        string     `json:"status"`
    Priority      string     `json:"priority"`
    StepIndex     int        `json:"step_index"`
    CurrentStep   string     `json:"current_step"`
    LoopIteration int        `json:"loop_iteration,omitempty"`
    BranchName    string     `json:"branch_name,omitempty"`
    Branch        string     `json:"branch"`
    Worktree      bool       `json:"worktree"`
    WorktreePath  string     `json:"worktree_path,omitempty"`
    ErrorMessage  string     `json:"error_message,omitempty"`
    Context       string     `json:"context,omitempty"`
    Images        []string   `json:"images,omitempty"`
    BlockedBy     []int64    `json:"blocked_by,omitempty"`
    CreatedAt     time.Time  `json:"created_at"`
    StartedAt     *time.Time `json:"started_at,omitempty"`
    CompletedAt   *time.Time `json:"completed_at,omitempty"`
}
```

Note: `AgentInfo` in the protocol is a simplified projection. Internal `agent.Agent` has additional fields (`PID`, `CurrentStep`, `StepIndex`, `Duration()`) not exposed over the wire.
