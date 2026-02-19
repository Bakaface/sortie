package tui

import (
	"fmt"
	"strings"

	"github.com/aface/ralph-tamer-kit/internal/daemon"
	"github.com/charmbracelet/lipgloss"
)

type listView struct {
	tasks        []daemon.TaskInfo
	cursor       int
	width        int
	height       int
	showHelp     bool
	globalMode   bool
	tmuxSessions map[int64]bool
}

func newListView(globalMode bool) listView {
	return listView{
		tasks:      make([]daemon.TaskInfo, 0),
		cursor:     0,
		showHelp:   false,
		globalMode: globalMode,
	}
}

func (l *listView) SetTasks(tasks []daemon.TaskInfo) {
	l.tasks = tasks
	if l.cursor >= len(tasks) {
		l.cursor = max(0, len(tasks)-1)
	}
}

func (l *listView) UpdateTask(task daemon.TaskInfo) {
	for i, t := range l.tasks {
		if t.ID == task.ID {
			l.tasks[i] = task
			return
		}
	}
	l.tasks = append(l.tasks, task)
}

func (l *listView) RemoveTask(id int64) {
	for i, t := range l.tasks {
		if t.ID == id {
			l.tasks = append(l.tasks[:i], l.tasks[i+1:]...)
			if l.cursor >= len(l.tasks) {
				l.cursor = max(0, len(l.tasks)-1)
			}
			return
		}
	}
}

func (l *listView) Selected() *daemon.TaskInfo {
	if l.cursor >= 0 && l.cursor < len(l.tasks) {
		return &l.tasks[l.cursor]
	}
	return nil
}

func (l *listView) MoveUp() {
	if l.cursor > 0 {
		l.cursor--
	}
}

func (l *listView) MoveDown() {
	if l.cursor < len(l.tasks)-1 {
		l.cursor++
	}
}

func (l *listView) SetSize(width, height int) {
	l.width = width
	l.height = height
}

func (l *listView) View() string {
	var b strings.Builder

	var titleText string
	if l.globalMode {
		titleText = " Ralph Tamer Kit (Global) "
	} else {
		titleText = " Ralph Tamer Kit "
	}
	title := titleStyle.Render(titleText)
	b.WriteString(title)
	b.WriteString("\n\n")

	if len(l.tasks) == 0 {
		b.WriteString(dimStyle.Render("  No tasks found. Use 'ralph-tamer-kit plan <PRD.md>' to create tasks."))
		b.WriteString("\n")
	} else {
		var header string
		if l.globalMode {
			header = fmt.Sprintf("  %-5s %-14s %-22s %s",
				"ID", "PROJECT", "STATUS", "TITLE")
		} else {
			header = fmt.Sprintf("  %-5s %-22s %s",
				"ID", "STATUS", "TITLE")
		}
		b.WriteString(headerStyle.Render(header))
		b.WriteString("\n")

		for i, task := range l.tasks {
			line := l.renderTask(task, i == l.cursor)
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(l.renderHelp())

	return b.String()
}

func (l *listView) renderTask(task daemon.TaskInfo, selected bool) string {
	// Status icons
	statusIcon := ""
	switch task.Status {
	case "completed":
		statusIcon = "✓"
	case "running":
		statusIcon = "●"
	case "awaiting-approval":
		statusIcon = "◷"
	case "tmux":
		statusIcon = "▣"
	case "pending":
		statusIcon = "○"
	case "failed":
		statusIcon = "✗"
	case "summarizing":
		statusIcon = "◉"
	case "stopped":
		statusIcon = "■"
	default:
		statusIcon = "○"
	}

	// Format columns
	taskID := fmt.Sprintf("#%-2d", task.ID)

	// Merge step info into status: show step name for active states
	statusLabel := task.Status
	switch task.Status {
	case "running", "awaiting-approval", "tmux", "failed", "stopped":
		if task.CurrentStep != "" {
			statusLabel = task.CurrentStep
		}
	case "pending":
		if len(task.BlockedBy) > 0 {
			if l.hasFailedBlocker(task) {
				statusLabel = "pending (deadlocked)"
			} else {
				statusLabel = "pending (blocked)"
			}
		}
	}
	if l.tmuxSessions[task.ID] {
		statusLabel += " [T]"
	}
	status := fmt.Sprintf("%s %s", statusIcon, statusLabel)

	// Use title or truncated description
	title := task.Title
	if title == "" {
		title = task.Description
	}
	if len(title) > 60 {
		title = title[:57] + "..."
	}

	// Use lipgloss Width for ANSI-aware column alignment
	idCol := lipgloss.NewStyle().Width(5).Render(taskID)

	if selected {
		statusCol := lipgloss.NewStyle().Width(22).Render(status)
		var line string
		if l.globalMode {
			projName := task.ProjectName
			if projName == "" {
				projName = "-"
			}
			if len(projName) > 12 {
				projName = projName[:12]
			}
			projCol := lipgloss.NewStyle().Width(14).Render(projName)
			line = fmt.Sprintf("  %s %s %s %s", idCol, projCol, statusCol, title)
		} else {
			line = fmt.Sprintf("  %s %s %s", idCol, statusCol, title)
		}
		return selectedStyle.Render(line)
	}

	// Apply status color for non-selected rows
	statusStyle := stateStyle(task.Status)
	if strings.Contains(status, "(deadlocked)") {
		statusStyle = stateStyle("failed")
	}
	statusCol := statusStyle.Width(22).Render(status)
	var line string
	if l.globalMode {
		projName := task.ProjectName
		if projName == "" {
			projName = "-"
		}
		if len(projName) > 12 {
			projName = projName[:12]
		}
		projCol := lipgloss.NewStyle().Width(14).Render(projName)
		line = fmt.Sprintf("  %s %s %s %s", idCol, projCol, statusCol, title)
	} else {
		line = fmt.Sprintf("  %s %s %s", idCol, statusCol, title)
	}
	return normalStyle.Render(line)
}

func (l *listView) renderHelp() string {
	keys := newKeyMap()
	var help strings.Builder

	help.WriteString(helpStyle.Render("  "))

	bindings := keys.ShortHelp()
	for i, binding := range bindings {
		if i > 0 {
			help.WriteString(helpStyle.Render(" | "))
		}
		help.WriteString(dimStyle.Render(binding.Help().Key))
		help.WriteString(helpStyle.Render(" "))
		help.WriteString(helpStyle.Render(binding.Help().Desc))
	}

	return help.String()
}

// hasFailedBlocker checks if any of the task's blockers have failed status.
func (l *listView) hasFailedBlocker(task daemon.TaskInfo) bool {
	for _, blockerID := range task.BlockedBy {
		for _, t := range l.tasks {
			if t.ID == blockerID && t.Status == "failed" {
				return true
			}
		}
	}
	return false
}

