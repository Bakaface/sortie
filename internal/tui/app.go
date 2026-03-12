package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aface/sortie/internal/client"
	"github.com/aface/sortie/internal/config"
	"github.com/aface/sortie/internal/daemon"
	"github.com/charmbracelet/bubbles/spinner"
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
	projectPath     string // project directory path, empty in global mode
	projectName     string // repo basename for filtering in global mode
	globalMode      bool
	defaultWorktree bool   // per-project worktree preference

	// Blocking task creation state
	blockingTaskID int64 // when non-zero, the newly created task will block this task

	// Confirmation state
	confirmAction string // "continue", "finalize", or "delete"; empty if no confirmation pending
	confirmTaskID int64
	pendingDelete bool // tracks first "d" press for dd sequence

	// Workflow selection state
	selectingWorkflow bool
	workflowCursor    int
	workflowPendingG  bool
	selectedWorkflow  string

	// Continue workflow selection state
	selectingContinueWorkflow bool
	continueWorkflowCursor    int
	continueWorkflowPendingG  bool
	continueTaskID            int64
	continueSelectedWorkflow  string // workflow selected for continue, held while user enters prompt

	// Predefined task (one-off) selection state
	selectingTask bool
	taskCursor    int
	taskPendingG  bool

	// Init workflow selection state
	selectingInit bool
	initCursor    int
	initPendingG  bool

	// Priority selection state
	selectingPriority bool
	priorityCursor    int
	priorityPendingG  bool
	priorityTaskID    int64

	// Artifact pending key state
	pendingO bool // tracks first "o" press for oa sequence
	pendingE bool // tracks first "e" press for ea sequence

	// Artifact selection state
	selectingArtifact bool
	artifactCursor    int
	artifactPendingG  bool
	artifactNames     []string
	artifactTaskID    int64
	artifactWorktree  string
	artifactAction    string // "view" or "edit"

	// Artifact viewer state
	artifactView artifactViewState

	// Yank sequence state (task info view)
	pendingY bool

	// Status message (flash message, auto-clears after a few ticks)
	statusMessage    string
	statusMessageTTL int // ticks remaining before auto-clear

	// Command mode state (vim-style : commands)
	commandMode  bool
	commandInput string

	// Search mode state (vim-style / and ? search)
	searchMode      bool
	searchQuery     string
	searchDirection int // 1 for forward (/), -1 for backward (?)
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
type editorLogFinishedMsg struct{}
type editorFieldFinishedMsg struct {
	taskID int64
	field  string
	path   string
}
type artifactLoadedMsg struct {
	name    string
	content string
}

func NewModel(cfg *config.Config, projectID int64, projectPath, projectName string, globalMode bool, defaultWorktree bool) Model {
	return Model{
		cfg:             cfg,
		keys:            newKeyMap(),
		list:            newListView(globalMode, projectName),
		detail:          newDetailView(),
		taskInfo:        newTaskInfoView(),
		prompt:          newPromptView(defaultWorktree),
		view:            viewList,
		projectID:       projectID,
		projectPath:     projectPath,
		projectName:     projectName,
		globalMode:      globalMode,
		defaultWorktree: defaultWorktree,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.connectToDaemon(),
		m.tickCmd(),
		m.list.spinner.Tick,
	)
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
		m.list.refreshing = false
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

	case editorLogFinishedMsg:
		return m, nil

	case editorFieldFinishedMsg:
		return m, m.handleFieldEditorResult(msg)

	case taskFieldUpdatedMsg:
		label := msg.field
		if len(label) > 0 {
			label = strings.ToUpper(label[:1]) + label[1:]
		}
		m.statusMessage = fmt.Sprintf("%s updated", label)
		m.statusMessageTTL = 2
		m.list.refreshing = true
		return m, m.refreshTasks()

	case artifactLoadedMsg:
		m.artifactView.SetContent(msg.name, msg.content)
		m.view = viewArtifact
		return m, nil

	case tmuxDetachedMsg:
		m.list.refreshing = true
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
		if m.blockingTaskID != 0 {
			blockedTaskID := m.blockingTaskID
			newTaskID := msg.ID
			m.blockingTaskID = 0
			return m, m.addTaskDependency(blockedTaskID, newTaskID)
		}
		return m, nil

	case outputLoadedMsg:
		m.detail.SetOutput(msg.lines)
		return m, nil

	case tickMsg:
		var cmds []tea.Cmd

		// Auto-clear status message after TTL expires
		if m.statusMessage != "" {
			m.statusMessageTTL--
			if m.statusMessageTTL <= 0 {
				m.statusMessage = ""
			}
		}

		if m.view == viewDetail && m.detail.task != nil && m.client != nil {
			cmds = append(cmds, m.loadOutput(m.detail.task.ID))
		}

		if m.client != nil && !m.list.refreshing {
			m.list.refreshing = true
			cmds = append(cmds, m.refreshTasks())
		}

		cmds = append(cmds, m.checkTmuxSessions())
		cmds = append(cmds, m.tickCmd())
		return m, tea.Batch(cmds...)

	case errorMsg:
		m.err = msg
		return m, nil

	case spinner.TickMsg:
		if m.list.loading {
			cmd := m.list.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Clear error and status message on any keypress, but still process the key
	m.err = nil
	m.statusMessage = ""

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

	// Show prompt help overlay
	if m.prompt.showHelp && m.view == viewPrompt {
		return m.renderPromptHelpOverlay()
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

	// Show continue workflow selection as its own screen
	if m.selectingContinueWorkflow {
		workflows := m.cfg.ListWorkflowNames()
		var b strings.Builder
		b.WriteString(titleStyle.Render(fmt.Sprintf(" Continue Task #%d - Select Workflow ", m.continueTaskID)) + "\n\n")
		for i, name := range workflows {
			label := fmt.Sprintf("  %d. %s", i+1, name)
			if i == m.continueWorkflowCursor {
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

	// Show init workflow selection as its own screen
	if m.selectingInit {
		inits := m.cfg.ListInitWorkflowNames()
		var b strings.Builder
		b.WriteString(titleStyle.Render(" Run Init Workflow ") + "\n\n")
		for i, name := range inits {
			initCfg := m.cfg.GetInitWorkflow(name)
			label := fmt.Sprintf("  %d. %s", i+1, name)
			if i == m.initCursor {
				b.WriteString(selectedStyle.Render("> "+label) + "\n")
				if initCfg != nil && initCfg.Description != "" {
					b.WriteString(dimStyle.Render("     "+initCfg.Description) + "\n")
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

	// Build bottom bar lines (command bar, search bar, confirmation, status message).
	// These are rendered at the very bottom of the terminal.
	var bottomLines []string
	if m.confirmAction != "" {
		bottomLines = append(bottomLines, fmt.Sprintf("  %s task #%d? (y/n)", capitalize(m.confirmAction), m.confirmTaskID))
	}
	if m.commandMode {
		bottomLines = append(bottomLines, fmt.Sprintf("  :%s█", m.commandInput))
	}
	if m.searchMode {
		searchChar := "/"
		if m.searchDirection < 0 {
			searchChar = "?"
		}
		matchInfo := ""
		if len(m.list.matchedIndices) > 0 {
			matchInfo = fmt.Sprintf(" [%d/%d]", m.list.currentMatchIdx+1, len(m.list.matchedIndices))
		}
		bottomLines = append(bottomLines, fmt.Sprintf("%s%s█%s", searchChar, m.searchQuery, matchInfo))
	}
	if m.statusMessage != "" {
		bottomLines = append(bottomLines, fmt.Sprintf("  %s", m.statusMessage))
	}

	// Count extra lines so views can reserve space for the bottom bar.
	extra := len(bottomLines)
	m.list.extraLines = extra

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

	// Append bottom bar lines, padded to the very bottom of the terminal.
	if len(bottomLines) > 0 {
		if m.height > 0 {
			contentLines := strings.Count(content, "\n")
			// Account for the trailing newline we add at the end
			totalUsed := contentLines + 1 + len(bottomLines)
			padding := m.height - totalUsed
			if padding > 0 {
				content += strings.Repeat("\n", padding)
			}
		}
		content += "\n" + strings.Join(bottomLines, "\n")
	}

	return content + "\n"
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

	b.WriteString(dimStyle.Render("  Press ctrl+h or esc to close"))
	return b.String()
}

func (m Model) renderPromptHelpOverlay() string {
	keys := newPromptKeyMap()
	groups := keys.FullHelp()

	groupNames := []string{"Input", "Actions", "General"}

	headingStyle := lipgloss.NewStyle().Bold(true).Foreground(highlight)

	var b strings.Builder
	b.WriteString(titleStyle.Render(" New Task Help ") + "\n\n")

	for i, group := range groups {
		if i < len(groupNames) {
			b.WriteString("  " + headingStyle.Render(groupNames[i]) + "\n")
		}
		for _, binding := range group {
			fmt.Fprintf(&b, "    %-12s %s\n", dimStyle.Render(binding.Help().Key), helpStyle.Render(binding.Help().Desc))
		}
		b.WriteString("\n")
	}

	b.WriteString(dimStyle.Render("  Press ctrl+h or esc to close"))
	return b.String()
}

func Run(cfg *config.Config, projectID int64, projectPath, projectName string, globalMode bool, defaultWorktree bool) error {
	p := tea.NewProgram(
		NewModel(cfg, projectID, projectPath, projectName, globalMode, defaultWorktree),
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
