# TUI Architecture

TRIGGER when: editing files in `internal/tui/`, working on terminal UI components, BubbleTea models, Lip Gloss styling, keybindings, or task list/detail views.

## Component Hierarchy

Single root `Model` (app.go) with nested sub-views:

```
Model (app.go)
  listView       (list.go)         Task table with search/filtering
  detailView     (detail.go)       Live logs with follow mode
  taskInfoView   (task_info.go)    Task metadata + workflow progress
  promptView     (prompt.go)       New task creation form
  artifactViewState (artifact_view.go)  Artifact file viewer
```

View switching via `view` enum: `viewList`, `viewDetail`, `viewTaskInfo`, `viewPrompt`, `viewArtifact`.

## File Organization

| File | Responsibility |
|------|----------------|
| `app.go` | Root Model, Init/Update/View routing, message handling |
| `keys.go` | KeyMap structs for each view mode |
| `styles.go` | Pre-computed `lipgloss.Style` maps, status icons, adaptive colors |
| `list.go` | Task table with custom row rendering, scroll, search |
| `detail.go` | Viewport-based log viewer with follow mode, ANSI stripping |
| `prompt.go` | Dual-field form (textarea + textinput), image detection |
| `task_info.go` | Metadata display + workflow step progress visualization |
| `artifact_view.go` | Artifact content viewer |
| `actions.go` | Async `tea.Cmd` functions (API calls, editor spawning, tmux) |
| `command.go` | Vim-style `:` command parsing (goto, toggles) |
| `search.go` | Forward/backward search with match highlighting |
| `update_list.go` | List view key handlers |
| `update_detail.go` | Detail view key handlers |
| `update_task_info.go` | Task info key handlers |
| `update_prompt.go` | Prompt view key handlers |
| `update_artifact.go` | Artifact view key handlers |

## Patterns to Follow

### Keybindings
- Define keys in `keys.go` as `keyMap` structs with `key.Binding` fields
- Multiple KeyMaps exist: `keyMap` (list), `detailKeyMap`, `detailFollowKeyMap`, `detailNormalKeyMap`, `taskInfoKeyMap`
- Vim-style mnemonics: `gg`/`G` (top/bottom), `dd` (delete), `/`/`?` (search), `:` (command)
- Multi-key sequences tracked via `pending*` booleans on the Model

### State Management
- Selection dialogs use dedicated state flags: `selecting*` + `*Cursor` + `*PendingG`
- Confirmation dialogs: `confirmAction` + `confirmTaskID` for y/n prompts
- Always reset pending states when switching views

### Message Flow
- User key -> handler creates `tea.Cmd` -> goroutine runs -> returns `tea.Msg` -> `Update()` processes
- Custom messages defined as types implementing `tea.Msg` (e.g., `taskUpdateMsg`, `outputLoadedMsg`)
- Async API calls go in `actions.go`, return typed messages

### Styling
- Pre-computed style maps in `styles.go`: `stateStyles`, `priorityStyles`
- Use adaptive colors (`lipgloss.AdaptiveColor`) for light/dark terminal support
- Status icons: `StatusRunning="●"`, `StatusPending="○"`, `StatusCompleted="✓"`, `StatusFailed="✗"`, `StatusAwaiting="◷"`, `StatusTmux="▣"`
- Helper functions: `stateStyle()`, `priorityStyle()`, `priorityBadge()`, `statusIconFor()`

### View Rendering
- List view: custom row rendering via `renderTask()`, NOT table.Model's built-in render
- Detail view: wraps `viewport.Model`, strips ANSI via regex, tracks `contentDirty` to avoid re-wrapping
- Task info: renders step progress with icons (`○`/`●`/`✓`/`✗`) and attributes (`[human]`, `[artifact]`, `[loop]`)

## Interaction with Other Packages

- **daemon** via `client.Client`: `ListTasksFiltered()`, `CreateTask()`, `StopTask()`, `RetryTask()`, `ContinueTask()`, `FinalizeTask()`, `DeleteTask()`, `GetLogs()`, `Subscribe()`
- **config**: `ListWorkflowNames()`, `GetWorkflow()`, `ListPredefinedTaskNames()`, `GetPredefinedTask()`
- **tmux**: `ListSessions()`, `AttachCommand()`, `SwitchClientCommand()`
- **workflow**: `ArtifactsDir()`, `ReadArtifact()`, `ProjectLogsDir()`

## Common Pitfalls

- Do NOT use `table.Model`'s built-in rendering for the task list; it uses custom `renderTask()` per row
- Detail view performance: check `contentDirty` before re-wrapping content
- Always handle `tea.WindowSizeMsg` in every view to recalculate dimensions
- Multi-key sequences (e.g., `gg`, `dd`) must reset on view switch or unrelated key press
- The `promptView` auto-detects image paths from textarea content; preserve this behavior when modifying the prompt form
