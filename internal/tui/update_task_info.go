package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) handleTaskInfoKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	// Handle confirmation prompt if active
	if m.confirmAction != "" {
		return m.handleConfirmKey(msg)
	}

	// Handle selection dialog if active (must come before q/esc handling)
	if m.selector.IsActive() {
		return m.handleSelectorKey(msg)
	}

	// Handle chord sequences (gg, oa, ea, ed, et, ec, yd, yc)
	if ret, cmd, handled := m.tryChord(keyStr); handled {
		return ret, cmd
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
