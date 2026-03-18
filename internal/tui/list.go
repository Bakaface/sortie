package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aface/sortie/internal/daemon"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type listView struct {
	table           table.Model
	tasks           []daemon.TaskInfo
	allTasks        []daemon.TaskInfo // unfiltered tasks (used when showFinished is false)
	width           int
	height          int
	showHelp        bool
	showLineNumbers bool
	showBranch      bool
	showTarget      bool
	showFinished    bool
	globalMode      bool
	projectName     string
	tmuxSessions    map[int64]bool
	pendingG        bool
	matchedIndices  []int // indices of tasks matching current search
	currentMatchIdx int   // index within matchedIndices
	scrollOffset       int  // index of first visible task row
	extraLines         int  // extra lines reserved below the list (search bar, command bar, etc.)
	hasScrollIndicator bool // true when not all tasks fit in the visible window
	loading            bool           // true before the first successful task load
	refreshing         bool           // true while a background refresh is in-flight
	spinner            spinner.Model  // loading spinner for initial load
}

// safeKeyMap returns a table.KeyMap that only handles basic navigation.
// Keys like G, gg, dd, n, N are handled manually to avoid conflicts.
func safeKeyMap() table.KeyMap {
	return table.KeyMap{
		LineUp:       key.NewBinding(key.WithKeys("up", "k")),
		LineDown:     key.NewBinding(key.WithKeys("down", "j")),
		PageUp:       key.NewBinding(key.WithKeys("pgup")),
		PageDown:     key.NewBinding(key.WithKeys("pgdown")),
		HalfPageUp:   key.NewBinding(key.WithKeys("ctrl+u")),
		HalfPageDown: key.NewBinding(key.WithKeys("ctrl+d")),
		GotoTop:      key.NewBinding(key.WithKeys("home")),
		GotoBottom:   key.NewBinding(key.WithKeys("end")),
	}
}

func newListView(globalMode bool, projectName string) listView {
	t := table.New(
		table.WithColumns(computeColumns(80, globalMode, true, 0)),
		table.WithFocused(true),
		table.WithHeight(20),
		table.WithKeyMap(safeKeyMap()),
	)
	// Minimal styles — custom rendering handles visuals
	s := table.DefaultStyles()
	s.Selected = lipgloss.NewStyle()
	s.Cell = lipgloss.NewStyle()
	s.Header = lipgloss.NewStyle()
	t.SetStyles(s)
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = dimStyle
	return listView{
		table:           t,
		tasks:           make([]daemon.TaskInfo, 0),
		allTasks:        make([]daemon.TaskInfo, 0),
		showLineNumbers: true,
		showFinished:    true,
		globalMode:      globalMode,
		projectName:     projectName,
		loading:         true,
		spinner:         sp,
	}
}

// computeColumns calculates column widths with the title column filling remaining space.
func computeColumns(width int, globalMode, lineNumbers bool, taskCount int) []table.Column {
	const (
		idWidth      = 5
		priWidth     = 2
		statusWidth  = 18
		projWidth    = 14
		spacing      = 5 // gaps between columns
		minTitleWidth = 15
	)

	fixed := idWidth + priWidth + statusWidth + spacing
	if globalMode {
		fixed += projWidth
	}
	if lineNumbers {
		fixed += lineNumWidthForCount(taskCount)
	}

	titleWidth := width - fixed
	if titleWidth < minTitleWidth {
		titleWidth = minTitleWidth
	}

	// Columns are used for width tracking only; we render rows ourselves
	cols := []table.Column{
		{Title: "ID", Width: idWidth},
		{Title: "P", Width: priWidth},
	}
	if globalMode {
		cols = append(cols, table.Column{Title: "PROJECT", Width: projWidth})
	}
	cols = append(cols,
		table.Column{Title: "STATUS", Width: statusWidth},
		table.Column{Title: "TITLE", Width: titleWidth},
	)
	return cols
}

// lineNumWidthForCount returns the gutter width for a given task count.
func lineNumWidthForCount(n int) int {
	width := 0
	for n > 0 {
		width++
		n /= 10
	}
	return max(2, width)
}

func (l *listView) SetTasks(tasks []daemon.TaskInfo) {
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ID > tasks[j].ID
	})

	wasLoading := l.loading

	// Skip full rebuild if nothing meaningful changed
	if !wasLoading && !l.tasksChanged(l.allTasks, tasks) {
		l.loading = false
		l.refreshing = false
		return
	}

	l.allTasks = tasks
	l.tasks = l.filterTasks(tasks)
	l.table.SetRows(l.toRows())
	if l.table.Cursor() >= len(l.tasks) && len(l.tasks) > 0 {
		l.table.SetCursor(len(l.tasks) - 1)
	} else if len(l.tasks) == 0 {
		l.table.SetCursor(0)
	}
	l.recomputeColumns()
	l.ensureVisible()
	l.loading = false
	l.refreshing = false
}

// tasksChanged returns true if the two task slices differ in any visible field.
func (l *listView) tasksChanged(old, new []daemon.TaskInfo) bool {
	if len(old) != len(new) {
		return true
	}
	for i := range old {
		a, b := old[i], new[i]
		if a.ID != b.ID ||
			a.Status != b.Status ||
			a.Title != b.Title ||
			a.CurrentStep != b.CurrentStep ||
			a.Priority != b.Priority ||
			a.LoopIteration != b.LoopIteration ||
			a.TmuxActivity != b.TmuxActivity ||
			a.WorktreeDetached != b.WorktreeDetached {
			return true
		}
		// Compare blocked-by lists
		if len(a.BlockedBy) != len(b.BlockedBy) {
			return true
		}
		for j := range a.BlockedBy {
			if a.BlockedBy[j] != b.BlockedBy[j] {
				return true
			}
		}
	}
	return false
}

// filterTasks returns tasks filtered according to display settings.
func (l *listView) filterTasks(tasks []daemon.TaskInfo) []daemon.TaskInfo {
	if l.showFinished {
		return tasks
	}
	filtered := make([]daemon.TaskInfo, 0, len(tasks))
	for _, t := range tasks {
		if t.Status != "completed" && t.Status != "failed" {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// applyFilter re-filters allTasks into tasks and adjusts cursor.
func (l *listView) applyFilter() {
	l.tasks = l.filterTasks(l.allTasks)
	l.table.SetRows(l.toRows())
	if l.table.Cursor() >= len(l.tasks) && len(l.tasks) > 0 {
		l.table.SetCursor(len(l.tasks) - 1)
	} else if len(l.tasks) == 0 {
		l.table.SetCursor(0)
	}
	l.recomputeColumns()
	l.ensureVisible()
}

func (l *listView) UpdateTask(task daemon.TaskInfo) {
	// Update in allTasks
	found := false
	for i, t := range l.allTasks {
		if t.ID == task.ID {
			l.allTasks[i] = task
			found = true
			break
		}
	}
	if !found {
		l.allTasks = append(l.allTasks, task)
	}
	l.applyFilter()
}

func (l *listView) RemoveTask(id int64) {
	for i, t := range l.allTasks {
		if t.ID == id {
			l.allTasks = append(l.allTasks[:i], l.allTasks[i+1:]...)
			break
		}
	}
	l.applyFilter()
}

func (l *listView) Selected() *daemon.TaskInfo {
	cursor := l.table.Cursor()
	if cursor >= 0 && cursor < len(l.tasks) {
		return &l.tasks[cursor]
	}
	return nil
}

func (l *listView) MoveUp() {
	l.table.MoveUp(1)
	l.ensureVisible()
}

func (l *listView) MoveDown() {
	l.table.MoveDown(1)
	l.ensureVisible()
}

func (l *listView) GotoTop() {
	l.table.GotoTop()
	l.ensureVisible()
}

func (l *listView) GotoBottom() {
	l.table.GotoBottom()
	l.ensureVisible()
}

// GotoIndex moves the cursor to the task at the given row index.
func (l *listView) GotoIndex(index int) {
	if index >= 0 && index < len(l.tasks) {
		l.table.SetCursor(index)
	}
	l.ensureVisible()
}

// ensureVisible adjusts scrollOffset so the cursor is within the visible window.
func (l *listView) ensureVisible() {
	cursor := l.table.Cursor()
	visible := l.visibleRows()
	if cursor < l.scrollOffset {
		l.scrollOffset = cursor
	}
	if cursor >= l.scrollOffset+visible {
		l.scrollOffset = cursor - visible + 1
	}
	if l.scrollOffset < 0 {
		l.scrollOffset = 0
	}
}

// lineNumWidth returns the character width needed for the line number gutter.
func (l *listView) lineNumWidth() int {
	return lineNumWidthForCount(len(l.tasks))
}

// visibleRows returns the number of task rows visible in the list.
// Layout: title(1) + blank(1) + header(2, includes border) + tasks + blank(1) + help(1) = 6 lines overhead.
// extraLines accounts for overlays appended below (search bar, command bar, etc.).
// When scroll indicators are shown (tasks above or below), they replace task rows
// rather than adding extra lines, so the total height stays constant.
func (l *listView) visibleRows() int {
	return max(1, l.height-7-l.extraLines)
}

func (l *listView) PageDown() {
	half := max(1, l.visibleRows()/2)
	target := l.table.Cursor() + half
	if target >= len(l.tasks) {
		target = max(0, len(l.tasks)-1)
	}
	l.table.SetCursor(target)
	l.ensureVisible()
}

func (l *listView) PageUp() {
	half := max(1, l.visibleRows()/2)
	target := l.table.Cursor() - half
	if target < 0 {
		target = 0
	}
	l.table.SetCursor(target)
	l.ensureVisible()
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
	l.table.SetWidth(width)
	l.table.SetHeight(l.visibleRows())
	l.recomputeColumns()
	l.ensureVisible()
}

func (l *listView) recomputeColumns() {
	cols := computeColumns(l.width, l.globalMode, l.showLineNumbers, len(l.tasks))
	l.table.SetColumns(cols)
}

// Update delegates key messages to the table for navigation.
func (l *listView) Update(msg tea.Msg) tea.Cmd {
	if l.loading {
		var cmd tea.Cmd
		l.spinner, cmd = l.spinner.Update(msg)
		return cmd
	}
	var cmd tea.Cmd
	l.table, cmd = l.table.Update(msg)
	return cmd
}

// titleWidth returns the width available for the title column.
func (l *listView) titleWidth() int {
	cols := l.table.Columns()
	for _, c := range cols {
		if c.Title == "TITLE" {
			return c.Width
		}
	}
	return 60
}

func (l *listView) View() string {
	var b strings.Builder

	titleText := " " + AppTitle + " "
	if l.globalMode {
		titleText = " " + AppTitle + " (Global) "
	}
	title := titleStyle.Render(titleText)
	if l.refreshing && !l.loading {
		title += " " + dimStyle.Render("⟳")
	}

	// Right-align the project name bracket widget on the same line
	if !l.globalMode && l.projectName != "" && l.width > 0 {
		projectWidget := projectIndicatorStyle.Render("[" + l.projectName + "]")
		gap := l.width - lipgloss.Width(title) - lipgloss.Width(projectWidget)
		if gap < 0 {
			gap = 0
		}
		b.WriteString(title + strings.Repeat(" ", gap) + projectWidget)
	} else {
		b.WriteString(title)
	}
	b.WriteString("\n\n")

	if len(l.tasks) == 0 && l.loading {
		b.WriteString("  " + l.spinner.View() + dimStyle.Render(" Loading tasks…"))
		b.WriteString("\n")
	} else if len(l.tasks) == 0 {
		b.WriteString(dimStyle.Render("  No tasks found. Use 'sortie plan <PRD.md>' to create tasks."))
		b.WriteString("\n")
	} else {
		b.WriteString(l.renderHeader())
		b.WriteString("\n")

		// Windowed rendering: only show tasks in the visible range
		cursor := l.table.Cursor()
		visible := l.visibleRows()
		l.ensureVisible()
		end := min(l.scrollOffset+visible, len(l.tasks))

		for i := l.scrollOffset; i < end; i++ {
			line := l.renderTask(l.tasks[i], i, i == cursor)
			b.WriteString(line)
			b.WriteString("\n")
		}

		// Show scroll position indicator in the help bar when not all tasks visible
		l.hasScrollIndicator = l.scrollOffset > 0 || end < len(l.tasks)
	}

	b.WriteString("\n")
	b.WriteString(l.renderHelp())

	return b.String()
}

func (l *listView) renderHeader() string {
	gutter := " "
	if l.showLineNumbers {
		gutterWidth := l.lineNumWidth()
		gutter = strings.Repeat(" ", gutterWidth+1)
	}
	tw := l.titleWidth()
	var header string
	if l.globalMode {
		header = fmt.Sprintf("%s %-5s %-2s %-14s %-18s %-*s",
			gutter, "ID", "P", "PROJECT", "STATUS", tw, "TITLE")
	} else {
		header = fmt.Sprintf("%s %-5s %-2s %-18s %-*s",
			gutter, "ID", "P", "STATUS", tw, "TITLE")
	}
	return headerStyle.Render(header)
}

func (l *listView) renderTask(task daemon.TaskInfo, index int, selected bool) string {
	// Check if this task is a search match
	isMatch := l.isSearchMatch(index)

	// Status icons
	statusIcon := statusIconFor(task.Status)

	taskID := fmt.Sprintf("%-2d", task.ID)

	// Merge step info into status: show step name for active states
	statusLabel := task.Status
	switch task.Status {
	case "running", "awaiting-approval", "tmux", "failed", "stopped", "artifact-missing":
		if task.CurrentStep != "" {
			statusLabel = task.CurrentStep
		}
	case "merge-blocked":
		statusLabel = "merge-blocked"
	case "pending":
		if len(task.BlockedBy) > 0 {
			if l.hasFailedBlocker(task) {
				statusLabel = "pending (deadlocked)"
			} else {
				statusLabel = "pending (blocked)"
			}
		}
	}
	if task.LoopIteration > 0 {
		statusLabel += fmt.Sprintf(" [L%d]", task.LoopIteration)
	}
	if task.WorktreeDetached {
		statusLabel += " [detached]"
	} else if l.tmuxSessions[task.ID] {
		switch task.TmuxActivity {
		case "idle":
			statusLabel += " [idle]"
		case "wip":
			statusLabel += " [wip]"
		default:
			statusLabel += " [T]"
		}
	}
	status := fmt.Sprintf("%s %s", statusIcon, statusLabel)

	// Use title or truncated description
	title := task.Title
	if title == "" {
		title = task.Description
	}

	// Build optional branch suffix
	var branchSuffix string
	if l.showBranch && task.Branch != "" {
		branchSuffix += " <- " + task.Branch
	}
	if l.showTarget && task.TargetBranch != "" {
		branchSuffix += " -> " + task.TargetBranch
	}
	if branchSuffix != "" {
		title = title + branchSuffix
	}

	tw := l.titleWidth()
	title = truncateOrPad(title, tw)

	priBadge := priorityBadge(task.Priority)

	// Selected and matched rows use plain inner styles so the outer style's
	// background covers the entire row without inner ANSI resets breaking it.
	if selected || isMatch {
		outerStyle := searchMatchStyle
		if selected {
			outerStyle = selectedStyle
		}
		idCol := truncateOrPad(taskID, 5)
		priCol := truncateOrPad(priBadge, 2)
		statusCol := truncateOrPad(status, 18)

		var line string
		if l.showLineNumbers {
			gutterWidth := l.lineNumWidth()
			idxStr := fmt.Sprintf("%*d", gutterWidth, index+1)
			idxCol := truncateOrPad(idxStr, gutterWidth)
			if l.globalMode {
				projCol := truncateOrPad(l.projectNameFor(task), 14)
				line = fmt.Sprintf(" %s %s %s %s %s %s", idxCol, idCol, priCol, projCol, statusCol, title)
			} else {
				line = fmt.Sprintf(" %s %s %s %s %s", idxCol, idCol, priCol, statusCol, title)
			}
		} else {
			if l.globalMode {
				projCol := truncateOrPad(l.projectNameFor(task), 14)
				line = fmt.Sprintf("  %s %s %s %s %s", idCol, priCol, projCol, statusCol, title)
			} else {
				line = fmt.Sprintf("  %s %s %s %s", idCol, priCol, statusCol, title)
			}
		}
		// Pad to full terminal width so the background fills the entire row
		if lineWidth := lipgloss.Width(line); lineWidth < l.width {
			line += strings.Repeat(" ", l.width-lineWidth)
		}
		return outerStyle.Render(line)
	}

	// Apply priority/status colors for non-selected, non-matched rows
	idCol := lipgloss.NewStyle().Width(5).Render(taskID)
	priCol := priorityStyle(task.Priority).Width(2).Render(priBadge)
	statusSt := stateStyle(task.Status)
	if strings.Contains(status, "(deadlocked)") {
		statusSt = stateStyle("failed")
	}
	statusCol := statusSt.Width(18).Render(status)
	var line string
	if l.showLineNumbers {
		gutterWidth := l.lineNumWidth()
		idxStr := fmt.Sprintf("%*d", gutterWidth, index+1)
		idxCol := lineNumStyle.Width(gutterWidth).Render(idxStr)
		if l.globalMode {
			projCol := lipgloss.NewStyle().Width(14).Render(l.projectNameFor(task))
			line = fmt.Sprintf(" %s %s %s %s %s %s", idxCol, idCol, priCol, projCol, statusCol, title)
		} else {
			line = fmt.Sprintf(" %s %s %s %s %s", idxCol, idCol, priCol, statusCol, title)
		}
	} else {
		if l.globalMode {
			projCol := lipgloss.NewStyle().Width(14).Render(l.projectNameFor(task))
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

	// Show scroll position indicator when task list is scrollable
	if l.hasScrollIndicator && len(l.tasks) > 0 {
		visible := l.visibleRows()
		end := min(l.scrollOffset+visible, len(l.tasks))
		help.WriteString(helpStyle.Render(" | "))
		help.WriteString(dimStyle.Render(fmt.Sprintf("%d-%d/%d", l.scrollOffset+1, end, len(l.tasks))))
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

// toRows converts tasks to table.Row for the table's internal management.
func (l *listView) toRows() []table.Row {
	rows := make([]table.Row, len(l.tasks))
	for i, t := range l.tasks {
		rows[i] = table.Row{fmt.Sprintf("%d", t.ID), t.Title}
	}
	return rows
}

// projectNameFor returns a sanitized project name for display.
func (l *listView) projectNameFor(task daemon.TaskInfo) string {
	name := task.ProjectName
	if name == "" {
		name = "-"
	}
	if len(name) > 12 {
		name = name[:12]
	}
	return name
}

// statusIconFor returns the status icon for a given task status.
func statusIconFor(status string) string {
	switch status {
	case "completed":
		return "✓"
	case "running":
		return "●"
	case "awaiting-approval":
		return "◷"
	case "tmux":
		return "▣"
	case "pending":
		return "○"
	case "failed":
		return "✗"
	case "finalizing":
		return "◉"
	case "summarizing":
		return "◉"
	case "stopped":
		return "■"
	case "artifact-missing":
		return "◇"
	case "merge-blocked":
		return "⊘"
	default:
		return "○"
	}
}

// truncateOrPad truncates or pads a plain string to the given width.
func truncateOrPad(s string, width int) string {
	// Count rune width for proper handling
	runeLen := len([]rune(s))
	if runeLen > width {
		runes := []rune(s)
		if width > 3 {
			return string(runes[:width-3]) + "..."
		}
		return string(runes[:width])
	}
	if runeLen < width {
		return s + strings.Repeat(" ", width-runeLen)
	}
	return s
}
