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

Read `internal/db/schema.sql` for the canonical table definitions. Core tables: `projects`, `tasks`, `task_dependencies`, `task_steps`. Migrations use `if version < N` blocks in `db.go:migrate()` — append the next version check; auto-applied on startup. Fresh databases apply the embedded `schema.sql` directly as version 10.

### `task_steps` Table

Stores per-step execution results for each task run. Populated by the workflow engine after each step completes.

| Column | Type | Description |
|--------|------|-------------|
| `id` | INTEGER PK | Auto-increment |
| `task_id` | INTEGER FK | References `tasks.id` |
| `step_name` | TEXT | Step identifier (matches workflow step `name`) |
| `status` | TEXT | Step execution status |
| `context` | TEXT | Step result captured from Claude's NDJSON `result` event |
| `exit_code` | INTEGER | Claude process exit code |
| `started_at` | DATETIME | When step execution began |
| `completed_at` | DATETIME | When step execution finished |

Step contexts are fetched via daemon RPC for TUI display. Template access: `{{steps.<step_name>.context}}` (or backward-compat `{{artifacts.<step_name>}}`).

## Project Operations

```go
type Project struct {
    ID              int64
    Path            string
    Name            string
    DefaultPriority task.Priority
    CreatedAt       time.Time
}

GetOrCreateProject(projectPath string) (*Project, error)  // Upsert by path
GetProjectByPath(path string) (*Project, error)
GetProject(id int64) (*Project, error)
GetProjectsByName(name string) ([]*Project, error)
ListProjects() ([]*Project, error)
```

## Task Query Patterns

### Status-Filtered Queries
- `GetPendingTasks()` / `GetRunningTasks()` — filter by status
- `GetClaimableTasks()` — pending tasks not blocked by incomplete dependencies, ordered by priority desc then created_at asc
- `GetAllTasks()` — all tasks regardless of status
- `GetTasksByProject(projectID int64)` — tasks for a specific project
- `GetTasksByProjectName(name string)` — tasks by project name

### ClaimTask(id)
Atomically transition pending -> running with `started_at`. Returns `(bool, error)` — false if not pending.

### Field Update Functions
```go
UpdateTaskStatus(id int64, status task.Status) error
UpdateTaskWorktreePath(id int64, worktreePath string) error
ClearWorktreePath(id int64) error
UpdateTaskStep(id int64, stepIndex int, currentStep string) error
UpdateTaskExitCode(id int64, exitCode int, errorMessage string) error
UpdateTaskError(id int64, errMsg string) error
UpdateTaskPriority(id int64, priority task.Priority) error
UpdateTaskContext(id int64, taskContext string) error
UpdateTaskTitle(id int64, title string) error
UpdateTaskDescription(id int64, description string) error
FinalizeTaskIdentity(id int64, title, slug, branch string) error
UpdateTaskLoopIteration(id int64, iteration int) error
```

### Reset Operations
- `ResetTaskForRetry(id int64)` — reset to pending, clear step/error/timing
- `ResetTaskForRetryFromStep(id int64)` — reset to pending, clear current_step/error but **keep step_index**
- `ResetTaskForContinue(id int64, workflow, prompt string)` — reset to pending, update workflow and context prompt
- `DeleteTask(id int64)` — hard delete

## Patterns

- Parameterized queries only (`?` placeholders), never string interpolation
- Images stored as JSON array: `json.Marshal`/`json.Unmarshal`
- Nullable fields use `sql.NullString`, `sql.NullInt64`, `sql.NullTime`
- `blocked_by` computed from `task_dependencies` table, not stored directly
- Test with `NewTestDB()` using in-memory SQLite (`":memory:"`)
- New columns: add migration (`if version < N`), handle NULL defaults for existing rows
