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

`StartWithPrompt(ctx, prompt)` spawns Claude CLI:

```
claude [config.Args()] --verbose --output-format stream-json -p <prompt>
```

- Stderr -> `.claude-output.log` (raw NDJSON)
- Stdout piped to `StreamParser` -> `OutputFunc` callback
- `Stop()`: SIGTERM -> 5s grace -> SIGKILL
- `SetEnv(key, value)` before starting for env vars

### StreamParser

Parses Claude's `stream-json` NDJSON format. Event types: `system`, `assistant`, `user`, `result`. Content blocks: `text`, `tool_use`, `thinking`, `tool_result`. Formats output with `[HH:MM:SS]` timestamps, extracts tool summaries (command, file_path, pattern).

## Agent Management (internal/agent/)

### Agent States

`pending` -> `starting` -> `running` -> `completed`/`failed`/`stopped`

### Manager

- Enforces `maxConcurrent` limit; excess agents queued
- `StartAgent(id, task, workDir, runFn)`: enqueues if at capacity
- `StopAgent(id)`: cancels context, processes queue
- `OnStateChange` callback fires outside mutex (deadlock prevention)
- `Shutdown()`: cancels all contexts with 500ms polling grace

### RingBuffer

Fixed-size circular buffer for streaming output: `Append(lines)`, `GetFrom(fromLine)`, `GetAll()`. Supports incremental consumption for live TUI updates.

## Data Flow

Claude stdout -> StreamParser -> OutputFunc -> Agent.outputBuffer -> TUI via `get_output`

## Patterns

- State transitions go through Manager methods, not direct field assignment
- `OnStateChange` is the primary integration point with daemon
- Check `HasExited()` before reading `ExitCode()`
- Queue processing in `processQueue()` after agent completes/stops
- Env vars (`SORTIE_TASK_ID`, etc.) set via `process.SetEnv()` before start
