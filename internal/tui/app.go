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
	"github.com/aface/sortie/internal/config"
	"github.com/aface/sortie/internal/daemon"
	"github.com/aface/sortie/internal/tmux"
	"github.com/aface/sortie/internal/workflow"
	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type view int

const (
	viewList view = iota
	viewDetail
	viewTaskInfo
	viewPrompt
	viewArtifact
)

type Model struct {
	cfg         *config.Config
	client      *client.Client
	keys        keyMap
	list        listView
	detail      detailView
	taskInfo    taskInfoView
	prompt      promptView
	view        view
	width       int
	height      int
	err         error
	quitting    bool
	projectID   int64  // 0 = global mode (show all projects)
	projectPath string // project directory path, empty in global mode
	globalMode  bool

	// Confirmation state
	confirmAction string // "approve", "reject", or "delete"; empty if no confirmation pending
	confirmTaskID int64
	pendingDelete bool // tracks first "d" press for dd sequence

	// Workflow selection state
	selectingWorkflow bool
	workflowCursor    int
	selectedWorkflow  string

	// Predefined task selection state
	selectingTask bool
	taskCursor    int

	// Priority selection state
	selectingPriority bool
	priorityCursor    int
	priorityTaskID    int64

	// Artifact pending key state
	pendingO bool // tracks first "o" press for oa sequence
	pendingE bool // tracks first "e" press for ea sequence

	// Artifact selection state
	selectingArtifact bool
	artifactCursor    int
	artifactNames     []string
	artifactTaskID    int64
	artifactWorktree  string
	artifactAction    string // "view" or "edit"

	// Artifact viewer state
	artifactView artifactViewState

	// Yank sequence state (task info view)
	pendingY bool

	// Command mode state (vim-style : commands)
	commandMode  bool
	commandInput string
}

type clientConnectedMsg struct {
	client *client.Client
	tasks  []daemon.TaskInfo
}
type taskUpdateMsg daemon.TaskInfo
type taskCreatedMsg daemon.TaskInfo
type editorFinishedMsg struct{ path string }
type editorPromptFinishedMsg struct{ path string }
type tasksLoadedMsg []daemon.TaskInfo
type outputLoadedMsg struct {
	lines []string
	total int
}
type errorMsg error
type tickMsg time.Time
type tmuxDetachedMsg struct{ taskID int64 }
type tmuxSessionsMsg map[int64]bool
type editorArtifactFinishedMsg struct{}
type artifactLoadedMsg struct {
	name    string
	content string
}

func NewModel(cfg *config.Config, projectID int64, projectPath string, globalMode bool) Model {
	return Model{
		cfg:         cfg,
		keys:        newKeyMap(),
		list:        newListView(globalMode),
		detail:      newDetailView(),
		taskInfo:    newTaskInfoView(),
		prompt:      newPromptView(),
		view:        viewList,
		projectID:   projectID,
		projectPath: projectPath,
		globalMode:  globalMode,
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

		tasks, err := c.ListTasksFiltered(m.projectID)
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
		m.taskInfo.SetSize(msg.Width, msg.Height)
		m.prompt.SetSize(msg.Width, msg.Height)
		m.artifactView.SetSize(msg.Width, msg.Height)
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
		if m.view == viewTaskInfo && m.taskInfo.task != nil && m.taskInfo.task.ID == msg.ID {
			task := daemon.TaskInfo(msg)
			m.taskInfo.SetTask(&task)
		}
		return m, nil

	case editorFinishedMsg:
		return m, m.handleEditorResult(msg.path)

	case editorPromptFinishedMsg:
		data, err := os.ReadFile(msg.path)
		os.Remove(msg.path)
		if err != nil {
			m.err = fmt.Errorf("failed to read temp file: %w", err)
			return m, nil
		}
		text := strings.TrimSpace(string(data))
		if text != "" {
			m.prompt.textarea.SetValue(text)
		}
		m.prompt.Focus()
		return m, nil

	case editorArtifactFinishedMsg:
		return m, nil

	case artifactLoadedMsg:
		m.artifactView.SetContent(msg.name, msg.content)
		m.view = viewArtifact
		return m, nil

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
		m.list.GotoTop()
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
	// Clear error on any keypress, but still process the key
	m.err = nil

	switch m.view {
	case viewList:
		return m.handleListKey(msg)
	case viewDetail:
		return m.handleDetailKey(msg)
	case viewTaskInfo:
		return m.handleTaskInfoKey(msg)
	case viewPrompt:
		return m.handlePromptKey(msg)
	case viewArtifact:
		return m.handleArtifactViewKey(msg)
	}
	return m, nil
}

func (m Model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle command mode input
	if m.commandMode {
		return m.handleCommandKey(msg)
	}

	// Handle priority selection if active
	if m.selectingPriority {
		return m.handlePrioritySelectKey(msg)
	}

	// Handle workflow selection if active
	if m.selectingWorkflow {
		return m.handleWorkflowSelectKey(msg)
	}

	// Handle predefined task selection if active
	if m.selectingTask {
		return m.handleTaskSelectKey(msg)
	}

	// Handle artifact selection if active
	if m.selectingArtifact {
		return m.handleArtifactSelectKey(msg)
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
			case "continue":
				return m, m.continueTask(taskID)
			case "finalize":
				return m, m.finalizeTask(taskID)
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

	// Handle help overlay — consume all keys except ? and esc which dismiss it
	if m.list.showHelp {
		if keyStr == "?" || keyStr == "esc" {
			m.list.showHelp = false
			return m, nil
		}
		return m, nil
	}

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

	// Handle "gg" sequence for go-to-top
	if keyStr == "g" {
		if m.list.IsPendingG() {
			m.list.SetPendingG(false)
			m.pendingDelete = false
			m.list.GotoTop()
			return m, nil
		}
		m.list.SetPendingG(true)
		return m, nil
	}

	// Handle second key after "o" prefix
	if m.pendingO {
		m.pendingO = false
		m.pendingDelete = false
		m.list.SetPendingG(false)
		if keyStr == "a" {
			if task := m.list.Selected(); task != nil {
				return m.openArtifactSelection(task, "view")
			}
		}
		return m, nil
	}

	// Handle second key after "e" prefix
	if m.pendingE {
		m.pendingE = false
		m.pendingDelete = false
		m.list.SetPendingG(false)
		if keyStr == "a" {
			if task := m.list.Selected(); task != nil {
				return m.openArtifactSelection(task, "edit")
			}
		}
		return m, nil
	}

	// Handle "o" key — start "oa" sequence
	if keyStr == "o" {
		m.pendingO = true
		m.pendingDelete = false
		m.list.SetPendingG(false)
		m.pendingE = false
		return m, nil
	}

	// Handle "e" key — start "ea" sequence
	if keyStr == "e" {
		m.pendingE = true
		m.pendingDelete = false
		m.list.SetPendingG(false)
		m.pendingO = false
		return m, nil
	}

	// Any other key resets pending states
	m.pendingDelete = false
	m.list.SetPendingG(false)
	m.pendingO = false
	m.pendingE = false

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

	case "G":
		m.list.GotoBottom()
		return m, nil

	case "ctrl+d", "pgdown":
		m.list.PageDown()
		return m, nil

	case "ctrl+u", "pgup":
		m.list.PageUp()
		return m, nil

	case "enter":
		if task := m.list.Selected(); task != nil {
			m.view = viewTaskInfo
			m.taskInfo.SetTask(task)
			m.taskInfo.SetWorkflow(m.cfg.GetWorkflow(task.Workflow))
			return m, nil
		}
		return m, nil

	case "l":
		if task := m.list.Selected(); task != nil {
			m.view = viewDetail
			m.detail.SetTask(task)
			m.detail.SetFollowMode(true)
			return m, m.loadOutput(task.ID)
		}
		return m, nil

	case "a":
		if task := m.list.Selected(); task != nil && m.client != nil {
			if task.Status == "awaiting-approval" || task.Status == "tmux" {
				m.confirmAction = "approve"
				m.confirmTaskID = task.ID
				return m, nil
			}
		}
		return m, nil

	case "x":
		if task := m.list.Selected(); task != nil && m.client != nil {
			if task.Status == "awaiting-approval" || task.Status == "tmux" {
				m.confirmAction = "reject"
				m.confirmTaskID = task.ID
				return m, nil
			}
		}
		return m, nil

	case "r":
		// Retry if selected task is failed
		if task := m.list.Selected(); task != nil && m.client != nil {
			if task.Status == "failed" {
				return m, m.retryTask(task.ID)
			}
		}
		// Show predefined task selection if tasks are configured
		if m.client != nil && m.projectPath != "" {
			tasks := m.cfg.ListPredefinedTaskNames()
			if len(tasks) > 0 {
				m.selectingTask = true
				m.taskCursor = 0
				return m, nil
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

	case "c":
		if task := m.list.Selected(); task != nil && m.client != nil {
			if task.Status == "completed" || task.Status == "failed" || task.Status == "artifact-missing" {
				m.confirmAction = "continue"
				m.confirmTaskID = task.ID
				return m, nil
			}
			if task.Status == "tmux" {
				m.confirmAction = "finalize"
				m.confirmTaskID = task.ID
				return m, nil
			}
		}
		return m, nil

	case "p":
		if task := m.list.Selected(); task != nil && m.client != nil {
			m.selectingPriority = true
			m.priorityTaskID = task.ID
			m.priorityCursor = 0
			return m, nil
		}
		return m, nil

	case "n":
		if m.client == nil || m.projectPath == "" {
			return m, nil
		}
		workflows := m.cfg.ListWorkflowNames()
		if len(workflows) > 1 {
			m.selectingWorkflow = true
			m.workflowCursor = 0
			return m, nil
		}
		// Single workflow (or default) — skip selection and open prompt view
		m.selectedWorkflow = ""
		m.view = viewPrompt
		m.prompt.Reset()
		m.prompt.Focus()
		return m, nil

	case "?":
		m.list.showHelp = !m.list.showHelp
		return m, nil

	case ":":
		m.commandMode = true
		m.commandInput = ""
		return m, nil
	}

	return m, nil
}

func (m Model) handleCommandKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	switch keyStr {
	case "esc":
		m.commandMode = false
		m.commandInput = ""
		return m, nil

	case "enter":
		input := m.commandInput
		m.commandMode = false
		m.commandInput = ""
		return executeCommand(m, input)

	case "backspace":
		if len(m.commandInput) > 0 {
			m.commandInput = m.commandInput[:len(m.commandInput)-1]
		}
		if m.commandInput == "" {
			m.commandMode = false
		}
		return m, nil

	default:
		// Only accept printable characters
		if len(keyStr) == 1 && keyStr[0] >= ' ' && keyStr[0] <= '~' {
			m.commandInput += keyStr
		}
		return m, nil
	}
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
		m.view = viewPrompt
		m.prompt.Reset()
		m.prompt.Focus()
		return m, nil
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
			m.view = viewPrompt
			m.prompt.Reset()
			m.prompt.Focus()
			return m, nil
		}
	}

	return m, nil
}

func (m Model) handleTaskSelectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	tasks := m.cfg.ListPredefinedTaskNames()

	switch msg.String() {
	case "up", "k":
		if m.taskCursor > 0 {
			m.taskCursor--
		}
		return m, nil
	case "down", "j":
		if m.taskCursor < len(tasks)-1 {
			m.taskCursor++
		}
		return m, nil
	case "enter":
		taskName := tasks[m.taskCursor]
		taskCfg := m.cfg.GetPredefinedTask(taskName)
		m.selectingTask = false
		if taskCfg == nil {
			return m, nil
		}
		// Create task directly with the predefined description and workflow
		m.selectedWorkflow = "task:" + taskCfg.Name
		description := taskCfg.Description
		if description == "" {
			description = taskCfg.Name
		}
		return m, m.createTaskWithPrompt(description, nil)
	case "esc", "q":
		m.selectingTask = false
		return m, nil
	}

	// Number keys for quick selection (1-9)
	if len(msg.String()) == 1 && msg.String()[0] >= '1' && msg.String()[0] <= '9' {
		idx := int(msg.String()[0] - '1')
		if idx < len(tasks) {
			taskName := tasks[idx]
			taskCfg := m.cfg.GetPredefinedTask(taskName)
			m.selectingTask = false
			if taskCfg == nil {
				return m, nil
			}
			m.selectedWorkflow = "task:" + taskCfg.Name
			description := taskCfg.Description
			if description == "" {
				description = taskCfg.Name
			}
			return m, m.createTaskWithPrompt(description, nil)
		}
	}

	return m, nil
}

func (m Model) handlePrioritySelectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	priorities := []string{"low", "medium", "high", "urgent"}

	switch msg.String() {
	case "up", "k":
		if m.priorityCursor > 0 {
			m.priorityCursor--
		}
		return m, nil
	case "down", "j":
		if m.priorityCursor < len(priorities)-1 {
			m.priorityCursor++
		}
		return m, nil
	case "enter":
		selected := priorities[m.priorityCursor]
		m.selectingPriority = false
		return m, m.updateTaskPriority(m.priorityTaskID, selected)
	case "esc", "q":
		m.selectingPriority = false
		return m, nil
	}

	// Number keys for quick selection (1-4)
	if len(msg.String()) == 1 && msg.String()[0] >= '1' && msg.String()[0] <= '4' {
		idx := int(msg.String()[0] - '1')
		selected := priorities[idx]
		m.selectingPriority = false
		return m, m.updateTaskPriority(m.priorityTaskID, selected)
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
	case "ctrl+d", "pgdown":
		m.detail.SetFollowMode(false)
		m.detail.PageDown()
		return m, nil
	case "ctrl+u", "pgup":
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

func (m Model) handleTaskInfoKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	// Handle artifact selection if active (must come before q/esc handling)
	if m.selectingArtifact {
		return m.handleArtifactSelectKey(msg)
	}

	switch keyStr {
	case "q", "esc":
		m.view = viewList
		return m, nil
	case "ctrl+c":
		if m.taskInfo.task != nil && m.client != nil {
			return m, m.stopTask(m.taskInfo.task.ID)
		}
		return m, nil
	case "t":
		if m.taskInfo.task != nil {
			return m, m.attachTmuxSession(m.taskInfo.task.ID)
		}
		return m, nil
	case "l":
		if m.taskInfo.task != nil {
			m.view = viewDetail
			m.detail.SetTask(m.taskInfo.task)
			m.detail.SetFollowMode(true)
			return m, m.loadOutput(m.taskInfo.task.ID)
		}
		return m, nil
	}

	// Handle second key after "o" prefix
	if m.pendingO {
		m.pendingO = false
		if keyStr == "a" {
			if m.taskInfo.task != nil {
				return m.openArtifactSelection(m.taskInfo.task, "view")
			}
		}
		// Fall through to handle this key normally
	}

	// Handle second key after "e" prefix
	if m.pendingE {
		m.pendingE = false
		if keyStr == "a" {
			if m.taskInfo.task != nil {
				return m.openArtifactSelection(m.taskInfo.task, "edit")
			}
		}
		// Fall through to handle this key normally
	}

	// Handle "o" key — start "oa" sequence
	if keyStr == "o" {
		m.pendingO = true
		m.pendingE = false
		m.pendingY = false
		return m, nil
	}

	// Handle "e" key — start "ea" sequence
	if keyStr == "e" {
		m.pendingE = true
		m.pendingO = false
		m.pendingY = false
		return m, nil
	}

	// Handle "y" prefix for yank sequences (yd, yc)
	if keyStr == "y" {
		m.pendingY = true
		m.pendingO = false
		m.pendingE = false
		m.taskInfo.pendingG = false
		return m, nil
	}

	if m.pendingY {
		m.pendingY = false
		m.taskInfo.pendingG = false
		switch keyStr {
		case "d":
			if m.taskInfo.task != nil && m.taskInfo.task.Description != "" {
				if err := clipboard.WriteAll(m.taskInfo.task.Description); err != nil {
					m.err = fmt.Errorf("clipboard: %w", err)
				}
			}
			return m, nil
		case "c":
			if m.taskInfo.task != nil && m.taskInfo.task.Context != "" {
				if err := clipboard.WriteAll(m.taskInfo.task.Context); err != nil {
					m.err = fmt.Errorf("clipboard: %w", err)
				}
			}
			return m, nil
		}
		// Any other key after "y" — fall through to normal handling
	}

	// Handle "gg" sequence
	if keyStr == "g" {
		if m.taskInfo.pendingG {
			m.taskInfo.pendingG = false
			m.taskInfo.GotoTop()
			return m, nil
		}
		m.taskInfo.pendingG = true
		return m, nil
	}
	m.taskInfo.pendingG = false

	switch keyStr {
	case "G":
		m.taskInfo.GotoBottom()
		return m, nil
	case "j", "down":
		m.taskInfo.ScrollDown()
		return m, nil
	case "k", "up":
		m.taskInfo.ScrollUp()
		return m, nil
	case "ctrl+d", "pgdown":
		m.taskInfo.PageDown()
		return m, nil
	case "ctrl+u", "pgup":
		m.taskInfo.PageUp()
		return m, nil
	}

	return m, nil
}

func (m Model) handlePromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	switch keyStr {
	case "enter":
		// Submit the task
		description := m.prompt.Value()
		if description == "" {
			return m, nil
		}
		images := m.prompt.Images()
		m.view = viewList
		return m, m.createTaskWithPrompt(description, images)

	case "esc":
		// Cancel and return to list
		m.view = viewList
		return m, nil

	case "ctrl+g":
		// Open $EDITOR for prompt editing
		return m, m.openEditorForPrompt()

	case "ctrl+x":
		// Remove last image
		m.prompt.RemoveLastImage()
		return m, nil

	default:
		// Pass all other keys to the prompt view
		cmd := m.prompt.Update(msg)
		return m, cmd
	}
}

func (m Model) refreshTasks() tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return nil
		}
		tasks, err := m.client.ListTasksFiltered(m.projectID)
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

		info, err := m.client.CreateTask(description, m.selectedWorkflow, m.projectPath, nil)
		if err != nil {
			return errorMsg(fmt.Errorf("failed to create task: %w", err))
		}

		return taskCreatedMsg(*info)
	}
}

func (m Model) createTaskWithPrompt(description string, images []string) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return nil
		}

		info, err := m.client.CreateTask(description, m.selectedWorkflow, m.projectPath, images)
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

func (m Model) continueTask(taskID int64) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return nil
		}
		if err := m.client.ContinueTask(taskID); err != nil {
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

// openArtifactSelection starts the artifact selection flow for a task.
func (m Model) openArtifactSelection(task *daemon.TaskInfo, action string) (tea.Model, tea.Cmd) {
	if task.WorktreePath == "" {
		m.err = fmt.Errorf("no worktree available for task #%d", task.ID)
		return m, nil
	}

	wf := m.cfg.GetWorkflow(task.Workflow)
	if wf == nil {
		m.err = fmt.Errorf("no workflow found for task #%d", task.ID)
		return m, nil
	}

	// Find steps with artifact: true that have actual files on disk
	artifactsDir := workflow.ArtifactsDir(task.WorktreePath)
	var names []string
	for _, step := range wf.Steps {
		if step.Artifact {
			path := filepath.Join(artifactsDir, step.Name+".md")
			if _, err := os.Stat(path); err == nil {
				names = append(names, step.Name)
			}
		}
	}

	if len(names) == 0 {
		m.err = fmt.Errorf("no artifacts available for task #%d", task.ID)
		return m, nil
	}

	// If only one artifact, skip selection
	if len(names) == 1 {
		return m.performArtifactAction(task.WorktreePath, names[0], action)
	}

	m.selectingArtifact = true
	m.artifactCursor = 0
	m.artifactNames = names
	m.artifactTaskID = task.ID
	m.artifactWorktree = task.WorktreePath
	m.artifactAction = action
	return m, nil
}

// handleArtifactSelectKey handles key input in the artifact selection menu.
func (m Model) handleArtifactSelectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.artifactCursor > 0 {
			m.artifactCursor--
		}
		return m, nil
	case "down", "j":
		if m.artifactCursor < len(m.artifactNames)-1 {
			m.artifactCursor++
		}
		return m, nil
	case "enter":
		name := m.artifactNames[m.artifactCursor]
		m.selectingArtifact = false
		return m.performArtifactAction(m.artifactWorktree, name, m.artifactAction)
	case "esc", "q":
		m.selectingArtifact = false
		return m, nil
	}

	// Number keys for quick selection (1-9)
	if len(msg.String()) == 1 && msg.String()[0] >= '1' && msg.String()[0] <= '9' {
		idx := int(msg.String()[0] - '1')
		if idx < len(m.artifactNames) {
			name := m.artifactNames[idx]
			m.selectingArtifact = false
			return m.performArtifactAction(m.artifactWorktree, name, m.artifactAction)
		}
	}

	return m, nil
}

// performArtifactAction either views or edits the selected artifact.
func (m Model) performArtifactAction(worktreePath, stepName, action string) (tea.Model, tea.Cmd) {
	artifactPath := filepath.Join(workflow.ArtifactsDir(worktreePath), stepName+".md")

	if action == "edit" {
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vi"
		}
		c := exec.Command(editor, artifactPath)
		return m, tea.ExecProcess(c, func(err error) tea.Msg {
			if err != nil {
				return errorMsg(fmt.Errorf("editor exited with error: %w", err))
			}
			return editorArtifactFinishedMsg{}
		})
	}

	// View action: read content and display
	return m, func() tea.Msg {
		content, err := workflow.ReadArtifact(worktreePath, stepName)
		if err != nil {
			return errorMsg(fmt.Errorf("failed to read artifact: %w", err))
		}
		if content == "" {
			return errorMsg(fmt.Errorf("artifact %q is empty", stepName))
		}
		return artifactLoadedMsg{name: stepName, content: content}
	}
}

// handleArtifactViewKey handles key input in the artifact viewer.
func (m Model) handleArtifactViewKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	switch keyStr {
	case "q", "esc":
		m.view = viewList
		return m, nil
	}

	// Handle "gg" sequence
	if keyStr == "g" {
		if m.artifactView.pendingG {
			m.artifactView.pendingG = false
			m.artifactView.GotoTop()
			return m, nil
		}
		m.artifactView.pendingG = true
		return m, nil
	}
	m.artifactView.pendingG = false

	switch keyStr {
	case "G":
		m.artifactView.GotoBottom()
		return m, nil
	case "j", "down":
		m.artifactView.ScrollDown()
		return m, nil
	case "k", "up":
		m.artifactView.ScrollUp()
		return m, nil
	case "ctrl+d", "pgdown":
		m.artifactView.PageDown()
		return m, nil
	case "ctrl+u", "pgup":
		m.artifactView.PageUp()
		return m, nil
	}

	return m, nil
}

func (m Model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	if m.err != nil {
		return fmt.Sprintf("  Error: %v\n\n  Press any key to continue.", m.err)
	}

	// Show help overlay
	if m.list.showHelp && m.view == viewList {
		return m.renderHelpOverlay()
	}

	// Show priority selection as its own screen
	if m.selectingPriority {
		priorities := []string{"low", "medium", "high", "urgent"}
		var b strings.Builder
		b.WriteString(titleStyle.Render(" Select Priority ") + "\n\n")
		for i, name := range priorities {
			label := fmt.Sprintf("  %d. %s", i+1, name)
			if i == m.priorityCursor {
				b.WriteString(selectedStyle.Render("> "+label) + "\n")
			} else {
				b.WriteString("    " + priorityStyle(name).Render(label) + "\n")
			}
		}
		b.WriteString("\n" + dimStyle.Render("  j/k: navigate | enter: select | 1-4: quick select | esc: cancel"))
		return b.String()
	}

	// Show workflow selection as its own screen
	if m.selectingWorkflow {
		workflows := m.cfg.ListWorkflowNames()
		var b strings.Builder
		b.WriteString(titleStyle.Render(" Select Workflow ") + "\n\n")
		for i, name := range workflows {
			label := fmt.Sprintf("  %d. %s", i+1, name)
			if i == m.workflowCursor {
				b.WriteString(selectedStyle.Render("> "+label) + "\n")
			} else {
				b.WriteString("    " + label + "\n")
			}
		}
		b.WriteString("\n" + dimStyle.Render("  j/k: navigate | enter: select | esc: cancel"))
		return b.String()
	}

	// Show predefined task selection as its own screen
	if m.selectingTask {
		tasks := m.cfg.ListPredefinedTaskNames()
		var b strings.Builder
		b.WriteString(titleStyle.Render(" Run Predefined Task ") + "\n\n")
		for i, name := range tasks {
			taskCfg := m.cfg.GetPredefinedTask(name)
			label := fmt.Sprintf("  %d. %s", i+1, name)
			if i == m.taskCursor {
				b.WriteString(selectedStyle.Render("> "+label) + "\n")
				if taskCfg != nil && taskCfg.Description != "" {
					b.WriteString(dimStyle.Render("     "+taskCfg.Description) + "\n")
				}
			} else {
				b.WriteString("    " + label + "\n")
			}
		}
		b.WriteString("\n" + dimStyle.Render("  j/k: navigate | enter: select | esc: cancel"))
		return b.String()
	}

	// Show artifact selection as its own screen
	if m.selectingArtifact {
		var b strings.Builder
		title := " Select Artifact "
		if m.artifactAction == "edit" {
			title = " Edit Artifact "
		}
		b.WriteString(titleStyle.Render(title) + "\n\n")
		for i, name := range m.artifactNames {
			label := fmt.Sprintf("  %d. %s", i+1, name)
			if i == m.artifactCursor {
				b.WriteString(selectedStyle.Render("> "+label) + "\n")
			} else {
				b.WriteString("    " + label + "\n")
			}
		}
		b.WriteString("\n" + dimStyle.Render("  j/k: navigate | enter: select | 1-9: quick select | esc: cancel"))
		return b.String()
	}

	var content string
	switch m.view {
	case viewDetail:
		content = m.detail.View()
	case viewTaskInfo:
		content = m.taskInfo.View()
	case viewPrompt:
		content = m.prompt.View()
	case viewArtifact:
		content = m.artifactView.View()
	default:
		content = m.list.View()
	}

	// Show confirmation bar if active
	if m.confirmAction != "" {
		content += fmt.Sprintf("\n  %s task #%d? (y/n)", capitalize(m.confirmAction), m.confirmTaskID)
	}

	// Show command bar if in command mode
	if m.commandMode {
		content += fmt.Sprintf("\n  :%s█", m.commandInput)
	}

	return content
}

func (m Model) renderHelpOverlay() string {
	keys := newKeyMap()
	groups := keys.FullHelp()

	groupNames := []string{"Navigation", "Actions", "General"}

	headingStyle := lipgloss.NewStyle().Bold(true).Foreground(highlight)

	var b strings.Builder
	b.WriteString(titleStyle.Render(" Help ") + "\n\n")

	for i, group := range groups {
		if i < len(groupNames) {
			b.WriteString("  " + headingStyle.Render(groupNames[i]) + "\n")
		}
		for _, binding := range group {
			fmt.Fprintf(&b, "    %-12s %s\n", dimStyle.Render(binding.Help().Key), helpStyle.Render(binding.Help().Desc))
		}
		b.WriteString("\n")
	}

	b.WriteString(dimStyle.Render("  Press ? or esc to close"))
	return b.String()
}

func Run(cfg *config.Config, projectID int64, projectPath string, globalMode bool) error {
	p := tea.NewProgram(
		NewModel(cfg, projectID, projectPath, globalMode),
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
