# Database & Persistence

TRIGGER when: editing files in `internal/db/`, working on SQLite schema, migrations, task queries, project persistence, or dependency management.

## Overview

SQLite database with WAL mode, single writer (`MaxOpenConns=1`), foreign keys enabled. Schema version tracking with progressive migrations (currently v10).

## Schema

**Core tables:**
- `projects` - Project metadata (id, path UNIQUE, name, default_priority, created_at)
- `tasks` - Task records with full lifecycle state
- `task_dependencies` - Many-to-many blocking relationships (task_id, depends_on_task_id)

**Task columns:** id, project_id (FK), title, description, slug, workflow, status, priority, step_index, current_step, loop_iteration, branch_name, branch, worktree (bool), worktree_path, exit_code, error_message, context, images (JSON array), blocked_by (computed), created_at, started_at, completed_at, updated_at

## Migration Pattern

Migrations are numbered v1-v10 in the `migrations` slice within `db.go`. Each migration runs in `ensureSchema()`:

```go
var migrations = []struct {
    version int
    sql     string
}{
    {1, `CREATE TABLE IF NOT EXISTS tasks (...)`},
    {2, `ALTER TABLE tasks ADD COLUMN workflow TEXT ...`},
    // ...
}
```

To add a new migration: append to the slice with the next version number. The system auto-applies unapplied migrations on startup.

## Key Query Patterns

### GetClaimableTasks()
Complex query: finds pending tasks NOT blocked by incomplete dependencies, ordered by priority (urgent > high > medium > low) then creation date (FIFO within priority).

```sql
WHERE t.status = 'pending'
AND NOT EXISTS (
    SELECT 1 FROM task_dependencies td
    JOIN tasks dep ON dep.id = td.depends_on_task_id
    WHERE td.task_id = t.id AND dep.status NOT IN ('completed')
)
ORDER BY CASE priority ... END, created_at ASC
```

### ClaimTask()
Atomically transitions pending -> running with `started_at` timestamp. Returns error if task not in pending state (prevents duplicate agents).

### Status Tracking
- `UpdateTaskStatus(id, status)` - General status transition
- `UpdateTaskStep(id, stepIndex, stepName)` - Workflow progress
- `UpdateTaskLoopIteration(id, iteration)` - Loop counter
- `UpdateTaskContext(id, context)` - Stores summarizer output
- `UpdateTaskExitCode(id, code)` - Claude process exit code

### Reset Operations
- `ResetTaskForRetry(id)` - Resets to pending, clears step/error/timing
- `ResetTaskForRetryFromStep(id, stepIdx, stepName)` - Partial retry from specific step
- `ResetTaskForContinue(id)` - Resets to pending, preserves step progress

## Patterns to Follow

- Always use parameterized queries (`?` placeholders), never string interpolation
- Images stored as JSON array: `json.Marshal(task.Images)` on write, `json.Unmarshal` on read
- Nullable fields use `sql.NullString`, `sql.NullInt64`, `sql.NullTime`
- `blocked_by` is computed from `task_dependencies` table, not stored directly
- Test with `NewTestDB()` which uses in-memory SQLite (`":memory:"`)
- When adding columns: create a new migration, handle NULL defaults for existing rows
