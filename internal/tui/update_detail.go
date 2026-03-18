package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	// Handle confirmation prompt if active
	if m.confirmAction != "" {
		return m.handleConfirmKey(msg)
	}

	// Common keys (both modes)
	switch keyStr {
	case "q":
		m.view = viewList
		return m, nil
	case "ctrl+c":
		if m.detail.task != nil && m.client != nil {
			m.confirmAction = "stop"
			m.confirmTaskID = m.detail.task.ID
			return m, nil
		}
		return m, nil
	case "t":
		if m.detail.task != nil {
			return m, m.attachTmuxSession(m.detail.task.ID)
		}
		return m, nil
	case "e":
		if m.detail.task != nil {
			return m, m.openLogInEditor(m.detail.task)
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
