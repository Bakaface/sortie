# internal/tui — Terminal UI

BubbleTea terminal UI for monitoring and task management. Load `/tui` skill before making substantial changes.

## Critical Invariants

- **`lipgloss.Width()` must match expected frame width for every line** — verify rendering programmatically, never by reasoning alone (see root CLAUDE.md "Verifying Non-Interactive Output")
- **Chord registry in `chords.go`** — new two-key sequences added by appending to `chordRegistry[view]` in `init()`; don't add standalone handlers
- **Worktree toggle state (`alt+w`) persists in DB and session** — affects branch input visibility and `task.Worktree` flag
