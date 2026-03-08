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

See [references/file-map.md](references/file-map.md) for responsibility of each file and handler organization.

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
