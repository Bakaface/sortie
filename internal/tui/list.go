package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/aface/ralph-tamer-kit/internal/daemon"
	"github.com/charmbracelet/lipgloss"
)

type listView struct {
	tasks    []daemon.TaskInfo
	cursor   int
	width    int
	height   int
	showHelp bool
}

func newListView() listView {
	return listView{
		tasks:    make([]daemon.TaskInfo, 0),
		cursor:   0,
		showHelp: false,
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

	title := titleStyle.Render(" Ralph Tamer Kit ")
	b.WriteString(title)
	b.WriteString("\n\n")

	if len(l.tasks) == 0 {
		b.WriteString(dimStyle.Render("  No tasks found. Use 'ralph-tamer-kit plan <PRD.md>' to create tasks."))
		b.WriteString("\n")
	} else {
		header := fmt.Sprintf("  %-4s %-10s %-30s %s",
			"ID", "STATUS", "STEP", "TITLE")
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
	case "awaiting_approval":
		statusIcon = "◷"
	case "pending":
		statusIcon = "○"
	case "failed":
		statusIcon = "✗"
	case "stopped":
		statusIcon = "■"
	default:
		statusIcon = "○"
	}

	// Format columns
	taskID := fmt.Sprintf("#%-2d", task.ID)
	status := fmt.Sprintf("%s %-8s", statusIcon, task.Status)

	// Add blocked suffix for pending tasks
	if task.Status == "pending" && len(task.BlockedBy) > 0 {
		status += " (blocked)"
	}

	// Show step info
	step := ""
	if task.CurrentStep != "" {
		step = truncate(task.CurrentStep, 30)
	} else if task.StepIndex > 0 {
		step = fmt.Sprintf("Step %d", task.StepIndex)
	}

	// Use title or truncated description
	title := task.Title
	if title == "" {
		title = task.Description
	}
	title = truncate(title, 60)

	stateStyled := stateStyle(task.Status).Render(fmt.Sprintf("%-20s", status))

	line := fmt.Sprintf("  %-4s %s %-30s %s",
		taskID, stateStyled, step, title)

	if selected {
		return selectedStyle.Render(line)
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

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return lipgloss.NewStyle().Width(maxLen).Render(s)
	}
	return s[:maxLen-3] + "..."
}

func shortenPath(path string) string {
	home := "~"
	if strings.HasPrefix(path, "/home/") {
		parts := strings.SplitN(path, "/", 4)
		if len(parts) >= 4 {
			return home + "/" + parts[3]
		}
	}
	return path
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
