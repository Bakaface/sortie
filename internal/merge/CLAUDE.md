# internal/merge — Per-Repo Merge Serialization

The deep module that owns the "all merges into the same repo must serialize, and on any failure
the repo's working tree must be cleaned" invariant. Consolidates logic that previously lived
across the daemon, the workflow engine, and `internal/git`.

## Critical Invariants

- **All merges go through `Coordinator.Finalize()`** — the public surface is intentionally tiny.
  Workflow engine calls `e.coord.Finalize(ctx, t, baseBranch, action, logFn)` and nothing else.
- **Per-repo serialization via a shared `*Lock`** — the daemon owns a `*Locks` registry
  (`Locks.For(repoRoot)` returns a stable per-repo Lock). Multiple Coordinators (e.g., after
  config reload) that point at the same repo share the same Lock so serialization survives.
- **Cleanup on any failure** — Coordinator must call `git.CleanRepoState` in a `defer` so a
  failed merge never leaves the repo with conflict markers, partial commits, or a dirty index.
- **Conflict resolver is engine-supplied** — `ConflictResolver` is wired by the workflow engine
  (typically spawns a Claude agent to fix conflicts and stage). A nil resolver means conflicts
  abort the merge — never silently succeed.
- **Non-worktree mode short-circuits** — when `task.Worktree == false`, merge is replaced by
  `commit()` (or no-op if `OnComplete` is `none`).

## File Map

| File | Purpose |
|------|---------|
| `coordinator.go` | `Coordinator`, `Config`, `Lock`, `Locks`, the `Finalize` pipeline, conflict-retry, `waitForCleanTarget`, commit-recorder hooks |
| `coordinator_test.go` | Unit tests for the pipeline + locking |

## Patterns

- `Finalize(ctx, t, baseBranch, action, logFn)` is the only entrypoint callers should use. The
  `action` ("commit"/"merge"/"none") is resolved per-task by the caller (workflow-level override
  falling back to the project-level `on_complete`), not stored on the Coordinator.
- `NewCoordinator(repoRoot, lock, cfg, resolveConflicts, setStatus, recordCommit)` zero-fills
  sensible defaults (`MaxAttempts = 3`, `BlockedPollInterval = 10s`, empty Lock if nil).
- Status writes happen **outside** the per-repo lock via the `StatusSetter` callback so they
  can take DB mutexes without inversion.
