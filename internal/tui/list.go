package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aface/sortie/internal/daemon"
	"github.com/charmbracelet/lipgloss"
)

type listView struct {
	tasks           []daemon.TaskInfo
	cursor          int
	width           int
	height          int
	showHelp        bool
	showLineNumbers bool
	globalMode      bool
	tmuxSessions    map[int64]bool
	pendingG        bool
}

func newListView(globalMode bool) listView {
	return listView{
		tasks:           make([]daemon.TaskInfo, 0),
		cursor:          0,
		showHelp:        false,
		showLineNumbers: true,
		globalMode:      globalMode,
	}
}

func (l *listView) SetTasks(tasks []daemon.TaskInfo) {
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ID > tasks[j].ID
	})
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

func (l *listView) GotoTop() {
	l.cursor = 0
}

func (l *listView) GotoBottom() {
	if len(l.tasks) > 0 {
		l.cursor = len(l.tasks) - 1
	}
}

// GotoIndex moves the cursor to the task at the given row index.
func (l *listView) GotoIndex(index int) {
	if index >= 0 && index < len(l.tasks) {
		l.cursor = index
	}
}

// lineNumWidth returns the character width needed for the line number gutter.
// Minimum width of 2 keeps columns aligned with the old layout; scales up for 100+ tasks.
func (l *listView) lineNumWidth() int {
	n := len(l.tasks)
	width := 0
	for n > 0 {
		width++
		n /= 10
	}
	return max(2, width)
}

// visibleRows returns the number of task rows visible in the list.
// Layout: title(1) + blank(1) + header(1) + tasks + blank(1) + help(1) = 5 lines overhead.
func (l *listView) visibleRows() int {
	return max(1, l.height-5)
}

func (l *listView) PageDown() {
	half := max(1, l.visibleRows()/2)
	l.cursor += half
	if l.cursor >= len(l.tasks) {
		l.cursor = max(0, len(l.tasks)-1)
	}
}

func (l *listView) PageUp() {
	half := max(1, l.visibleRows()/2)
	l.cursor -= half
	if l.cursor < 0 {
		l.cursor = 0
	}
}

func (l *listView) IsPendingG() bool {
	return l.pendingG
}

func (l *listView) SetPendingG(v bool) {
	l.pendingG = v
}

func (l *listView) SetSize(width, height int) {
	l.width = width
	l.height = height
}

func (l *listView) View() string {
	var b strings.Builder

	var titleText string
	if l.globalMode {
		titleText = " Sortie (Global) "
	} else {
		titleText = " Sortie "
	}
	title := titleStyle.Render(titleText)
	b.WriteString(title)
	b.WriteString("\n\n")

	if len(l.tasks) == 0 {
		b.WriteString(dimStyle.Render("  No tasks found. Use 'sortie plan <PRD.md>' to create tasks."))
		b.WriteString("\n")
	} else {
		// Line number gutter (vim-style, no header label)
		gutter := ""
		if l.showLineNumbers {
			gutterWidth := l.lineNumWidth()
			gutter = strings.Repeat(" ", gutterWidth+2)
		}
		var header string
		if l.globalMode {
			header = fmt.Sprintf("%s %-5s %-2s %-14s %-22s %s",
				gutter, "ID", "P", "PROJECT", "STATUS", "TITLE")
		} else {
			header = fmt.Sprintf("%s %-5s %-2s %-22s %s",
				gutter, "ID", "P", "STATUS", "TITLE")
		}
		b.WriteString(headerStyle.Render(header))
		b.WriteString("\n")

		for i, task := range l.tasks {
			line := l.renderTask(task, i, i == l.cursor)
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(l.renderHelp())

	return b.String()
}

func (l *listView) renderTask(task daemon.TaskInfo, index int, selected bool) string {
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
	case "artifact-missing":
		statusIcon = "◇"
	default:
		statusIcon = "○"
	}

	taskID := fmt.Sprintf("%-2d", task.ID)

	// Merge step info into status: show step name for active states
	statusLabel := task.Status
	switch task.Status {
	case "running", "awaiting-approval", "tmux", "failed", "stopped", "artifact-missing":
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
	priBadge := priorityBadge(task.Priority)

	if selected {
		priCol := lipgloss.NewStyle().Width(2).Render(priBadge)
		statusCol := lipgloss.NewStyle().Width(22).Render(status)
		var line string
		if l.showLineNumbers {
			gutterWidth := l.lineNumWidth()
			idxStr := fmt.Sprintf("%*d", gutterWidth, index+1)
			// Use plain style for line number so selectedStyle's background covers it
			idxCol := lipgloss.NewStyle().Width(gutterWidth).Render(idxStr)
			if l.globalMode {
				projName := task.ProjectName
				if projName == "" {
					projName = "-"
				}
				if len(projName) > 12 {
					projName = projName[:12]
				}
				projCol := lipgloss.NewStyle().Width(14).Render(projName)
				line = fmt.Sprintf("  %s %s %s %s %s %s", idxCol, idCol, priCol, projCol, statusCol, title)
			} else {
				line = fmt.Sprintf("  %s %s %s %s %s", idxCol, idCol, priCol, statusCol, title)
			}
		} else {
			if l.globalMode {
				projName := task.ProjectName
				if projName == "" {
					projName = "-"
				}
				if len(projName) > 12 {
					projName = projName[:12]
				}
				projCol := lipgloss.NewStyle().Width(14).Render(projName)
				line = fmt.Sprintf("  %s %s %s %s %s", idCol, priCol, projCol, statusCol, title)
			} else {
				line = fmt.Sprintf("  %s %s %s %s", idCol, priCol, statusCol, title)
			}
		}
		// Pad to full terminal width so the blue background fills the entire row
		if lineWidth := lipgloss.Width(line); lineWidth < l.width {
			line += strings.Repeat(" ", l.width-lineWidth)
		}
		return selectedStyle.Render(line)
	}

	// Apply priority/status colors for non-selected rows
	priCol := priorityStyle(task.Priority).Width(2).Render(priBadge)
	statusSt := stateStyle(task.Status)
	if strings.Contains(status, "(deadlocked)") {
		statusSt = stateStyle("failed")
	}
	statusCol := statusSt.Width(22).Render(status)
	var line string
	if l.showLineNumbers {
		gutterWidth := l.lineNumWidth()
		idxStr := fmt.Sprintf("%*d", gutterWidth, index+1)
		idxCol := lineNumStyle.Width(gutterWidth).Render(idxStr)
		if l.globalMode {
			projName := task.ProjectName
			if projName == "" {
				projName = "-"
			}
			if len(projName) > 12 {
				projName = projName[:12]
			}
			projCol := lipgloss.NewStyle().Width(14).Render(projName)
			line = fmt.Sprintf("  %s %s %s %s %s %s", idxCol, idCol, priCol, projCol, statusCol, title)
		} else {
			line = fmt.Sprintf("  %s %s %s %s %s", idxCol, idCol, priCol, statusCol, title)
		}
	} else {
		if l.globalMode {
			projName := task.ProjectName
			if projName == "" {
				projName = "-"
			}
			if len(projName) > 12 {
				projName = projName[:12]
			}
			projCol := lipgloss.NewStyle().Width(14).Render(projName)
			line = fmt.Sprintf("  %s %s %s %s %s", idCol, priCol, projCol, statusCol, title)
		} else {
			line = fmt.Sprintf("  %s %s %s %s", idCol, priCol, statusCol, title)
		}
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

