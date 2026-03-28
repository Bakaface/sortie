package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/aface/sortie/internal/daemon"
	"github.com/aface/sortie/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) openArtifactSelection(task *daemon.TaskInfo, action string) (tea.Model, tea.Cmd) {
	if task.WorktreePath == "" {
		m.err = fmt.Errorf("no worktree available for task #%d", task.ID)
		return m, nil
	}

	wf := m.cfg.GetWorkflow(task.Workflow)
	if wf == nil {
		m.err = fmt.Errorf("no workflow found for task #%d", task.ID)
		return m, nil
	}

	// Find steps that have actual artifact files on disk
	artifactsDir := workflow.ArtifactsDir(task.WorktreePath)
	var names []string
	for _, step := range wf.Steps {
		path := filepath.Join(artifactsDir, step.Name+".md")
		if _, err := os.Stat(path); err == nil {
			names = append(names, step.Name)
		}
	}

	if len(names) == 0 {
		m.err = fmt.Errorf("no artifacts available for task #%d", task.ID)
		return m, nil
	}

	// If only one artifact, skip selection
	if len(names) == 1 {
		return m.performArtifactAction(task.WorktreePath, names[0], action)
	}

	m.selectingArtifact = true
	m.artifactCursor = 0
	m.artifactNames = names
	m.artifactTaskID = task.ID
	m.artifactWorktree = task.WorktreePath
	m.artifactAction = action
	return m, nil
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
		return m.performArtifactAction(m.artifactWorktree, name, m.artifactAction)
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
			return m.performArtifactAction(m.artifactWorktree, name, m.artifactAction)
		}
	}

	return m, nil
}

func (m Model) performArtifactAction(worktreePath, stepName, action string) (tea.Model, tea.Cmd) {
	artifactPath := filepath.Join(workflow.ArtifactsDir(worktreePath), stepName+".md")

	if action == "edit" {
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vi"
		}
		c := exec.Command(editor, artifactPath)
		return m, tea.ExecProcess(c, func(err error) tea.Msg {
			if err != nil {
				return errorMsg(fmt.Errorf("editor exited with error: %w", err))
			}
			return editorArtifactFinishedMsg{}
		})
	}

	// View action: read content and display
	return m, func() tea.Msg {
		content, err := workflow.ReadArtifact(worktreePath, stepName)
		if err != nil {
			return errorMsg(fmt.Errorf("failed to read artifact: %w", err))
		}
		if content == "" {
			return errorMsg(fmt.Errorf("artifact %q is empty", stepName))
		}
		return artifactLoadedMsg{name: stepName, content: content}
	}
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
