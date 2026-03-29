# internal/agent — Agent State Management

Agent state machine and concurrent agent manager. Load `/claude-process` skill before making substantial changes.

## Critical Invariants

- **State transitions go through Manager methods, not direct field assignment** — ensures callbacks fire and state stays consistent
- **Manager enforces `maxConcurrent` limit** — excess agents queued, not dropped
- **OnStateChange callback fires outside mutex** — prevents deadlocks with daemon broadcaster
