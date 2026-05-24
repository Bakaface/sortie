# internal/agent — Agent State Management

Agent state machine and concurrent agent manager. Load `/claude-process` skill before making
substantial changes (also covers `internal/claude/`).

## Critical Invariants

- **Manager enforces `maxConcurrent` limit** — excess agents queued, not dropped.

See `internal/claude/CLAUDE.md` for the cross-package invariants this package shares
(state-transition routing through Manager methods, OnStateChange callback firing outside the
mutex).
