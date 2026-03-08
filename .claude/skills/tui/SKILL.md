---
name: tui
description: >
  Sortie's BubbleTea terminal UI architecture, component patterns, and conventions.
  Use when editing files in internal/tui/, working on terminal UI components, BubbleTea
  models, Lip Gloss styling, keybindings, views, or task list/detail rendering.
---

# TUI Architecture

## Component Hierarchy

Single root `Model` (app.go) with nested sub-views, switched via `view` enum (`viewList`, `viewDetail`, `viewTaskInfo`, `viewPrompt`, `viewArtifact`):

```
Model (app.go)
  listView       (list.go)            Task table with search/filtering
  detailView     (detail.go)          Live logs with follow mode
  taskInfoView   (task_info.go)       Task metadata + workflow progress
  promptView     (prompt.go)          New task creation form
  artifactViewState (artifact_view.go) Artifact file viewer
```

## File Map

### Core

| File | Responsibility |
|------|----------------|
| `app.go` | Root Model, Init/Update/View routing, global state, message handling |
| `keys.go` | KeyMap structs for each view mode |
| `styles.go` | Pre-computed `lipgloss.Style` maps, status icons, adaptive colors |

### Views

| File | Responsibility |
|------|----------------|
| `list.go` | Task table with custom row rendering, scroll offset, search matching |
| `detail.go` | Viewport-based log viewer with follow mode, ANSI stripping |
| `prompt.go` | Dual-field form (textarea + textinput), worktree toggle, image detection |
| `task_info.go` | Metadata display + workflow step progress (icons: `○`/`●`/`✓`/`✗`) |
| `artifact_view.go` | Artifact content viewer |

### Update Handlers

| File | Responsibility |
|------|----------------|
| `update_list.go` | List view: navigation, task actions, selections, search, commands |
| `update_detail.go` | Detail view: scroll, follow toggle, back |
| `update_task_info.go` | Task info: scroll, copy-to-clipboard (yd/yc), edit fields, artifact selection |
| `update_prompt.go` | Prompt view: submit, tab switching, editor open, worktree toggle |
| `update_artifact.go` | Artifact selection/viewing: list navigation, open/edit |

### Actions & Utilities

| File | Responsibility |
|------|----------------|
| `actions.go` | Async `tea.Cmd` functions: API calls, editor spawning, tmux attachment, log loading |
| `command.go` | Vim-style `:` command parsing: goto line, toggle line numbers, toggle finished, clear search |
| `search.go` | Forward/backward search with match highlighting via `performSearch()`, `nextMatch()`, `previousMatch()` |

## Custom Message Types

Task updates: `taskUpdateMsg`, `taskCreatedMsg`, `taskDeletedMsg`, `tasksLoadedMsg`
Output: `outputLoadedMsg`, `artifactLoadedMsg`
Editor: `editorFinishedMsg`, `editorPromptFinishedMsg`, `editorFieldFinishedMsg`
System: `clientConnectedMsg`, `tmuxDetachedMsg`, `tickMsg`, `errorMsg`

## Key Patterns

### Keybindings
- Define in `keys.go` as `keyMap` structs with `key.Binding` fields
- Multiple KeyMaps: `keyMap` (list), `detailKeyMap`, `detailFollowKeyMap`, `detailNormalKeyMap`, `taskInfoKeyMap`
- Vim-style mnemonics: `gg`/`G`, `dd`, `/`/`?`, `:`
- Multi-key sequences tracked via `pending*` booleans — reset on view switch

### Message Flow
User key -> handler returns `tea.Cmd` -> goroutine -> `tea.Msg` -> `Update()` processes. Async API calls in `actions.go`, return typed messages.

### Selection/Confirmation Dialogs
Dedicated state flags per dialog: `selecting*` + `*Cursor` + `*PendingG`. Confirmation: `confirmAction` + `confirmTaskID` for y/n prompts.

### Styling
- Pre-computed style maps in `styles.go`: `stateStyles`, `priorityStyles`
- Use `lipgloss.AdaptiveColor` for light/dark terminal support
- Status icons: `●` running, `○` pending, `✓` completed, `✗` failed, `◷` awaiting, `▣` tmux
- Helpers: `stateStyle()`, `priorityStyle()`, `priorityBadge()`, `statusIconFor()`

## Pitfalls

- List view uses custom `renderTask()`, NOT `table.Model`'s built-in rendering
- Detail view: check `contentDirty` before re-wrapping content (performance)
- Handle `tea.WindowSizeMsg` in every view to recalculate dimensions
- `promptView` auto-detects image paths from textarea — preserve this behavior

## Adjacent Packages

- **daemon** via `client.Client`: task CRUD, subscriptions, log streaming
- **config**: `ListWorkflowNames()`, `GetWorkflow()`, `ListPredefinedTaskNames()`
- **tmux**: `ListSessions()`, `AttachCommand()`, `SwitchClientCommand()`
- **workflow**: `ArtifactsDir()`, `ReadArtifact()`, `ProjectLogsDir()`
