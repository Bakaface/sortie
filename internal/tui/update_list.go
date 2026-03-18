package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle search mode input
	if m.searchMode {
		return m.handleSearchKey(msg)
	}

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

	// Handle init workflow selection if active
	if m.selectingInit {
		return m.handleInitSelectKey(msg)
	}

	// Handle continue workflow selection if active
	if m.selectingContinueWorkflow {
		return m.handleContinueWorkflowSelectKey(msg)
	}

	// Handle artifact selection if active
	if m.selectingArtifact {
		return m.handleArtifactSelectKey(msg)
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
			return m, m.loadOutput(task.ID)
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
				m.selectingTask = true
				m.taskCursor = 0
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
			if task.Status == "awaiting-approval" || task.Status == "artifact-missing" {
				m.confirmAction = "continue"
				m.confirmTaskID = task.ID
				return m, nil
			}
			if task.Status == "completed" || task.Status == "failed" {
				workflows := m.cfg.ListWorkflowNames()
				m.continueTaskID = task.ID
				m.selectingContinueWorkflow = true
				m.continueWorkflowCursor = 0
				// If only one workflow (default), skip selection and go to prompt
				if len(workflows) == 1 && workflows[0] == "default" {
					m.selectingContinueWorkflow = false
					m.continueSelectedWorkflow = "default"
					m.view = viewPrompt
					m.prompt.Reset()
					m.prompt.workflowName = "default"
					m.prompt.Focus()
					return m, nil
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
			m.selectingPriority = true
			m.priorityTaskID = task.ID
			m.priorityCursor = 0
			return m, nil
		}
		return m, nil

	case "i":
		// Show init workflow selection if init workflows are configured
		if m.client != nil && m.projectPath != "" {
			inits := m.cfg.ListInitWorkflowNames()
			if len(inits) > 0 {
				m.selectingInit = true
				m.initCursor = 0
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
			m.selectingWorkflow = true
			m.workflowCursor = 0
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
			m.selectingWorkflow = true
			m.workflowCursor = 0
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

func (m Model) handleWorkflowSelectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	workflows := m.cfg.ListWorkflowNames()
	keyStr := msg.String()

	// Handle "gg" sequence for go-to-top
	if keyStr == "g" {
		if m.workflowPendingG {
			m.workflowPendingG = false
			m.workflowCursor = 0
			return m, nil
		}
		m.workflowPendingG = true
		return m, nil
	}
	m.workflowPendingG = false

	switch keyStr {
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
	case "G":
		m.workflowCursor = max(0, len(workflows)-1)
		return m, nil
	case "ctrl+d", "pgdown":
		half := max(1, len(workflows)/2)
		m.workflowCursor = min(m.workflowCursor+half, len(workflows)-1)
		return m, nil
	case "ctrl+u", "pgup":
		half := max(1, len(workflows)/2)
		m.workflowCursor = max(m.workflowCursor-half, 0)
		return m, nil
	case "enter":
		m.selectedWorkflow = workflows[m.workflowCursor]
		m.selectingWorkflow = false
		m.view = viewPrompt
		m.prompt.Reset()
		m.prompt.workflowName = m.selectedWorkflow
		m.prompt.Focus()
		return m, nil
	case "esc", "q":
		m.selectingWorkflow = false
		return m, nil
	}

	// Number keys for quick selection (1-9)
	if len(keyStr) == 1 && keyStr[0] >= '1' && keyStr[0] <= '9' {
		idx := int(keyStr[0] - '1')
		if idx < len(workflows) {
			m.selectedWorkflow = workflows[idx]
			m.selectingWorkflow = false
			m.view = viewPrompt
			m.prompt.Reset()
			m.prompt.workflowName = m.selectedWorkflow
			m.prompt.Focus()
			return m, nil
		}
	}

	return m, nil
}

func (m Model) handleTaskSelectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	tasks := m.cfg.ListPredefinedTaskNames()
	keyStr := msg.String()

	// Handle "gg" sequence for go-to-top
	if keyStr == "g" {
		if m.taskPendingG {
			m.taskPendingG = false
			m.taskCursor = 0
			return m, nil
		}
		m.taskPendingG = true
		return m, nil
	}
	m.taskPendingG = false

	switch keyStr {
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
	case "G":
		m.taskCursor = max(0, len(tasks)-1)
		return m, nil
	case "ctrl+d", "pgdown":
		half := max(1, len(tasks)/2)
		m.taskCursor = min(m.taskCursor+half, len(tasks)-1)
		return m, nil
	case "ctrl+u", "pgup":
		half := max(1, len(tasks)/2)
		m.taskCursor = max(m.taskCursor-half, 0)
		return m, nil
	case "enter":
		taskName := tasks[m.taskCursor]
		taskCfg := m.cfg.GetPredefinedTask(taskName)
		m.selectingTask = false
		if taskCfg == nil {
			return m, nil
		}
		// Create task directly with the predefined description and workflow
		m.selectedWorkflow = "oneoff:" + taskCfg.Name
		description := taskCfg.Description
		if description == "" {
			description = taskCfg.Name
		}
		return m, m.createTaskWithPrompt(description, "", true, nil, "", "")
	case "esc", "q":
		m.selectingTask = false
		return m, nil
	}

	// Number keys for quick selection (1-9)
	if len(keyStr) == 1 && keyStr[0] >= '1' && keyStr[0] <= '9' {
		idx := int(keyStr[0] - '1')
		if idx < len(tasks) {
			taskName := tasks[idx]
			taskCfg := m.cfg.GetPredefinedTask(taskName)
			m.selectingTask = false
			if taskCfg == nil {
				return m, nil
			}
			m.selectedWorkflow = "oneoff:" + taskCfg.Name
			description := taskCfg.Description
			if description == "" {
				description = taskCfg.Name
			}
			return m, m.createTaskWithPrompt(description, "", true, nil, "", "")
		}
	}

	return m, nil
}

func (m Model) handlePrioritySelectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	priorities := []string{"low", "medium", "high", "urgent"}
	keyStr := msg.String()

	// Handle "gg" sequence for go-to-top
	if keyStr == "g" {
		if m.priorityPendingG {
			m.priorityPendingG = false
			m.priorityCursor = 0
			return m, nil
		}
		m.priorityPendingG = true
		return m, nil
	}
	m.priorityPendingG = false

	switch keyStr {
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
	case "G":
		m.priorityCursor = len(priorities) - 1
		return m, nil
	case "ctrl+d", "pgdown":
		half := max(1, len(priorities)/2)
		m.priorityCursor = min(m.priorityCursor+half, len(priorities)-1)
		return m, nil
	case "ctrl+u", "pgup":
		half := max(1, len(priorities)/2)
		m.priorityCursor = max(m.priorityCursor-half, 0)
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
	if len(keyStr) == 1 && keyStr[0] >= '1' && keyStr[0] <= '4' {
		idx := int(keyStr[0] - '1')
		selected := priorities[idx]
		m.selectingPriority = false
		return m, m.updateTaskPriority(m.priorityTaskID, selected)
	}

	return m, nil
}

func (m Model) handleInitSelectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	inits := m.cfg.ListInitWorkflowNames()
	keyStr := msg.String()

	// Handle "gg" sequence for go-to-top
	if keyStr == "g" {
		if m.initPendingG {
			m.initPendingG = false
			m.initCursor = 0
			return m, nil
		}
		m.initPendingG = true
		return m, nil
	}
	m.initPendingG = false

	switch keyStr {
	case "up", "k":
		if m.initCursor > 0 {
			m.initCursor--
		}
		return m, nil
	case "down", "j":
		if m.initCursor < len(inits)-1 {
			m.initCursor++
		}
		return m, nil
	case "G":
		m.initCursor = max(0, len(inits)-1)
		return m, nil
	case "ctrl+d", "pgdown":
		half := max(1, len(inits)/2)
		m.initCursor = min(m.initCursor+half, len(inits)-1)
		return m, nil
	case "ctrl+u", "pgup":
		half := max(1, len(inits)/2)
		m.initCursor = max(m.initCursor-half, 0)
		return m, nil
	case "enter":
		initName := inits[m.initCursor]
		initCfg := m.cfg.GetInitWorkflow(initName)
		m.selectingInit = false
		if initCfg == nil {
			return m, nil
		}
		// Create task directly with the init workflow description
		m.selectedWorkflow = "init:" + initCfg.Name
		description := initCfg.Description
		if description == "" {
			description = initCfg.Name
		}
		return m, m.createTaskWithPrompt(description, "", true, nil, "", "")
	case "esc", "q":
		m.selectingInit = false
		return m, nil
	}

	// Number keys for quick selection (1-9)
	if len(keyStr) == 1 && keyStr[0] >= '1' && keyStr[0] <= '9' {
		idx := int(keyStr[0] - '1')
		if idx < len(inits) {
			initName := inits[idx]
			initCfg := m.cfg.GetInitWorkflow(initName)
			m.selectingInit = false
			if initCfg == nil {
				return m, nil
			}
			m.selectedWorkflow = "init:" + initCfg.Name
			description := initCfg.Description
			if description == "" {
				description = initCfg.Name
			}
			return m, m.createTaskWithPrompt(description, "", true, nil, "", "")
		}
	}

	return m, nil
}

func (m Model) handleContinueWorkflowSelectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	workflows := m.cfg.ListWorkflowNames()
	keyStr := msg.String()

	// Handle "gg" sequence for go-to-top
	if keyStr == "g" {
		if m.continueWorkflowPendingG {
			m.continueWorkflowPendingG = false
			m.continueWorkflowCursor = 0
			return m, nil
		}
		m.continueWorkflowPendingG = true
		return m, nil
	}
	m.continueWorkflowPendingG = false

	switch keyStr {
	case "up", "k":
		if m.continueWorkflowCursor > 0 {
			m.continueWorkflowCursor--
		}
		return m, nil
	case "down", "j":
		if m.continueWorkflowCursor < len(workflows)-1 {
			m.continueWorkflowCursor++
		}
		return m, nil
	case "G":
		m.continueWorkflowCursor = max(0, len(workflows)-1)
		return m, nil
	case "ctrl+d", "pgdown":
		half := max(1, len(workflows)/2)
		m.continueWorkflowCursor = min(m.continueWorkflowCursor+half, len(workflows)-1)
		return m, nil
	case "ctrl+u", "pgup":
		half := max(1, len(workflows)/2)
		m.continueWorkflowCursor = max(m.continueWorkflowCursor-half, 0)
		return m, nil
	case "enter":
		workflow := workflows[m.continueWorkflowCursor]
		m.continueSelectedWorkflow = workflow
		m.selectingContinueWorkflow = false
		// Don't zero continueTaskID - prompt view needs it
		m.view = viewPrompt
		m.prompt.Reset()
		m.prompt.workflowName = workflow
		m.prompt.Focus()
		return m, nil
	case "esc", "q":
		m.selectingContinueWorkflow = false
		m.continueTaskID = 0
		return m, nil
	}

	// Number keys for quick selection (1-9)
	if len(keyStr) == 1 && keyStr[0] >= '1' && keyStr[0] <= '9' {
		idx := int(keyStr[0] - '1')
		if idx < len(workflows) {
			workflow := workflows[idx]
			m.continueSelectedWorkflow = workflow
			m.selectingContinueWorkflow = false
			// Don't zero continueTaskID - prompt view needs it
			m.view = viewPrompt
			m.prompt.Reset()
			m.prompt.workflowName = workflow
			m.prompt.Focus()
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
