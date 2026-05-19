package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	gitpkg "github.com/aface/sortie/internal/git"
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

func (m Model) loadOutput(taskID int64, offset int) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return nil
		}
		lines, totalLines, err := m.client.GetLogs(taskID, 0, offset)
		if err != nil {
			return errorMsg(err)
		}
		return outputLoadedMsg{
			taskID:     taskID,
			lines:      lines,
			totalLines: totalLines,
			offset:     offset,
		}
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

func (m Model) revertTask(taskID int64) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return nil
		}
		if err := m.client.RevertTask(taskID); err != nil {
			return errorMsg(err)
		}
		return nil
	}
}

func (m Model) detachBranch(taskID int64) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return nil
		}
		if err := m.client.DetachBranch(taskID); err != nil {
			return errorMsg(err)
		}
		return nil
	}
}

func (m Model) attachBranch(taskID int64) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return nil
		}
		if err := m.client.AttachBranch(taskID); err != nil {
			return errorMsg(err)
		}
		return nil
	}
}

func (m Model) continueTask(taskID int64, workflow, prompt string) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return nil
		}
		if err := m.client.ContinueTask(taskID, workflow, prompt); err != nil {
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

func (m Model) addTaskDependency(taskID, blockedByID int64) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return nil
		}
		if err := m.client.AddTaskDependency(taskID, blockedByID); err != nil {
			return errorMsg(fmt.Errorf("failed to add dependency: %w", err))
		}
		return nil
	}
}

func (m Model) createTaskWithPrompt(title, description, branchName string, worktree bool, images []string, targetBranch, checkoutBranch string) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return nil
		}

		bm := int(m.prompt.branchMode)
		req := daemon.CreateTaskRequest{
			Title:          title,
			Description:    description,
			Workflow:       m.selectedWorkflow,
			BranchName:     branchName,
			TargetBranch:   targetBranch,
			CheckoutBranch: checkoutBranch,
			ProjectPath:    m.projectPath,
			Worktree:       &worktree,
			BranchMode:     &bm,
			Images:         images,
		}
		if m.blockingTaskID != 0 {
			req.BlockedBy = []int64{m.blockingTaskID}
		}
		info, err := m.client.CreateTaskWithOptions(req)
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

		worktree := true
		req := daemon.CreateTaskRequest{
			Description: description,
			Workflow:    m.selectedWorkflow,
			ProjectPath: m.projectPath,
			Worktree:    &worktree,
		}
		info, err := m.client.CreateTaskWithOptions(req)
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

// openEditorForStepContext launches $EDITOR with the current step context
// written to a temporary file. On editor exit the file contents are pushed
// back to the daemon via UpdateStepContext.
func (m Model) openEditorForStepContext(taskID int64, stepName, currentValue string) tea.Cmd {
	safeStep := strings.NewReplacer("/", "-", " ", "_").Replace(stepName)
	f, err := os.CreateTemp("", fmt.Sprintf("sortie-step-%s-*.md", safeStep))
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
		return editorStepContextFinishedMsg{taskID: taskID, stepName: stepName, path: path}
	})
}

func (m Model) handleStepContextEditorResult(msg editorStepContextFinishedMsg) tea.Cmd {
	return func() tea.Msg {
		defer os.Remove(msg.path)

		data, err := os.ReadFile(msg.path)
		if err != nil {
			return errorMsg(fmt.Errorf("failed to read temp file: %w", err))
		}

		value := string(data)
		if m.client == nil {
			return nil
		}

		if err := m.client.UpdateStepContext(msg.taskID, msg.stepName, value); err != nil {
			return errorMsg(fmt.Errorf("failed to update step context: %w", err))
		}

		return stepContextUpdatedMsg{taskID: msg.taskID, stepName: msg.stepName, context: value}
	}
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
	logFile := workflow.ProjectLogPath(dataDir, task.ID)

	if _, err := os.Stat(logFile); err != nil {
		return func() tea.Msg {
			return errorMsg(fmt.Errorf("no log file found for this task"))
		}
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

func (m Model) attachTmuxSession(taskID int64) tea.Cmd {
	taskIDStr := fmt.Sprintf("%d", taskID)
	projectName := ""
	if m.cfg != nil {
		projectName = m.cfg.Project.Name
	}
	session := tmux.NewSession(projectName, taskIDStr, "")
	if !session.Exists() {
		return func() tea.Msg {
			return errorMsg(fmt.Errorf("no tmux session found for task #%d", taskID))
		}
	}

	sessionName := session.Name

	if tmux.IsInsideTmux() {
		behavior := ""
		if m.cfg != nil {
			behavior = m.cfg.TmuxNestedAttachBehavior
		}
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
	projectName := ""
	if m.cfg != nil {
		projectName = m.cfg.Project.Name
	}
	return func() tea.Msg {
		sessions, err := tmux.ListSessions(tmux.SessionPrefix(projectName))
		if err != nil {
			return nil
		}
		result := make(map[int64]bool)
		for _, s := range sessions {
			taskIDStr := tmux.ExtractTaskID(projectName, s.Name)
			if taskID, err := strconv.ParseInt(taskIDStr, 10, 64); err == nil {
				result[taskID] = true
			}
		}
		return tmuxSessionsMsg(result)
	}
}

type branchesLoadedMsg []string

func (m Model) loadLocalBranches() tea.Cmd {
	return func() tea.Msg {
		branches, err := gitpkg.ListLocalBranches(m.projectPath)
		if err != nil {
			return errorMsg(fmt.Errorf("failed to list branches: %w", err))
		}
		if len(branches) == 0 {
			return errorMsg(fmt.Errorf("no local branches found"))
		}
		return branchesLoadedMsg(branches)
	}
}

func (m Model) createBranchTask(branch string) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return nil
		}

		worktree := true
		req := daemon.CreateTaskRequest{
			CheckoutBranch: branch,
			ProjectPath:    m.projectPath,
			Worktree:       &worktree,
			TmuxDirect:     true,
		}
		info, err := m.client.CreateTaskWithOptions(req)
		if err != nil {
			return errorMsg(fmt.Errorf("failed to create branch task: %w", err))
		}

		return taskCreatedMsg(*info)
	}
}

// blockingTaskTitleFromList looks up a task title by ID from the current list.
func (m Model) blockingTaskTitleFromList(taskID int64) string {
	for _, t := range m.list.allTasks {
		if t.ID == taskID {
			return t.Title
		}
	}
	return ""
}

// fuzzyFilterBranches returns branches that match the query using fuzzy matching.
// Characters in the query must appear in order in the branch name, but not necessarily contiguously.
func fuzzyFilterBranches(branches []string, query string) []string {
	if query == "" {
		result := make([]string, len(branches))
		copy(result, branches)
		return result
	}

	query = strings.ToLower(query)
	var result []string
	for _, b := range branches {
		if fuzzyMatch(strings.ToLower(b), query) {
			result = append(result, b)
		}
	}
	return result
}

// fuzzyMatch returns true if all characters in pattern appear in str in order.
func fuzzyMatch(str, pattern string) bool {
	pi := 0
	for i := 0; i < len(str) && pi < len(pattern); i++ {
		if str[i] == pattern[pi] {
			pi++
		}
	}
	return pi == len(pattern)
}
