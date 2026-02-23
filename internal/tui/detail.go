package tui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/aface/sortie/internal/daemon"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// ansiRegex matches ANSI escape sequences (CSI, OSC, and carriage returns).
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]|\x1b\][^\x07]*\x07|\x1b[()][0-9A-B]|\r`)

type detailView struct {
	task       *daemon.TaskInfo
	output     []string
	viewport   viewport.Model
	width      int
	height     int
	ready      bool
	followMode bool
	pendingG   bool

	// Performance: track content state to avoid redundant re-wraps
	contentLineCount int  // number of lines last set on viewport
	contentDirty     bool // true when content needs re-wrap (e.g. resize)
}

func newDetailView() detailView {
	return detailView{
		output:     make([]string, 0),
		followMode: true,
	}
}

func (d *detailView) SetTask(task *daemon.TaskInfo) {
	d.task = task
	d.recalcViewport()
}

func (d *detailView) SetOutput(lines []string) {
	// Skip expensive re-wrap if content hasn't changed
	if len(lines) == d.contentLineCount && !d.contentDirty {
		return
	}
	// Strip ANSI escape codes (from tmux pipe-pane captures, etc.)
	cleaned := make([]string, len(lines))
	for i, line := range lines {
		cleaned[i] = ansiRegex.ReplaceAllString(line, "")
	}
	d.output = cleaned
	d.updateViewportContent()
	if d.followMode {
		d.viewport.GotoBottom()
	}
}

func (d *detailView) AppendOutput(lines []string) {
	d.output = append(d.output, lines...)
	d.contentDirty = true
	d.updateViewportContent()
	d.viewport.GotoBottom()
}

func (d *detailView) SetSize(width, height int) {
	d.width = width
	d.height = height
	d.recalcViewport()
}

func (d *detailView) headerLines() int {
	// "Sortie" title bar + blank line + gap before viewport = 3 lines
	base := 3
	if d.task != nil && d.width > 0 {
		taskTitle := d.task.Title
		if taskTitle == "" {
			taskTitle = d.task.Description
		}
		titleText := fmt.Sprintf("#%d %s", d.task.ID, taskTitle)
		rendered := lipgloss.NewStyle().Width(d.width).Render(titleText)
		return base + lipgloss.Height(rendered)
	}
	return base + 1
}

func (d *detailView) recalcViewport() {
	if d.width == 0 || d.height == 0 {
		return
	}

	headerHeight := d.headerLines()
	footerHeight := 2
	vpHeight := d.height - headerHeight - footerHeight
	if vpHeight < 1 {
		vpHeight = 1
	}

	if !d.ready {
		d.viewport = viewport.New(d.width-4, vpHeight)
		d.viewport.HighPerformanceRendering = false
		d.ready = true
	} else {
		d.viewport.Width = d.width - 4
		d.viewport.Height = vpHeight
	}

	d.contentDirty = true
	d.updateViewportContent()
}

func (d *detailView) updateViewportContent() {
	if !d.ready {
		return
	}

	// Set content directly without expensive lipgloss full-content wrapping.
	// The viewport's own View() method handles rendering visible lines.
	content := strings.Join(d.output, "\n")
	d.viewport.SetContent(content)
	d.contentLineCount = len(d.output)
	d.contentDirty = false
}

func (d *detailView) ScrollUp() {
	d.viewport.LineUp(1)
}

func (d *detailView) ScrollDown() {
	d.viewport.LineDown(1)
}

func (d *detailView) PageUp() {
	d.viewport.HalfViewUp()
}

func (d *detailView) PageDown() {
	d.viewport.HalfViewDown()
}

func (d *detailView) GotoTop() {
	d.viewport.GotoTop()
}

func (d *detailView) GotoBottom() {
	d.viewport.GotoBottom()
}

func (d *detailView) IsFollowMode() bool {
	return d.followMode
}

func (d *detailView) SetFollowMode(follow bool) {
	d.followMode = follow
	if follow {
		d.viewport.GotoBottom()
	}
}

func (d *detailView) IsPendingG() bool {
	return d.pendingG
}

func (d *detailView) SetPendingG(pending bool) {
	d.pendingG = pending
}

func (d *detailView) View() string {
	if d.task == nil {
		return "No task selected"
	}

	var b strings.Builder

	// App title
	b.WriteString(titleStyle.Render(" Sortie "))
	b.WriteString("\n\n")

	// Task title with word wrapping
	taskTitle := d.task.Title
	if taskTitle == "" {
		taskTitle = d.task.Description
	}
	titleText := fmt.Sprintf("#%d %s", d.task.ID, taskTitle)
	wrappedTitle := subHeaderStyle.Width(d.width).Render("  " + titleText)
	b.WriteString(wrappedTitle)
	b.WriteString("\n")

	// Live logs viewport
	if d.ready {
		vpContent := viewportStyle.Render(d.viewport.View())
		b.WriteString(vpContent)
	} else {
		b.WriteString("  Loading...")
	}

	b.WriteString("\n")
	b.WriteString(d.renderHelp())

	return b.String()
}

func (d *detailView) renderHelp() string {
	var help strings.Builder
	help.WriteString(helpStyle.Render("  "))

	// Mode indicator
	if d.followMode {
		help.WriteString(stateStyle("running").Render("FOLLOW"))
	} else {
		help.WriteString(stateStyle("pending").Render("NORMAL"))
	}
	help.WriteString(helpStyle.Render(" "))

	var bindings []key.Binding
	if d.followMode {
		bindings = cachedDetailFollowKeyMap.ShortHelp()
	} else {
		bindings = cachedDetailNormalKeyMap.ShortHelp()
	}
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

