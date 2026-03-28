package tui

import (
	"fmt"

	"github.com/aface/sortie/internal/daemon"
	tea "github.com/charmbracelet/bubbletea"
)

type stepContextsLoadedMsg struct {
	taskID   int64
	contexts map[string]string
	action   string
}

func (m Model) openArtifactSelection(task *daemon.TaskInfo, action string) (tea.Model, tea.Cmd) {
	if m.client == nil {
		m.err = fmt.Errorf("not connected to daemon")
		return m, nil
	}

	taskID := task.ID
	return m, func() tea.Msg {
		contexts, err := m.client.GetStepContexts(taskID)
		if err != nil {
			return errorMsg(fmt.Errorf("failed to get step contexts: %w", err))
		}
		if len(contexts) == 0 {
			return errorMsg(fmt.Errorf("no step contexts available for task #%d", taskID))
		}
		return stepContextsLoadedMsg{
			taskID:   taskID,
			contexts: contexts,
			action:   action,
		}
	}
}

func (m Model) handleArtifactSelectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	// Handle "gg" sequence for go-to-top
	if keyStr == "g" {
		if m.artifactPendingG {
			m.artifactPendingG = false
			m.artifactCursor = 0
			return m, nil
		}
		m.artifactPendingG = true
		return m, nil
	}
	m.artifactPendingG = false

	switch keyStr {
	case "up", "k":
		if m.artifactCursor > 0 {
			m.artifactCursor--
		}
		return m, nil
	case "down", "j":
		if m.artifactCursor < len(m.artifactNames)-1 {
			m.artifactCursor++
		}
		return m, nil
	case "G":
		m.artifactCursor = max(0, len(m.artifactNames)-1)
		return m, nil
	case "ctrl+d", "pgdown":
		half := max(1, len(m.artifactNames)/2)
		m.artifactCursor = min(m.artifactCursor+half, len(m.artifactNames)-1)
		return m, nil
	case "ctrl+u", "pgup":
		half := max(1, len(m.artifactNames)/2)
		m.artifactCursor = max(m.artifactCursor-half, 0)
		return m, nil
	case "enter":
		name := m.artifactNames[m.artifactCursor]
		m.selectingArtifact = false
		return m.performArtifactAction(name, m.artifactAction)
	case "esc", "q":
		m.selectingArtifact = false
		return m, nil
	}

	// Number keys for quick selection (1-9)
	if len(keyStr) == 1 && keyStr[0] >= '1' && keyStr[0] <= '9' {
		idx := int(keyStr[0] - '1')
		if idx < len(m.artifactNames) {
			name := m.artifactNames[idx]
			m.selectingArtifact = false
			return m.performArtifactAction(name, m.artifactAction)
		}
	}

	return m, nil
}

func (m Model) performArtifactAction(stepName, action string) (tea.Model, tea.Cmd) {
	if action == "edit" {
		// Editing step contexts directly is not supported (they come from Claude's output)
		m.err = fmt.Errorf("step contexts are read-only (captured from Claude output)")
		return m, nil
	}

	// View action: use pre-loaded context
	content, ok := m.stepContexts[stepName]
	if !ok || content == "" {
		m.err = fmt.Errorf("step context %q is empty", stepName)
		return m, nil
	}
	m.artifactView.SetContent(stepName, content)
	m.view = viewArtifact
	return m, nil
}

func (m Model) handleArtifactViewKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	switch keyStr {
	case "q", "esc":
		m.view = viewList
		return m, nil
	}

	// Handle "gg" sequence
	if keyStr == "g" {
		if m.artifactView.pendingG {
			m.artifactView.pendingG = false
			m.artifactView.GotoTop()
			return m, nil
		}
		m.artifactView.pendingG = true
		return m, nil
	}
	m.artifactView.pendingG = false

	switch keyStr {
	case "G":
		m.artifactView.GotoBottom()
		return m, nil
	case "j", "down":
		m.artifactView.ScrollDown()
		return m, nil
	case "k", "up":
		m.artifactView.ScrollUp()
		return m, nil
	case "ctrl+d", "pgdown":
		m.artifactView.PageDown()
		return m, nil
	case "ctrl+u", "pgup":
		m.artifactView.PageUp()
		return m, nil
	}

	return m, nil
}
