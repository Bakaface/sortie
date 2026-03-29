package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// deleteWordBackward removes the last word from s, mimicking ctrl+backspace behavior.
// It first strips trailing whitespace, then removes non-whitespace characters.
func deleteWordBackward(s string) string {
	// Trim trailing spaces
	trimmed := strings.TrimRight(s, " ")
	if trimmed == "" {
		return ""
	}
	// Find the last space in the trimmed string
	lastSpace := strings.LastIndex(trimmed, " ")
	if lastSpace == -1 {
		return ""
	}
	return trimmed[:lastSpace+1]
}

func (m Model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle search mode input
	if m.searchMode {
		return m.handleSearchKey(msg)
	}

	// Handle command mode input
	if m.commandMode {
		return m.handleCommandKey(msg)
	}

	// Handle generic selection dialog if active
	if m.selector.IsActive() {
		return m.handleSelectorKey(msg)
	}

	// Handle branch selection if active
	if m.selectingBranch {
		return m.handleBranchSelectKey(msg)
	}

	// Handle confirmation prompt if active
	if m.confirmAction != "" {
		return m.handleConfirmKey(msg)
	}

	keyStr := msg.String()

	// Handle help overlay — consume all keys except ctrl+h and esc which dismiss it
	if m.list.showHelp {
		if keyStr == "ctrl+h" || keyStr == "esc" {
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
		switch keyStr {
		case "a":
			if task := m.list.Selected(); task != nil {
				return m.openArtifactSelection(task, "edit")
			}
		case "d":
			if task := m.list.Selected(); task != nil {
				return m, m.openEditorForField(task.ID, "description", task.Description)
			}
		case "t":
			if task := m.list.Selected(); task != nil {
				return m, m.openEditorForField(task.ID, "title", task.Title)
			}
		case "c":
			if task := m.list.Selected(); task != nil {
				return m, m.openEditorForField(task.ID, "context", task.Context)
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
			return m, m.loadOutput(task.ID, 0)
		}
		return m, nil

	case "r":
		// Retry if selected task is failed, completed, or stale tmux
		if task := m.list.Selected(); task != nil && m.client != nil {
			if task.Status == "failed" || task.Status == "completed" || task.Status == "tmux" {
				return m, m.retryTask(task.ID)
			}
		}

	case "x":
		// Show predefined task selection if tasks are configured
		if m.client != nil && m.projectPath != "" {
			tasks := m.cfg.ListPredefinedTaskNames()
			if len(tasks) > 0 {
				var descs []string
				for _, name := range tasks {
					if tc := m.cfg.GetPredefinedTask(name); tc != nil {
						descs = append(descs, tc.Description)
					} else {
						descs = append(descs, "")
					}
				}
				m.selector = selector{
					kind:         selectorTask,
					title:        "Run Predefined Task",
					items:        tasks,
					descriptions: descs,
				}
				return m, nil
			}
		}

	case "R":
		if task := m.list.Selected(); task != nil && m.client != nil {
			if task.Status == "completed" || task.Status == "failed" {
				m.confirmAction = "revert"
				m.confirmTaskID = task.ID
				return m, nil
			}
		}
		return m, nil

	case "s":
		if task := m.list.Selected(); task != nil && m.client != nil {
			m.confirmAction = "stop"
			m.confirmTaskID = task.ID
			return m, nil
		}
		return m, nil

	case "t":
		if task := m.list.Selected(); task != nil {
			return m, m.attachTmuxSession(task.ID)
		}
		return m, nil

	case "c":
		if task := m.list.Selected(); task != nil && m.client != nil {
			if task.Status == "awaiting-approval" {
				m.confirmAction = "continue"
				m.confirmTaskID = task.ID
				return m, nil
			}
			if task.Status == "completed" || task.Status == "failed" {
				workflows := m.cfg.ListWorkflowNames()
				m.continueTaskID = task.ID
				// If only one workflow (default), skip selection and go to prompt
				if len(workflows) == 1 && workflows[0] == "default" {
					m.continueSelectedWorkflow = "default"
					m.view = viewPrompt
					m.prompt.Reset()
					m.prompt.workflowName = "default"
					m.prompt.Focus()
					return m, nil
				}
				m.selector = selector{
					kind:  selectorContinueWorkflow,
					title: fmt.Sprintf("Continue Task #%d - Select Workflow", task.ID),
					items: workflows,
				}
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
			m.selector = selector{
				kind:      selectorPriority,
				title:     "Select Priority",
				items:     []string{"low", "medium", "high", "urgent"},
				itemStyle: func(name string) lipgloss.Style { return priorityStyle(name) },
				hint:      "j/k: navigate | enter: select | 1-4: quick select | esc: cancel",
				taskID:    task.ID,
			}
			return m, nil
		}
		return m, nil

	case "i":
		// Show init workflow selection if init workflows are configured
		if m.client != nil && m.projectPath != "" {
			inits := m.cfg.ListInitWorkflowNames()
			if len(inits) > 0 {
				var descs []string
				for _, name := range inits {
					if ic := m.cfg.GetInitWorkflow(name); ic != nil {
						descs = append(descs, ic.Description)
					} else {
						descs = append(descs, "")
					}
				}
				m.selector = selector{
					kind:         selectorInit,
					title:        "Run Init Workflow",
					items:        inits,
					descriptions: descs,
				}
				return m, nil
			}
		}
		return m, nil

	case "n":
		// If search is active, "n" navigates to next match
		if len(m.list.matchedIndices) > 0 {
			m.list.nextMatch(m.searchDirection)
			return m, nil
		}
		// Otherwise, "n" is new task
		if m.client == nil || m.projectPath == "" {
			return m, nil
		}
		workflows := m.cfg.ListWorkflowNames()
		if len(workflows) > 1 {
			m.selector = selector{
				kind:  selectorWorkflow,
				title: "Select Workflow",
				items: workflows,
			}
			return m, nil
		}
		// Single workflow (or default) — skip selection and open prompt view
		m.selectedWorkflow = ""
		m.view = viewPrompt
		m.prompt.Reset()
		m.prompt.workflowName = ""
		m.prompt.Focus()
		return m, nil

	case "N":
		// Previous match (opposite direction) if search is active
		if len(m.list.matchedIndices) > 0 {
			m.list.previousMatch(m.searchDirection)
			return m, nil
		}
		// Otherwise, create a new task that blocks the selected task
		if m.client == nil || m.projectPath == "" {
			return m, nil
		}
		task := m.list.Selected()
		if task == nil {
			return m, nil
		}
		m.blockingTaskID = task.ID
		workflows := m.cfg.ListWorkflowNames()
		if len(workflows) > 1 {
			m.selector = selector{
				kind:  selectorWorkflow,
				title: "Select Workflow",
				items: workflows,
			}
			return m, nil
		}
		m.selectedWorkflow = ""
		m.view = viewPrompt
		m.prompt.Reset()
		m.prompt.workflowName = ""
		m.prompt.blockingTaskID = m.blockingTaskID
		m.prompt.Focus()
		return m, nil

	case "D":
		if task := m.list.Selected(); task != nil && m.client != nil {
			if task.Worktree && task.WorktreePath != "" && !task.WorktreeDetached {
				return m, m.detachBranch(task.ID)
			}
		}
		return m, nil

	case "A":
		if task := m.list.Selected(); task != nil && m.client != nil {
			if task.Worktree && task.WorktreePath != "" && task.WorktreeDetached {
				return m, m.attachBranch(task.ID)
			}
		}
		return m, nil

	case "b":
		if m.client != nil && m.projectPath != "" {
			return m, m.loadLocalBranches()
		}
		return m, nil

	case "ctrl+h":
		m.list.showHelp = !m.list.showHelp
		return m, nil

	case "/":
		m.searchMode = true
		m.searchQuery = ""
		m.searchDirection = 1 // forward search
		return m, nil

	case "?":
		m.searchMode = true
		m.searchQuery = ""
		m.searchDirection = -1 // backward search
		return m, nil

	case ":":
		m.commandMode = true
		m.commandInput = ""
		return m, nil

	case "esc":
		// Clear search highlights (like :noh in vim)
		m.list.matchedIndices = nil
		m.list.currentMatchIdx = 0
		m.searchQuery = ""
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

	case "tab":
		if completed, ok := completeRunTask(m, m.commandInput); ok {
			m.commandInput = completed
		}
		return m, nil

	case "ctrl+w":
		m.commandInput = deleteWordBackward(m.commandInput)
		if m.commandInput == "" {
			m.commandMode = false
		}
		return m, nil

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

func (m Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	switch keyStr {
	case "esc":
		m.searchMode = false
		m.searchQuery = ""
		return m, nil

	case "enter":
		query := m.searchQuery
		direction := m.searchDirection
		m.searchMode = false
		if query != "" {
			m.list.performSearchAndJump(query, m.list.table.Cursor(), direction)
		}
		return m, nil

	case "ctrl+w":
		m.searchQuery = deleteWordBackward(m.searchQuery)
		if m.searchQuery == "" {
			m.searchMode = false
		}
		return m, nil

	case "backspace":
		if len(m.searchQuery) > 0 {
			m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
		}
		if m.searchQuery == "" {
			m.searchMode = false
		}
		return m, nil

	default:
		// Only accept printable characters
		if len(keyStr) == 1 && keyStr[0] >= ' ' && keyStr[0] <= '~' {
			m.searchQuery += keyStr
		}
		return m, nil
	}
}

// handleSelectorKey dispatches key events to the generic selector and handles the result.
func (m Model) handleSelectorKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	result := m.selector.HandleKey(msg.String())
	switch result {
	case selChosen:
		return m.handleSelectorChoice()
	case selCancelled:
		return m.handleSelectorCancel()
	}
	return m, nil
}

// handleSelectorChoice dispatches the selected item based on selector kind.
func (m Model) handleSelectorChoice() (tea.Model, tea.Cmd) {
	item := m.selector.Selected()
	kind := m.selector.kind
	taskID := m.selector.taskID
	action := m.selector.action
	m.selector.Reset()

	switch kind {
	case selectorWorkflow:
		m.selectedWorkflow = item
		m.view = viewPrompt
		m.prompt.Reset()
		m.prompt.workflowName = item
		m.prompt.Focus()
		return m, nil

	case selectorContinueWorkflow:
		m.continueSelectedWorkflow = item
		m.view = viewPrompt
		m.prompt.Reset()
		m.prompt.workflowName = item
		m.prompt.Focus()
		return m, nil

	case selectorTask:
		taskCfg := m.cfg.GetPredefinedTask(item)
		if taskCfg == nil {
			return m, nil
		}
		m.selectedWorkflow = "oneoff:" + taskCfg.Name
		description := taskCfg.Description
		if description == "" {
			description = taskCfg.Name
		}
		return m, m.createTaskWithPrompt("", description, "", true, nil, "", "")

	case selectorInit:
		initCfg := m.cfg.GetInitWorkflow(item)
		if initCfg == nil {
			return m, nil
		}
		m.selectedWorkflow = "init:" + initCfg.Name
		description := initCfg.Description
		if description == "" {
			description = initCfg.Name
		}
		return m, m.createTaskWithPrompt("", description, "", true, nil, "", "")

	case selectorPriority:
		return m, m.updateTaskPriority(taskID, item)

	case selectorArtifact:
		return m.performArtifactAction(item, action)
	}

	return m, nil
}

// handleSelectorCancel handles cleanup when a selection is cancelled.
func (m Model) handleSelectorCancel() (tea.Model, tea.Cmd) {
	kind := m.selector.kind
	m.selector.Reset()

	if kind == selectorContinueWorkflow {
		m.continueTaskID = 0
	}
	return m, nil
}

func (m Model) handleBranchSelectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	// Handle "gg" sequence for go-to-top
	if keyStr == "g" {
		if m.branchPendingG {
			m.branchPendingG = false
			m.branchCursor = 0
			return m, nil
		}
		m.branchPendingG = true
		return m, nil
	}
	m.branchPendingG = false

	switch keyStr {
	case "up", "ctrl+k", "ctrl+p":
		if m.branchCursor > 0 {
			m.branchCursor--
		}
		return m, nil
	case "down", "ctrl+j", "ctrl+n":
		if m.branchCursor < len(m.branchFiltered)-1 {
			m.branchCursor++
		}
		return m, nil
	case "G":
		m.branchCursor = max(0, len(m.branchFiltered)-1)
		return m, nil
	case "ctrl+d", "pgdown":
		half := max(1, len(m.branchFiltered)/2)
		m.branchCursor = min(m.branchCursor+half, len(m.branchFiltered)-1)
		return m, nil
	case "ctrl+u", "pgup":
		half := max(1, len(m.branchFiltered)/2)
		m.branchCursor = max(m.branchCursor-half, 0)
		return m, nil
	case "enter":
		if len(m.branchFiltered) > 0 {
			branch := m.branchFiltered[m.branchCursor]
			m.selectingBranch = false
			m.branchFilter = ""
			return m, m.createBranchTask(branch)
		}
		return m, nil
	case "esc":
		m.selectingBranch = false
		m.branchFilter = ""
		return m, nil
	case "backspace":
		if len(m.branchFilter) > 0 {
			m.branchFilter = m.branchFilter[:len(m.branchFilter)-1]
			m.branchFiltered = fuzzyFilterBranches(m.branchList, m.branchFilter)
			m.branchCursor = 0
		}
		return m, nil
	default:
		// Accept printable characters for fuzzy search
		if len(keyStr) == 1 && keyStr[0] >= ' ' && keyStr[0] <= '~' {
			m.branchFilter += keyStr
			m.branchFiltered = fuzzyFilterBranches(m.branchList, m.branchFilter)
			m.branchCursor = 0
			return m, nil
		}
	}

	return m, nil
}

// handleConfirmKey handles y/n/esc keys for the shared confirmation dialog.
func (m Model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		action := m.confirmAction
		taskID := m.confirmTaskID
		m.confirmAction = ""
		m.confirmTaskID = 0
		switch action {
		case "continue":
			return m, m.continueTask(taskID, "", "")
		case "finalize":
			return m, m.finalizeTask(taskID)
		case "delete":
			return m, m.deleteTask(taskID)
		case "revert":
			return m, m.revertTask(taskID)
		case "stop":
			return m, m.stopTask(taskID)
		default:
			return m, nil
		}
	case "n", "esc":
		m.confirmAction = ""
		m.confirmTaskID = 0
		return m, nil
	default:
		return m, nil
	}
}
