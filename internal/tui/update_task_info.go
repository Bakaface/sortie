package tui

import (
	"fmt"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) handleTaskInfoKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	// Handle confirmation prompt if active
	if m.confirmAction != "" {
		return m.handleConfirmKey(msg)
	}

	// Handle artifact selection if active (must come before q/esc handling)
	if m.selectingArtifact {
		return m.handleArtifactSelectKey(msg)
	}

	// Handle second key after "o" prefix (must come before single-key handlers)
	if m.pendingO {
		m.pendingO = false
		if keyStr == "a" {
			if m.taskInfo.task != nil {
				return m.openArtifactSelection(m.taskInfo.task, "view")
			}
		}
		// Fall through to handle this key normally
	}

	// Handle second key after "e" prefix (must come before single-key handlers)
	if m.pendingE {
		m.pendingE = false
		switch keyStr {
		case "a":
			if m.taskInfo.task != nil {
				return m.openArtifactSelection(m.taskInfo.task, "edit")
			}
		case "d":
			if m.taskInfo.task != nil {
				return m, m.openEditorForField(m.taskInfo.task.ID, "description", m.taskInfo.task.Description)
			}
		case "t":
			if m.taskInfo.task != nil {
				return m, m.openEditorForField(m.taskInfo.task.ID, "title", m.taskInfo.task.Title)
			}
		case "c":
			if m.taskInfo.task != nil {
				return m, m.openEditorForField(m.taskInfo.task.ID, "context", m.taskInfo.task.Context)
			}
		}
		// Fall through to handle this key normally
	}

	switch keyStr {
	case "q", "esc":
		m.view = viewList
		return m, nil
	case "ctrl+c":
		if m.taskInfo.task != nil && m.client != nil {
			m.confirmAction = "stop"
			m.confirmTaskID = m.taskInfo.task.ID
			return m, nil
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
			return m, m.loadOutput(m.taskInfo.task.ID, 0)
		}
		return m, nil
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
				} else {
					m.statusMessage = "Copied description to clipboard"
					m.statusMessageTTL = 2
				}
			}
			return m, nil
		case "c":
			if m.taskInfo.task != nil && m.taskInfo.task.Context != "" {
				if err := clipboard.WriteAll(m.taskInfo.task.Context); err != nil {
					m.err = fmt.Errorf("clipboard: %w", err)
				} else {
					m.statusMessage = "Copied context to clipboard"
					m.statusMessageTTL = 2
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
