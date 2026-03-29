package tui

import (
	"fmt"

	"github.com/aface/sortie/internal/daemon"
	"github.com/charmbracelet/bubbles/key"
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
	ak := cachedArtifactViewKeyMap

	switch {
	case key.Matches(msg, ak.Back): // "q", "esc"
		m.view = viewList
		return m, nil
	}

	// Handle "gg" sequence
	if key.Matches(msg, ak.GotoTop) {
		if m.artifactView.pendingG {
			m.artifactView.pendingG = false
			m.artifactView.GotoTop()
			return m, nil
		}
		m.artifactView.pendingG = true
		return m, nil
	}
	m.artifactView.pendingG = false

	switch {
	case key.Matches(msg, ak.GotoBottom): // "G"
		m.artifactView.GotoBottom()
		return m, nil
	case key.Matches(msg, ak.Down): // "j", "down"
		m.artifactView.ScrollDown()
		return m, nil
	case key.Matches(msg, ak.Up): // "k", "up"
		m.artifactView.ScrollUp()
		return m, nil
	case key.Matches(msg, ak.HalfDown): // "ctrl+d", "pgdown"
		m.artifactView.PageDown()
		return m, nil
	case key.Matches(msg, ak.HalfUp): // "ctrl+u", "pgup"
		m.artifactView.PageUp()
		return m, nil
	}

	return m, nil
}
