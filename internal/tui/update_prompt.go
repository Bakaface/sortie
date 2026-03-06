package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) handlePromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	switch keyStr {
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
		if description == "" {
			return m, nil
		}
		images := m.prompt.Images()
		branchName := m.prompt.BranchName()
		m.view = viewList
		return m, m.createTaskWithPrompt(description, branchName, images)

	case "tab":
		// Switch focus between description and branch name
		m.prompt.SwitchFocus()
		return m, nil

	case "esc":
		// Cancel and return to list
		m.continueTaskID = 0
		m.continueSelectedWorkflow = ""
		m.view = viewList
		return m, nil

	case "ctrl+g":
		// Open $EDITOR for prompt editing (only from description field)
		if m.prompt.focusField == promptFieldDescription {
			return m, m.openEditorForPrompt()
		}
		cmd := m.prompt.Update(msg)
		return m, cmd

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
