# Task Lifecycle & State Transitions

TRIGGER when: editing files in `internal/task/`, working on task status transitions, priority management, or task metadata handling. Also relevant when modifying any code that changes task status.

## Task Struct (internal/task/task.go)

```go
type Task struct {
    ID            int64
    ProjectID     int64
    Title         string
    Description   string
    Slug          string
    Workflow      string          // Workflow template name
    Status        Status
    Priority      Priority
    StepIndex     int             // Current position in workflow
    CurrentStep   string          // Step name
    LoopIteration int
    BranchName    string          // User-provided branch template
    Branch        string          // Resolved branch name
    Worktree      bool            // Whether to use git worktree isolation
    WorktreePath  string
    ExitCode      *int
    ErrorMessage  string
    Context       string          // Summarizer output / arbitrary data
    BlockedBy     []int64         // Task IDs that block this task
    Images        []string        // Attached image paths
    CreatedAt     time.Time
    StartedAt     *time.Time
    CompletedAt   *time.Time
    UpdatedAt     time.Time
}
```

## Status State Machine

```
pending ──> init ──> running ──┬──> awaiting-approval ──> running (resumed)
                               ├──> tmux ──> finalizing ──> summarizing ──> completed
                               ├──> artifact-missing ──> running (continued)
                               ├──> merge-blocked ──> completed
                               ├──> completed
                               └──> failed
```

**Terminal states:** `completed`, `failed`
**Active states:** `running`, `init`, `finalizing`, `summarizing`, `merge-blocked`
**Waiting states:** `awaiting-approval`, `tmux`, `artifact-missing`
**Initial:** `pending`

## Priority System

| Priority | Value | Sort Order |
|----------|-------|------------|
| urgent   | 4     | First |
| high     | 3     | Second |
| medium   | 2     | Third (default) |
| low      | 1     | Last |

`GetClaimableTasks()` orders by priority descending, then `created_at` ascending (FIFO within same priority).

## Title Handling

- `SanitizeTitle()`: first line only, collapse whitespace, remove control chars, truncate to 80 chars (avoid mid-word cuts)
- `Slugify()`: lowercase, non-alphanumeric -> hyphens, trim, 40-char limit

## Dependency Blocking

Tasks can be blocked by other tasks via `task_dependencies` table:
- A task with non-empty `BlockedBy` where any dependency is not `completed` won't be claimed
- Dependencies checked at query time in `GetClaimableTasks()`

## Patterns to Follow

- Status transitions go through `db.UpdateTaskStatus()`, not direct field assignment
- Always use `SanitizeTitle()` when accepting user input for titles
- `Slugify()` is used for branch name generation
- The `Worktree` field defaults to `true`; when `false`, the task runs in the project root
- `Context` field stores the summarizer output after workflow completion
- `Images` is a string slice stored as JSON in the database
