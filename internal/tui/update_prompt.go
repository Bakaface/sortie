package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) handlePromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	pk := cachedPromptKeyMap

	// Handle pending ctrl+w pane switch chord
	if m.prompt.pendingPaneSwitch {
		m.prompt.pendingPaneSwitch = false
		switch msg.String() {
		case "h":
			m.prompt.activePane = paneTask
			m.prompt.Focus()
		case "l":
			if len(m.prompt.workflows) > 1 {
				m.prompt.activePane = paneWorkflow
				m.prompt.Blur()
			}
		}
		// Any other key: chord cancelled, do nothing
		return m, nil
	}

	// When workflow pane is focused, handle its keys
	if m.prompt.activePane == paneWorkflow {
		return m.handleWorkflowPaneKey(msg)
	}

	// When help overlay is showing, only allow closing it
	if m.prompt.showHelp {
		if key.Matches(msg, pk.Help) || key.Matches(msg, pk.Cancel) {
			m.prompt.showHelp = false
			return m, nil
		}
		return m, nil
	}

	switch {
	case key.Matches(msg, pk.Help):
		m.prompt.showHelp = true
		return m, nil

	case key.Matches(msg, pk.Submit): // "enter"
		description := m.prompt.Value()
		// Continue mode: send continue request with prompt
		if m.continueTaskID != 0 && m.continueSelectedWorkflow != "" {
			taskID := m.continueTaskID
			workflow := m.continueSelectedWorkflow
			m.continueTaskID = 0
			m.continueSelectedWorkflow = ""
			deferred := m.continueTask(taskID, workflow, description)
			if m.animationEnabled() {
				positions := m.planePositions(description)
				m.sortie = newSortieAnimation(positions, m.width, m.height, m.animationDuration())
				m.sortieCmd = deferred
				m.view = viewSortie
				return m, sortieTickCmd()
			}
			m.view = viewList
			return m, deferred
		}
		// New task mode: create task with prompt
		checkoutBranch := m.prompt.CheckoutBranch()
		// Allow empty description only when using existing branch mode
		if description == "" && checkoutBranch == "" {
			m.prompt.validationError = "description required"
			return m, nil
		}
		title := m.prompt.TitleValue()
		images := m.prompt.Images()
		branchName := m.prompt.BranchName()
		targetBranch := m.prompt.TargetBranch()
		worktree := m.prompt.Worktree()
		deferred := m.createTaskWithPrompt(title, description, branchName, worktree, images, targetBranch, checkoutBranch)
		if m.animationEnabled() {
			positions := m.planePositions(description)
			m.sortie = newSortieAnimation(positions, m.width, m.height, m.animationDuration())
			m.sortieCmd = deferred
			m.view = viewSortie
			return m, sortieTickCmd()
		}
		m.view = viewList
		return m, deferred

	case key.Matches(msg, pk.SwitchField): // "tab", "ctrl+n"
		// Switch focus to next field
		m.prompt.SwitchFocus(true)
		return m, nil

	case key.Matches(msg, pk.SwitchFieldPrev): // "shift+tab", "ctrl+p"
		// Switch focus to previous field
		m.prompt.SwitchFocus(false)
		return m, nil

	case key.Matches(msg, pk.Cancel): // "esc"
		// Cancel and return to list
		m.continueTaskID = 0
		m.continueSelectedWorkflow = ""
		m.blockingTaskID = 0
		m.view = viewList
		return m, nil

	case key.Matches(msg, pk.Editor): // "ctrl+g"
		// Open $EDITOR for prompt editing (only from description field)
		if m.prompt.focusField == promptFieldDescription {
			return m, m.openEditorForPrompt()
		}
		cmd := m.prompt.Update(msg)
		return m, cmd

	case key.Matches(msg, pk.Worktree): // "alt+w"
		// Toggle worktree mode
		m.prompt.ToggleWorktree()
		return m, nil

	case key.Matches(msg, pk.BranchMode): // "alt+m"
		// Toggle branch mode (only when worktree is on)
		if m.prompt.worktree {
			m.prompt.ToggleBranchMode()
		}
		return m, nil

	case key.Matches(msg, pk.PaneSwitch): // "ctrl+w"
		m.prompt.pendingPaneSwitch = true
		return m, nil

	case key.Matches(msg, pk.RemoveImage): // "ctrl+x"
		// Remove last image
		m.prompt.RemoveLastImage()
		return m, nil

	default:
		// Clear validation error on typing
		m.prompt.validationError = ""
		// Pass all other keys to the prompt view
		cmd := m.prompt.Update(msg)
		return m, cmd
	}
}

// animationEnabled returns true if the sortie animation is configured on.
// Enabled by default; can be disabled via options.animation.enabled: false in config.
func (m Model) animationEnabled() bool {
	if m.width == 0 || m.height == 0 {
		return false
	}
	if m.cfg == nil || m.cfg.Options.Animation == nil || m.cfg.Options.Animation.Enabled == nil {
		return false
	}
	return *m.cfg.Options.Animation.Enabled
}

// animationDuration returns the configured animation duration in milliseconds,
// defaulting to 1500ms.
func (m Model) animationDuration() int {
	if m.cfg != nil && m.cfg.Options.Animation != nil && m.cfg.Options.Animation.Duration != nil {
		d := *m.cfg.Options.Animation.Duration
		if d > 0 {
			return d
		}
	}
	return 1500
}

// planePositions returns the screen coordinates of each ✈ prompt prefix
// in the prompt view, so the animation can keep planes in their exact spots.
// Layout: title(1) + blank(1) + "Title: " input(1) + blank(1) = row 4 is the first textarea line.
// Each textarea line's ✈ sits at column 4 (2 padding + prompt char position).
// Soft-wrapped continuation lines also get a plane.
func (m Model) planePositions(description string) [][2]int {
	const (
		textareaStartRow = 4 // rows above the textarea in prompt view
		planeCol         = 2 // left padding where ✈ renders
		maxPlanes        = 12
	)

	if description == "" {
		return [][2]int{{planeCol, textareaStartRow}}
	}

	// Calculate content width matching promptView.visualLineCount() logic
	promptWidth := lipgloss.Width(PromptPrefix)
	contentWidth := (m.width - 4) - promptWidth
	if contentWidth < 1 {
		contentWidth = 1
	}

	var positions [][2]int
	row := textareaStartRow
	for _, line := range strings.Split(description, "\n") {
		lineWidth := lipgloss.Width(line)
		if lineWidth == 0 {
			positions = append(positions, [2]int{planeCol, row})
			row++
		} else {
			visualLines := (lineWidth + contentWidth - 1) / contentWidth
			for v := 0; v < visualLines; v++ {
				positions = append(positions, [2]int{planeCol, row})
				row++
				if len(positions) >= maxPlanes {
					return positions
				}
			}
		}
	}

	if len(positions) == 0 {
		positions = append(positions, [2]int{planeCol, textareaStartRow})
	}
	return positions
}

func (m Model) handleWorkflowPaneKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	pk := cachedPromptKeyMap

	switch {
	case key.Matches(msg, pk.PaneSwitch):
		m.prompt.pendingPaneSwitch = true
		return m, nil

	case key.Matches(msg, pk.Cancel):
		// Esc cancels prompt entirely (same as task pane)
		m.continueTaskID = 0
		m.continueSelectedWorkflow = ""
		m.blockingTaskID = 0
		m.view = viewList
		return m, nil

	case key.Matches(msg, pk.Submit):
		// Enter from workflow pane: switch back to task pane (selection is implicit)
		m.prompt.activePane = paneTask
		m.prompt.Focus()
		return m, nil

	case key.Matches(msg, pk.SwitchField), key.Matches(msg, pk.SwitchFieldPrev):
		// Tab/shift-tab: switch back to task pane
		m.prompt.activePane = paneTask
		m.prompt.Focus()
		return m, nil

	case key.Matches(msg, pk.Help):
		m.prompt.showHelp = true
		return m, nil
	}

	switch msg.String() {
	case "j", "down":
		if m.prompt.workflowCursor < len(m.prompt.workflows)-1 {
			m.prompt.workflowCursor++
			m.prompt.workflowName = m.prompt.workflows[m.prompt.workflowCursor]
			m.selectedWorkflow = m.prompt.workflowName
		}
	case "k", "up":
		if m.prompt.workflowCursor > 0 {
			m.prompt.workflowCursor--
			m.prompt.workflowName = m.prompt.workflows[m.prompt.workflowCursor]
			m.selectedWorkflow = m.prompt.workflowName
		}
	case "G":
		m.prompt.workflowCursor = max(0, len(m.prompt.workflows)-1)
		m.prompt.workflowName = m.prompt.workflows[m.prompt.workflowCursor]
		m.selectedWorkflow = m.prompt.workflowName
	}

	return m, nil
}
