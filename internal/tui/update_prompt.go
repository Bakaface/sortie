package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) handlePromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	// When help overlay is showing, only allow closing it
	if m.prompt.showHelp {
		if keyStr == "ctrl+h" || keyStr == "esc" {
			m.prompt.showHelp = false
			return m, nil
		}
		return m, nil
	}

	switch keyStr {
	case "ctrl+h":
		m.prompt.showHelp = true
		return m, nil

	case "enter":
		description := m.prompt.Value()
		// Continue mode: send continue request with prompt
		if m.continueTaskID != 0 && m.continueSelectedWorkflow != "" {
			taskID := m.continueTaskID
			workflow := m.continueSelectedWorkflow
			m.continueTaskID = 0
			m.continueSelectedWorkflow = ""
			m.view = viewList
			return m, m.continueTask(taskID, workflow, description)
		}
		// New task mode: create task with prompt
		checkoutBranch := m.prompt.CheckoutBranch()
		// Allow empty description only when using existing branch mode
		if description == "" && checkoutBranch == "" {
			return m, nil
		}
		title := m.prompt.TitleValue()
		images := m.prompt.Images()
		branchName := m.prompt.BranchName()
		targetBranch := m.prompt.TargetBranch()
		worktree := m.prompt.Worktree()
		m.view = viewList
		return m, m.createTaskWithPrompt(title, description, branchName, worktree, images, targetBranch, checkoutBranch)

	case "tab", "ctrl+n":
		// Switch focus to next field
		m.prompt.SwitchFocus(true)
		return m, nil

	case "shift+tab", "ctrl+p":
		// Switch focus to previous field
		m.prompt.SwitchFocus(false)
		return m, nil

	case "esc":
		// Cancel and return to list
		m.continueTaskID = 0
		m.continueSelectedWorkflow = ""
		m.blockingTaskID = 0
		m.view = viewList
		return m, nil

	case "ctrl+g":
		// Open $EDITOR for prompt editing (only from description field)
		if m.prompt.focusField == promptFieldDescription {
			return m, m.openEditorForPrompt()
		}
		cmd := m.prompt.Update(msg)
		return m, cmd

	case "alt+w":
		// Toggle worktree mode
		m.prompt.ToggleWorktree()
		return m, nil

	case "alt+m":
		// Toggle branch mode (only when worktree is on)
		if m.prompt.worktree {
			m.prompt.ToggleBranchMode()
		}
		return m, nil

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
