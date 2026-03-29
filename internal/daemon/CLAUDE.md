# internal/daemon — Background Daemon

Unix socket server, request handlers, task polling, agent lifecycle. Load `/daemon` skill before making substantial changes.

## Critical Invariants

- **Project context is lazy-loaded and cached** — use `getProjectContext()`, never re-load per-operation
- **Merge mutexes are per-repo, not per-task** — `mergeMus` keyed by `repoRoot` serializes concurrent merges in the same repo
- **Broadcasting happens outside locks** — agent state change callbacks fire after releasing mutexes to prevent deadlocks
