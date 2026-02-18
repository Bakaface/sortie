package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/aface/ralph-tamer-kit/internal/client"
	"github.com/aface/ralph-tamer-kit/internal/config"
	"github.com/aface/ralph-tamer-kit/internal/daemon"
	"github.com/aface/ralph-tamer-kit/internal/tmux"
	tea "github.com/charmbracelet/bubbletea"
)

type view int

const (
	viewList view = iota
	viewDetail
)

type Model struct {
	cfg      *config.Config
	client   *client.Client
	keys     keyMap
	list     listView
	detail   detailView
	view     view
	width    int
	height   int
	err      error
	quitting bool

	// Confirmation state
	confirmAction string // "approve", "reject", or "delete"; empty if no confirmation pending
	confirmTaskID int64
	pendingDelete bool // tracks first "d" press for dd sequence

	// Workflow selection state
	selectingWorkflow bool
	workflowCursor    int
	selectedWorkflow  string
}

type clientConnectedMsg struct {
	client *client.Client
	tasks  []daemon.TaskInfo
}
type taskUpdateMsg daemon.TaskInfo
type taskCreatedMsg daemon.TaskInfo
type editorFinishedMsg struct{ path string }
type tasksLoadedMsg []daemon.TaskInfo
type outputLoadedMsg struct {
	lines []string
	total int
}
type errorMsg error
type tickMsg time.Time
type tmuxDetachedMsg struct{ taskID int64 }
type tmuxSessionsMsg map[int64]bool

func NewModel(cfg *config.Config) Model {
	return Model{
		cfg:    cfg,
		keys:   newKeyMap(),
		list:   newListView(),
		detail: newDetailView(),
		view:   viewList,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.connectToDaemon(),
		m.tickCmd(),
	)
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

		tasks, err := c.ListTasks()
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

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width, msg.Height)
		m.detail.SetSize(msg.Width, msg.Height)
		return m, nil

	case clientConnectedMsg:
		m.client = msg.client
		m.list.SetTasks(msg.tasks)
		return m, nil

	case tasksLoadedMsg:
		m.list.SetTasks(msg)
		return m, nil

	case taskUpdateMsg:
		m.list.UpdateTask(daemon.TaskInfo(msg))
		if m.view == viewDetail && m.detail.task != nil && m.detail.task.ID == msg.ID {
			task := daemon.TaskInfo(msg)
			m.detail.SetTask(&task)
		}
		return m, nil

	case editorFinishedMsg:
		return m, m.handleEditorResult(msg.path)

	case tmuxDetachedMsg:
		return m, m.refreshTasks()

	case tmuxSessionsMsg:
		m.list.tmuxSessions = msg
		return m, nil

	case taskDeletedMsg:
		m.list.RemoveTask(int64(msg))
		return m, nil

	case taskCreatedMsg:
		m.list.UpdateTask(daemon.TaskInfo(msg))
		return m, m.refreshTasks()

	case outputLoadedMsg:
		m.detail.SetOutput(msg.lines)
		return m, nil

	case tickMsg:
		var cmds []tea.Cmd

		if m.view == viewDetail && m.detail.task != nil && m.client != nil {
			cmds = append(cmds, m.loadOutput(m.detail.task.ID))
		}

		if m.client != nil {
			cmds = append(cmds, m.refreshTasks())
		}

		cmds = append(cmds, m.checkTmuxSessions())
		cmds = append(cmds, m.tickCmd())
		return m, tea.Batch(cmds...)

	case errorMsg:
		m.err = msg
		return m, nil
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Clear error on any keypress
	if m.err != nil {
		m.err = nil
		return m, nil
	}

	switch m.view {
	case viewList:
		return m.handleListKey(msg)
	case viewDetail:
		return m.handleDetailKey(msg)
	}
	return m, nil
}

func (m Model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle workflow selection if active
	if m.selectingWorkflow {
		return m.handleWorkflowSelectKey(msg)
	}

	// Handle confirmation prompt if active
	if m.confirmAction != "" {
		switch msg.String() {
		case "y":
			action := m.confirmAction
			taskID := m.confirmTaskID
			m.confirmAction = ""
			m.confirmTaskID = 0
			switch action {
			case "approve":
				return m, m.approveTask(taskID)
			case "delete":
				return m, m.deleteTask(taskID)
			default:
				return m, m.rejectTask(taskID)
			}
		case "n", "esc":
			m.confirmAction = ""
			m.confirmTaskID = 0
			return m, nil
		default:
			return m, nil
		}
	}

	keyStr := msg.String()

	// Handle "d" key for dd delete sequence
	if keyStr == "d" {
		if m.pendingDelete {
			// Second "d" — trigger delete confirmation
			m.pendingDelete = false
			if task := m.list.Selected(); task != nil && m.client != nil {
				m.confirmAction = "delete"
				m.confirmTaskID = task.ID
				return m, nil
			}
			return m, nil
		}
		// First "d" — enter pending state
		m.pendingDelete = true
		return m, nil
	}

	// Any other key resets pending delete
	m.pendingDelete = false

	switch keyStr {
	case "q", "ctrl+c":
		m.quitting = true
		if m.client != nil {
			m.client.Close()
		}
		return m, tea.Quit

	case "up", "k":
		m.list.MoveUp()
		return m, nil

	case "down", "j":
		m.list.MoveDown()
		return m, nil

	case "enter":
		if task := m.list.Selected(); task != nil {
			m.view = viewDetail
			m.detail.SetTask(task)
			m.detail.SetFollowMode(true)
			return m, m.loadOutput(task.ID)
		}
		return m, nil

	case "a":
		if task := m.list.Selected(); task != nil && m.client != nil {
			if task.Status == "awaiting_approval" {
				m.confirmAction = "approve"
				m.confirmTaskID = task.ID
				return m, nil
			}
		}
		return m, nil

	case "x":
		if task := m.list.Selected(); task != nil && m.client != nil {
			if task.Status == "awaiting_approval" {
				m.confirmAction = "reject"
				m.confirmTaskID = task.ID
				return m, nil
			}
		}
		return m, nil

	case "r":
		if task := m.list.Selected(); task != nil && m.client != nil {
			if task.Status == "failed" {
				return m, m.retryTask(task.ID)
			}
		}
		// Otherwise just refresh
		return m, m.refreshTasks()

	case "R":
		return m, m.refreshTasks()

	case "s":
		if task := m.list.Selected(); task != nil && m.client != nil {
			return m, m.stopTask(task.ID)
		}
		return m, nil

	case "t":
		if task := m.list.Selected(); task != nil {
			return m, m.attachTmuxSession(task.ID)
		}
		return m, nil

	case "n":
		if m.client == nil {
			return m, nil
		}
		workflows := m.cfg.ListWorkflowNames()
		if len(workflows) > 1 {
			m.selectingWorkflow = true
			m.workflowCursor = 0
			return m, nil
		}
		// Single workflow (or default) — skip selection
		m.selectedWorkflow = ""
		return m, m.openEditorForNewTask()

	case "?":
		m.list.showHelp = !m.list.showHelp
		return m, nil
	}

	return m, nil
}

func (m Model) handleWorkflowSelectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	workflows := m.cfg.ListWorkflowNames()

	switch msg.String() {
	case "up", "k":
		if m.workflowCursor > 0 {
			m.workflowCursor--
		}
		return m, nil
	case "down", "j":
		if m.workflowCursor < len(workflows)-1 {
			m.workflowCursor++
		}
		return m, nil
	case "enter":
		m.selectedWorkflow = workflows[m.workflowCursor]
		m.selectingWorkflow = false
		return m, m.openEditorForNewTask()
	case "esc", "q":
		m.selectingWorkflow = false
		return m, nil
	}

	// Number keys for quick selection (1-9)
	if len(msg.String()) == 1 && msg.String()[0] >= '1' && msg.String()[0] <= '9' {
		idx := int(msg.String()[0] - '1')
		if idx < len(workflows) {
			m.selectedWorkflow = workflows[idx]
			m.selectingWorkflow = false
			return m, m.openEditorForNewTask()
		}
	}

	return m, nil
}

func (m Model) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	// Common keys (both modes)
	switch keyStr {
	case "q":
		m.view = viewList
		return m, nil
	case "ctrl+c":
		if m.detail.task != nil && m.client != nil {
			return m, m.stopTask(m.detail.task.ID)
		}
		return m, nil
	case "t":
		if m.detail.task != nil {
			return m, m.attachTmuxSession(m.detail.task.ID)
		}
		return m, nil
	}

	// Navigation keys work in both modes; in follow mode they switch to normal first
	// Handle "gg" sequence
	if keyStr == "g" {
		if m.detail.IsPendingG() {
			m.detail.SetPendingG(false)
			m.detail.SetFollowMode(false)
			m.detail.GotoTop()
			return m, nil
		}
		m.detail.SetPendingG(true)
		return m, nil
	}
	// Any non-g key resets pendingG
	m.detail.SetPendingG(false)

	switch keyStr {
	case "G":
		m.detail.SetFollowMode(false)
		m.detail.GotoBottom()
		return m, nil
	case "j", "down":
		m.detail.SetFollowMode(false)
		m.detail.ScrollDown()
		return m, nil
	case "k", "up":
		m.detail.SetFollowMode(false)
		m.detail.ScrollUp()
		return m, nil
	case "ctrl+d":
		m.detail.SetFollowMode(false)
		m.detail.PageDown()
		return m, nil
	case "ctrl+u":
		m.detail.SetFollowMode(false)
		m.detail.PageUp()
		return m, nil
	case "enter":
		m.detail.SetFollowMode(true)
		return m, nil
	case "esc":
		if m.detail.IsFollowMode() {
			m.detail.SetFollowMode(false)
		} else {
			m.view = viewList
		}
		return m, nil
	}

	return m, nil
}

func (m Model) refreshTasks() tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return nil
		}
		tasks, err := m.client.ListTasks()
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

func (m Model) approveTask(taskID int64) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return nil
		}
		if err := m.client.ApproveTask(taskID); err != nil {
			return errorMsg(err)
		}
		return nil
	}
}

func (m Model) rejectTask(taskID int64) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return nil
		}
		if err := m.client.RejectTask(taskID); err != nil {
			return errorMsg(err)
		}
		return nil
	}
}

func (m Model) openEditorForNewTask() tea.Cmd {
	f, err := os.CreateTemp("", "rtk-new-task-*.md")
	if err != nil {
		return func() tea.Msg { return errorMsg(fmt.Errorf("failed to create temp file: %w", err)) }
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
		return editorFinishedMsg{path: path}
	})
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

		info, err := m.client.CreateTask(description, m.selectedWorkflow)
		if err != nil {
			return errorMsg(fmt.Errorf("failed to create task: %w", err))
		}

		return taskCreatedMsg(*info)
	}
}

type taskDeletedMsg int64

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

func (m Model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\nPress any key to continue.", m.err)
	}

	// Show workflow selection as its own screen
	if m.selectingWorkflow {
		workflows := m.cfg.ListWorkflowNames()
		var content string
		content += titleStyle.Render("Select Workflow") + "\n\n"
		for i, name := range workflows {
			label := fmt.Sprintf("  %d. %s", i+1, name)
			if i == m.workflowCursor {
				content += selectedStyle.Render("> "+label) + "\n"
			} else {
				content += "    " + label + "\n"
			}
		}
		content += "\n" + dimStyle.Render("  j/k: navigate | enter: select | esc: cancel")
		return content
	}

	var content string
	switch m.view {
	case viewDetail:
		content = m.detail.View()
	default:
		content = m.list.View()
	}

	// Show confirmation bar if active
	if m.confirmAction != "" {
		content += fmt.Sprintf("\n  %s task #%d? (y/n)", capitalize(m.confirmAction), m.confirmTaskID)
	}

	return content
}

func Run(cfg *config.Config) error {
	p := tea.NewProgram(
		NewModel(cfg),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	_, err := p.Run()
	return err
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func init() {
	_ = os.Setenv("TERM", os.Getenv("TERM"))
}
