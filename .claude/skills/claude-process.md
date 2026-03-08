# Claude Code Process & Agent Management

TRIGGER when: editing files in `internal/claude/` or `internal/agent/`, working on Claude process spawning, output parsing, stream handling, agent state machines, or concurrency control.

## Claude Process (internal/claude/)

### Process Spawning

`StartWithPrompt(ctx, prompt)` spawns Claude CLI in one-shot mode:

```
claude [config.Args()] --verbose --output-format stream-json -p <prompt>
```

- Stderr -> `.claude-output.log` (raw NDJSON + diagnostics)
- Stdout piped real-time to `StreamParser` -> `OutputFunc` callback
- Goroutine handles concurrent streaming and exit waiting

### Process Lifecycle

- `IsRunning()`, `HasExited()`, `IsSuccess()` (exitCode == 0)
- `Stop()`: SIGTERM (graceful) -> 5s grace period -> SIGKILL
- `ExitCode()`: returns exit code or -1 if still running
- `CaptureOutput(maxLines)`: reads `.claude-output.log` (1MB buffer for large NDJSON)
- `SetEnv(key, value)`: set environment variables before starting

### StreamParser (NDJSON)

Parses Claude's `stream-json` format (one JSON object per line):

**Event types:** `system` (init), `assistant` (output), `user` (tool results), `result` (final summary)

**Content blocks:** `text`, `tool_use` (name + JSON input), `thinking`, `tool_result`

**Output formatting:**
- Timestamps `[HH:MM:SS]` prepended to parsed content
- Tool summaries extracted: command, file_path, pattern
- BOM trimming, truncation, last message ID tracking

## Agent Management (internal/agent/)

### Agent Struct

```go
type Agent struct {
    ID          string          // Often matches task ID
    Task        *task.Task
    WorkDir     string
    State       AgentState
    PID         int             // Claude CLI process PID
    StartedAt   time.Time
    EndedAt     *time.Time
    Error       error
    CurrentStep string
    StepIndex   int
    outputBuffer *RingBuffer    // Streaming log lines
}
```

**States:** `pending` -> `starting` -> `running` -> `completed`/`failed`/`stopped`

### Manager

Controls concurrent agent execution:

- Enforces `maxConcurrent` limit; excess agents queued
- `StartAgent(id, task, workDir, runFn)`: enqueues if at capacity, otherwise runs
- `StopAgent(id)`: cancels context, transitions to stopped, processes queue
- `GetClaimableTask()`: selects next pending agent when a slot opens
- `OnStateChange` callback fires outside mutex to avoid deadlocks
- `Shutdown()`: cancels all contexts with 500ms polling grace period

### RingBuffer

Fixed-size circular buffer for streaming output:
- `Append(lines)`, `GetFrom(fromLine)` -> (lines, total), `GetAll()`
- Supports incremental consumption for live TUI updates

## Patterns to Follow

- Agent state transitions must go through the Manager's methods (not set directly)
- The `OnStateChange` callback is the primary integration point with the daemon
- Process output flows: Claude stdout -> StreamParser -> OutputFunc -> Agent.outputBuffer -> TUI via `get_output`
- Always check `HasExited()` before reading `ExitCode()`
- The Manager's queue processing happens in `processQueue()` after any agent completes/stops
- Environment variables (`SORTIE_TASK_ID`, etc.) are set via `process.SetEnv()` before `StartWithPrompt()`
