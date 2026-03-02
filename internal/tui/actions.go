package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aface/sortie/internal/client"
	"github.com/aface/sortie/internal/daemon"
	"github.com/aface/sortie/internal/tmux"
	"github.com/aface/sortie/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
)

type taskFieldUpdatedMsg struct {
	field string
}

type taskDeletedMsg int64

func (m Model) listTasks(c *client.Client) ([]daemon.TaskInfo, error) {
	if m.projectID > 0 {
		return c.ListTasksFiltered(m.projectID)
	}
	if m.projectName != "" {
		return c.ListTasksByProjectName(m.projectName)
	}
	return c.ListTasksFiltered(0)
}

func (m Model) connectToDaemon() tea.Cmd {
	return func() tea.Msg {
		c := client.New(m.cfg)
		if err := c.Connect(); err != nil {
			return errorMsg(err)
		}

		if err := c.Subscribe(); err != nil {
			return errorMsg(err)
		}

		tasks, err := m.listTasks(c)
		if err != nil {
			return errorMsg(err)
		}

		// Drain subscription messages in background (picked up via tick refresh)
		go func() {
			for range c.Messages() {
			}
		}()

		return clientConnectedMsg{client: c, tasks: tasks}
	}
}

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) refreshTasks() tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return nil
		}
		tasks, err := m.listTasks(m.client)
		if err != nil {
			return errorMsg(err)
		}
		return tasksLoadedMsg(tasks)
	}
}

func (m Model) loadOutput(taskID int64) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return nil
		}
		lines, err := m.client.GetLogs(taskID, "", 0)
		if err != nil {
			return errorMsg(err)
		}
		return outputLoadedMsg{lines: lines, total: len(lines)}
	}
}

func (m Model) stopTask(taskID int64) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return nil
		}
		if err := m.client.StopTask(taskID); err != nil {
			return errorMsg(err)
		}
		return nil
	}
}

func (m Model) retryTask(taskID int64) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return nil
		}
		if err := m.client.RetryTask(taskID); err != nil {
			return errorMsg(err)
		}
		return nil
	}
}

func (m Model) continueTask(taskID int64, workflow string) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return nil
		}
		if err := m.client.ContinueTask(taskID, workflow); err != nil {
			return errorMsg(err)
		}
		return nil
	}
}

func (m Model) finalizeTask(taskID int64) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return nil
		}
		if err := m.client.FinalizeTask(taskID); err != nil {
			return errorMsg(err)
		}
		return nil
	}
}

func (m Model) deleteTask(taskID int64) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return nil
		}
		if err := m.client.DeleteTask(taskID); err != nil {
			return errorMsg(err)
		}
		return taskDeletedMsg(taskID)
	}
}

func (m Model) updateTaskPriority(taskID int64, priority string) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return nil
		}
		if err := m.client.UpdateTaskPriority(taskID, priority); err != nil {
			return errorMsg(err)
		}
		return nil
	}
}

func (m Model) createTaskWithPrompt(description, branchName string, images []string) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return nil
		}

		info, err := m.client.CreateTask(description, m.selectedWorkflow, branchName, m.projectPath, images)
		if err != nil {
			return errorMsg(fmt.Errorf("failed to create task: %w", err))
		}

		return taskCreatedMsg(*info)
	}
}

func (m Model) handleEditorResult(path string) tea.Cmd {
	return func() tea.Msg {
		defer os.Remove(path)

		data, err := os.ReadFile(path)
		if err != nil {
			return errorMsg(fmt.Errorf("failed to read temp file: %w", err))
		}

		description := strings.TrimSpace(string(data))
		if description == "" {
			return nil // User cancelled
		}

		if m.client == nil {
			return nil
		}

		info, err := m.client.CreateTask(description, m.selectedWorkflow, "", m.projectPath, nil)
		if err != nil {
			return errorMsg(fmt.Errorf("failed to create task: %w", err))
		}

		return taskCreatedMsg(*info)
	}
}

func (m Model) handleFieldEditorResult(msg editorFieldFinishedMsg) tea.Cmd {
	return func() tea.Msg {
		defer os.Remove(msg.path)

		data, err := os.ReadFile(msg.path)
		if err != nil {
			return errorMsg(fmt.Errorf("failed to read temp file: %w", err))
		}

		value := strings.TrimSpace(string(data))
		if value == "" {
			return nil // User cleared the field or cancelled
		}

		if m.client == nil {
			return nil
		}

		if err := m.client.UpdateTaskField(msg.taskID, msg.field, value); err != nil {
			return errorMsg(fmt.Errorf("failed to update %s: %w", msg.field, err))
		}

		return taskFieldUpdatedMsg{field: msg.field}
	}
}

func (m Model) openEditorForPrompt() tea.Cmd {
	f, err := os.CreateTemp("", "sortie-prompt-*.md")
	if err != nil {
		return func() tea.Msg { return errorMsg(fmt.Errorf("failed to create temp file: %w", err)) }
	}

	// Pre-populate with current textarea content
	if content := m.prompt.Value(); content != "" {
		if _, err := f.WriteString(content); err != nil {
			f.Close()
			os.Remove(f.Name())
			return func() tea.Msg { return errorMsg(fmt.Errorf("failed to write temp file: %w", err)) }
		}
	}
	f.Close()

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	path := f.Name()
	c := exec.Command(editor, path)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		if err != nil {
			os.Remove(path)
			return errorMsg(fmt.Errorf("editor exited with error: %w", err))
		}
		return editorPromptFinishedMsg{path: path}
	})
}

func (m Model) openEditorForField(taskID int64, field, currentValue string) tea.Cmd {
	f, err := os.CreateTemp("", fmt.Sprintf("sortie-%s-*.md", field))
	if err != nil {
		return func() tea.Msg { return errorMsg(fmt.Errorf("failed to create temp file: %w", err)) }
	}

	if currentValue != "" {
		if _, err := f.WriteString(currentValue); err != nil {
			f.Close()
			os.Remove(f.Name())
			return func() tea.Msg { return errorMsg(fmt.Errorf("failed to write temp file: %w", err)) }
		}
	}
	f.Close()

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	path := f.Name()
	c := exec.Command(editor, path)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		if err != nil {
			os.Remove(path)
			return errorMsg(fmt.Errorf("editor exited with error: %w", err))
		}
		return editorFieldFinishedMsg{taskID: taskID, field: field, path: path}
	})
}

func (m Model) openLogInEditor(task *daemon.TaskInfo) tea.Cmd {
	dataDir := filepath.Join(task.ProjectPath, ".sortie")
	logsDir := workflow.ProjectLogsDir(dataDir, task.ID)

	logFile, err := findLogFile(logsDir, task.CurrentStep)
	if err != nil {
		return func() tea.Msg { return errorMsg(err) }
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	c := exec.Command(editor, logFile)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		if err != nil {
			return errorMsg(fmt.Errorf("editor exited with error: %w", err))
		}
		return editorLogFinishedMsg{}
	})
}

// findLogFile determines which log file to open for a task.
// It prefers the current step's log, then falls back to the most recently modified log.
func findLogFile(logsDir, currentStep string) (string, error) {
	// Try current step's log first
	if currentStep != "" {
		stepLog := filepath.Join(logsDir, currentStep+".log")
		if _, err := os.Stat(stepLog); err == nil {
			return stepLog, nil
		}
	}

	// Fall back to the most recently modified log file
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return "", fmt.Errorf("no log files found for this task")
	}

	var newest string
	var newestTime time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if newest == "" || info.ModTime().After(newestTime) {
			newest = filepath.Join(logsDir, e.Name())
			newestTime = info.ModTime()
		}
	}

	if newest == "" {
		return "", fmt.Errorf("no log files found for this task")
	}
	return newest, nil
}

func (m Model) attachTmuxSession(taskID int64) tea.Cmd {
	taskIDStr := fmt.Sprintf("%d", taskID)
	prefix := tmux.SessionPrefix + taskIDStr + "-"
	sessions, err := tmux.ListSessions(prefix)
	if err != nil {
		return func() tea.Msg {
			return errorMsg(fmt.Errorf("failed to list tmux sessions: %w", err))
		}
	}
	if len(sessions) == 0 {
		return func() tea.Msg {
			return errorMsg(fmt.Errorf("no tmux session found for task #%d", taskID))
		}
	}

	sessionName := sessions[len(sessions)-1].Name

	if tmux.IsInsideTmux() {
		behavior := m.cfg.TmuxNestedAttachBehavior
		if behavior == "" {
			behavior = "switch"
		}

		if behavior == "nest" {
			// Nested attach: unset $TMUX and attach (creates nested tmux).
			// User detaches with prefix+d to return to the TUI.
			cmd := tmux.NestedAttachCommand(sessionName)
			return tea.ExecProcess(cmd, func(err error) tea.Msg {
				return tmuxDetachedMsg{taskID: taskID}
			})
		}

		// Default "switch": switch the client to the target session.
		// The TUI keeps running in the background. User returns with prefix+L.
		return func() tea.Msg {
			cmd := tmux.SwitchClientCommand(sessionName)
			if err := cmd.Run(); err != nil {
				return errorMsg(fmt.Errorf("failed to switch to tmux session: %w", err))
			}
			return tmuxDetachedMsg{taskID: taskID}
		}
	}

	// Not inside tmux: hand over the terminal to tmux attach.
	cmd := tmux.AttachCommand(sessionName)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return tmuxDetachedMsg{taskID: taskID}
	})
}

func (m Model) checkTmuxSessions() tea.Cmd {
	return func() tea.Msg {
		sessions, err := tmux.ListSessions(tmux.SessionPrefix)
		if err != nil {
			return nil
		}
		result := make(map[int64]bool)
		for _, s := range sessions {
			taskIDStr := tmux.ExtractTaskID(s.Name)
			if taskID, err := strconv.ParseInt(taskIDStr, 10, 64); err == nil {
				result[taskID] = true
			}
		}
		return tmuxSessionsMsg(result)
	}
}
