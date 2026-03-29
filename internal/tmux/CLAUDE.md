# internal/tmux — Tmux Session Management

Session creation, lifecycle, pane capture, activity monitoring. Load `/tmux` skill before making substantial changes.

## Critical Invariants

- **Session names use sanitized project name (dots → underscores)** — must match tmux's own character replacement rules
- **`SetupCommandControlsAgent()` check required** — determines whether daemon or user manages agent startup; skipping causes double-start or no-start
- **Must call `IsAvailable()` before any tmux operations** — binary may not exist; skipping causes cryptic failures
