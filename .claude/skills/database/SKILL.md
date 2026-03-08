---
name: database
description: >
  Sortie's SQLite persistence layer: schema, migrations, task/project queries, and
  dependency management. Use when editing files in internal/db/, working on schema
  migrations, task queries, project persistence, or dependency blocking logic.
---

# Database & Persistence

SQLite with WAL mode, single writer (`MaxOpenConns=1`), foreign keys enabled. Schema versioned with progressive migrations (currently v10).

## Schema

Read `internal/db/schema.sql` for the canonical table definitions. Core tables: `projects`, `tasks`, `task_dependencies`. Migrations are in the `migrations` slice in `db.go` — append with the next version number; auto-applied on startup via `ensureSchema()`.

## Key Query Patterns

### GetClaimableTasks()
Find pending tasks not blocked by incomplete dependencies, ordered by priority (urgent > low) then creation date (FIFO within priority).

### ClaimTask()
Atomically transition pending -> running with `started_at`. Returns error if not pending (prevents duplicate agents).

### Reset Operations
- `ResetTaskForRetry(id)` — reset to pending, clear step/error/timing
- `ResetTaskForRetryFromStep(id, stepIdx, stepName)` — partial retry
- `ResetTaskForContinue(id)` — reset to pending, preserve step progress

## Patterns

- Parameterized queries only (`?` placeholders), never string interpolation
- Images stored as JSON array: `json.Marshal`/`json.Unmarshal`
- Nullable fields use `sql.NullString`, `sql.NullInt64`, `sql.NullTime`
- `blocked_by` computed from `task_dependencies` table, not stored directly
- Test with `NewTestDB()` using in-memory SQLite (`":memory:"`)
- New columns: add migration, handle NULL defaults for existing rows
