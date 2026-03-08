---
name: tmux
description: >
  Sortie's tmux session management: session creation, lifecycle, pane capture,
  key sending, and nested tmux detection. Use when editing files in internal/tmux/,
  working on tmux session creation, attachment, log piping, or session cleanup.
---

# Tmux Session Management

`internal/tmux/` manages tmux sessions for interactive task steps (single file: `session.go`).

## Session Naming

```go
const SessionPrefix = "sortie-"
```

- Basic: `sortie-<taskID>` (via `NewSession`)
- Step-scoped: `sortie-<taskID>-<stepName>` (via `NewStepSession`)

## Session Struct & Lifecycle

```go
type Session struct {
    Name    string
    WorkDir string
}

NewSession(taskID, workDir string) *Session
NewStepSession(taskID, stepName, workDir string) *Session
```

### Creation & Teardown
```go
(s *Session) Create(command string, args ...string) error  // tmux new-session -d
(s *Session) Exists() bool
(s *Session) IsAlive() bool                                // Has active processes
(s *Session) Kill() error
KillSessionsForTask(taskID string) error                   // Kill all sessions with prefix
```

## Interaction

```go
(s *Session) CapturePane(scrollbackLines int) ([]string, error)  // Capture terminal output
(s *Session) SendKeys(keys string) error                          // Send keystrokes
(s *Session) PipePane(logFile string) error                       // Stream output to log file
```

## Attachment Commands

```go
AttachCommand(sessionName string) *exec.Cmd           // tmux attach-session
SwitchClientCommand(sessionName string) *exec.Cmd     // tmux switch-client (when inside tmux)
NestedAttachCommand(sessionName string) *exec.Cmd     // For nested tmux scenarios
```

## Detection & Listing

```go
IsAvailable() bool                              // tmux binary exists and works
IsInsideTmux() bool                             // Checks $TMUX env var
ListSessions(prefix string) ([]*Session, error) // List sessions matching prefix
ExtractTaskID(sessionName string) string        // Parse task ID from session name
```

## Patterns

- Always check `IsAvailable()` before attempting tmux operations
- Use `SwitchClientCommand` when `IsInsideTmux()` is true, `AttachCommand` otherwise
- `NestedAttachCommand` used when `TmuxNestedAttachBehavior == "nest"`
- Session working directory set via `-c` flag on creation
- `PipePane` enables log capture for non-interactive monitoring
