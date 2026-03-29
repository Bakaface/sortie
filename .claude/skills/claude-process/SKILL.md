---
name: claude-process
description: >
  Claude Code process spawning, NDJSON stream parsing, output handling, agent state
  machine, and concurrency control. Use when editing files in internal/claude/ or
  internal/agent/, working on process lifecycle, stream parsing, agent state
  transitions, or the concurrent agent manager.
---

# Claude Process & Agent Management

## Process Spawning (internal/claude/)

`Process` wraps a Claude CLI subprocess.

```go
type Process struct {
    TaskID, WorkDir, OutputFile string
    OutputFunc func(lines []string)
    // internal: cmd, env, parser, outputLines, exitCode, exited
}

NewProcess(taskID, workDir string, cfg *config.ClaudeConfig) *Process
SetEnv(env map[string]string)          // Set env vars before starting
StartWithPrompt(prompt string, systemPrompt ...string) error  // Spawns claude CLI
Stop() error                           // SIGTERM -> 5s grace -> SIGKILL
IsRunning() bool
HasExited() bool
ExitCode() int
IsSuccess() bool
PID() int
ResultText() string                    // Final text output (after exit)
CaptureOutput(maxLines int) ([]string, error)
```

### StreamParser

Parses Claude's `stream-json` NDJSON format. Event types: `system`, `assistant`, `user`, `result`. Content blocks: `text`, `tool_use`, `thinking`, `tool_result`. Formats output with `[HH:MM:SS]` timestamps, extracts tool summaries (command, file_path, pattern, description).

## Agent Management (internal/agent/)

### Agent States

```
pending -> starting -> running -+-> completed
                                +-> failed
                                +-> stopped
              \-> waiting_for_input (from running)
```

- `State.IsTerminal()` — completed, failed, stopped
- `State.IsActive()` — starting, running, waiting_for_input

### Agent Struct

```go
type Agent struct {
    ID          string
    Task        *task.Task
    WorkDir     string
    State       State
    PID         int          // Process ID of claude CLI
    StartedAt   time.Time
    EndedAt     time.Time
    Error       string
    CurrentStep string
    StepIndex   int
    // internal: outputBuffer *RingBuffer
}

New(t *task.Task, bufferSize int) *Agent
Duration() time.Duration          // EndedAt - StartedAt (or now - StartedAt)
GetState() / SetState(State)
SetError(string) / SetPID(int) / GetPID() int / SetWorkDir(string)
AppendOutput([]string) / GetOutput(fromLine int) / GetAllOutput()
```

### Manager

```go
var (
    ErrTaskAlreadyTracked = errors.New("task already tracked")
    ErrAgentNotFound      = errors.New("agent not found")
    ErrNoWorkDir          = errors.New("task has no workdir")
)

NewManager(maxConcurrent, bufferSize int) *Manager
SetStateChangeCallback(cb StateChangeCallback)
StartAgent(t *task.Task, workDir string, runner func(ctx context.Context) error) (*Agent, error)
StopAgent(agentID string) error
GetAgent(agentID string) (*Agent, bool)
GetAgentByTaskID(taskID int64) (*Agent, bool)
ListAgents() []*Agent
IsTaskKnown(taskID int64) bool
Shutdown(gracePeriod time.Duration)
GetOutput(agentID string, fromLine int) ([]string, int, error)
```

- Enforces `maxConcurrent` limit; excess agents queued
- `OnStateChange` callback fires outside mutex (deadlock prevention)
- Queue processing in `processQueue()` after agent completes/stops

### RingBuffer

Fixed-size circular buffer for streaming output: `Append(lines)`, `GetFrom(fromLine)`, `GetAll()`. Supports incremental consumption for live TUI updates.

## Data Flow

Claude stdout -> StreamParser -> OutputFunc -> Agent.outputBuffer -> TUI via `get_output`

## Patterns

- State transitions go through Manager methods, not direct field assignment
- `OnStateChange` is the primary integration point with daemon
- Check `HasExited()` before reading `ExitCode()`
- Env vars (`SORTIE_TASK_ID`, etc.) set via `process.SetEnv()` before start
- `buildEnv()` filters out `CLAUDECODE=` from the child environment to prevent "cannot launch inside another session" errors
