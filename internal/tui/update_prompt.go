package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

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
