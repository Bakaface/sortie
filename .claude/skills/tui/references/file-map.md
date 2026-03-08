# TUI File Map

## Core

| File | Responsibility |
|------|----------------|
| `app.go` | Root Model, Init/Update/View routing, global state, message handling |
| `keys.go` | KeyMap structs for each view mode |
| `styles.go` | Pre-computed `lipgloss.Style` maps, status icons, adaptive colors |

## Views

| File | Responsibility |
|------|----------------|
| `list.go` | Task table with custom row rendering, scroll offset, search matching |
| `detail.go` | Viewport-based log viewer with follow mode, ANSI stripping |
| `prompt.go` | Dual-field form (textarea + textinput), worktree toggle, image detection |
| `task_info.go` | Metadata display + workflow step progress (icons: `○`/`●`/`✓`/`✗`) |
| `artifact_view.go` | Artifact content viewer |

## Handlers

| File | Responsibility |
|------|----------------|
| `update_list.go` | List view: navigation, task actions, selections, search, commands |
| `update_detail.go` | Detail view: scroll, follow toggle, back |
| `update_task_info.go` | Task info: scroll, copy-to-clipboard (yd/yc), edit fields, artifact selection |
| `update_prompt.go` | Prompt view: submit, tab switching, editor open, worktree toggle |
| `update_artifact.go` | Artifact selection/viewing: list navigation, open/edit |

## Actions & Utilities

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
