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
| `prompt.go` | Dual-field form (textarea + textinput), worktree toggle (`alt+w`), image detection |
| `task_info.go` | Metadata display + workflow step progress (icons: `○`/`●`/`✓`/`✗`) |
| `artifact_view.go` | Step context viewer (fetches from daemon RPC, not disk) |

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
| `command.go` | Vim-style `:` command parsing with declarative option registry (`boolOption`/`intOption` slices → `matchSetOption`/`execSetOption`). Add new `:set` options by appending to `boolOptions` or `intOptions`. Also: goto line, RunTask, noh. |
| `selector.go` | Generic list-pick dialog: `selector` struct with `HandleKey()`/`View()`, vim navigation, number-key quick select. Used for workflow, task, init, priority, artifact selection. |
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
Generic `selector` struct (`selector.go`) handles all list-pick dialogs (workflow, task, init, priority, artifact). Add new selectors by appending a `selectorKind` const and a case in `handleSelectorChoice()`. Branch selection stays separate (has fuzzy filtering). Confirmation: `confirmAction` + `confirmTaskID` for y/n prompts.

### Styling
- Pre-computed style maps in `styles.go`: `stateStyles`, `priorityStyles`
- Use `lipgloss.AdaptiveColor` for light/dark terminal support
- Status icons: `●` running, `○` pending, `✓` completed, `✗` failed, `◷` awaiting, `▣` tmux
- Helpers: `stateStyle()`, `priorityStyle()`, `priorityBadge()`, `statusIconFor()`

## Non-Worktree Mode

The `promptView` includes a worktree toggle (`alt+w`):
- When **on** (default): task runs in an isolated git worktree with its own branch
- When **off**: task runs directly in the project root directory; branch input is hidden
- The toggle state persists per-project (stored in DB via `default_worktree`) and within a TUI session (survives `Reset()`)
- When worktree is toggled off while branch field is focused, focus auto-switches to description

## Pitfalls

- List view uses custom `renderTask()`, NOT `table.Model`'s built-in rendering
- Detail view: check `contentDirty` before re-wrapping content (performance)
- Handle `tea.WindowSizeMsg` in every view to recalculate dimensions
- `promptView` auto-detects image paths from textarea — preserve this behavior

## Adjacent Packages

- **daemon** via `client.Client`: task CRUD, subscriptions, log streaming
- **config**: `ListWorkflowNames()`, `GetWorkflow()`, `ListPredefinedTaskNames()`
- **tmux**: `ListSessions()`, `AttachCommand()`, `SwitchClientCommand()`
- **workflow**: `ProjectLogsDir()`
