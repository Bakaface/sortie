# internal/daemon — Background Daemon

Unix socket server, request handlers, task polling, agent lifecycle. Load `/daemon` skill before making substantial changes.

## Critical Invariants

- **Project context is lazy-loaded and cached** — use `getProjectContext()`, never re-load per-operation
- **Per-repo merge serialization is owned by `internal/merge`** — the daemon hands out `*merge.Lock` instances via `s.mergeLocks` (a `*merge.Locks` registry) to each Engine; handlers must never reach for raw mutexes
- **Broadcasting happens outside locks** — agent state change callbacks fire after releasing mutexes to prevent deadlocks

## Design notes

- **Why the project-context cache is a private method, not a `ProjectContextStore` module.**
  Audited 2026-05 (sortie#62). Every caller in `internal/daemon/` (21 sites) routes through
  `getProjectContext()`; the only `config.LoadForProject` call site is inside that method.
  The two direct reads of `s.projects` are intentional and would survive any extraction:
  `broadcast.go`'s `taskToInfo()` peeks the map without triggering a load (a serializer
  shouldn't issue DB queries), and `shutdown()` iterates the live cache to kill tmux
  sessions. There is no second adapter today — tests prime the real cache via
  `getProjectContext()` rather than using a fake — so promoting the seam to an exported
  module + interface would be one-adapter indirection without buying leverage. The
  natural cache key is `projectID` (carried on every `*task.Task`), not `repoRoot`, so
  the conventional `Store.Get(repoRoot)` shape would push an extra DB lookup into
  callers. Invalidation lives in the loader via `.sortie.yml` mod-time check; no handler
  needs an `Invalidate()` API. Concurrent first-time loads can race, but the duplicated
  work is a YAML parse and the loser's `*projectContext` is GC'd — `mergeMus` are keyed
  by `repoRoot` independently, so merge serialization survives the race. Revisit only if
  a second adapter (in-memory fake, second backing store, cross-process cache) appears.
