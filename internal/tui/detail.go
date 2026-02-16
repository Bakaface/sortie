package tui

import (
	"fmt"
	"strings"

	"github.com/aface/ralph-tamer-kit/internal/daemon"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

type detailView struct {
	task       *daemon.TaskInfo
	output     []string
	viewport   viewport.Model
	width      int
	height     int
	ready      bool
	followMode bool
	pendingG   bool
}

func newDetailView() detailView {
	return detailView{
		output:     make([]string, 0),
		followMode: true,
	}
}

func (d *detailView) SetTask(task *daemon.TaskInfo) {
	d.task = task
}

func (d *detailView) SetOutput(lines []string) {
	d.output = lines
	d.updateViewportContent()
	if d.followMode {
		d.viewport.GotoBottom()
	}
}

func (d *detailView) AppendOutput(lines []string) {
	d.output = append(d.output, lines...)
	d.updateViewportContent()
	d.viewport.GotoBottom()
}

func (d *detailView) SetSize(width, height int) {
	d.width = width
	d.height = height

	headerHeight := 2
	footerHeight := 2
	vpHeight := height - headerHeight - footerHeight

	if !d.ready {
		d.viewport = viewport.New(width-4, vpHeight)
		d.viewport.HighPerformanceRendering = false
		d.ready = true
	} else {
		d.viewport.Width = width - 4
		d.viewport.Height = vpHeight
	}

	d.updateViewportContent()
}

func (d *detailView) updateViewportContent() {
	if !d.ready {
		return
	}

	content := strings.Join(d.output, "\n")
	d.viewport.SetContent(content)
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

	// Title bar with status
	taskTitle := d.task.Title
	if taskTitle == "" {
		taskTitle = d.task.Description
	}
	stateStyled := stateStyle(d.task.Status).Render(d.task.Status)
	title := fmt.Sprintf(" #%d %s ", d.task.ID, taskTitle)
	b.WriteString(titleStyle.Render(title))
	b.WriteString(" ")
	b.WriteString(stateStyled)
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
		bindings = newDetailFollowKeyMap().ShortHelp()
	} else {
		bindings = newDetailNormalKeyMap().ShortHelp()
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

func (d *detailView) renderStatusBar() string {
	if d.task == nil {
		return ""
	}

	scrollPercent := 0
	if d.viewport.TotalLineCount() > 0 {
		scrollPercent = int(float64(d.viewport.YOffset) / float64(d.viewport.TotalLineCount()) * 100)
	}

	left := fmt.Sprintf(" #%d ", d.task.ID)
	right := fmt.Sprintf(" %d%% ", scrollPercent)

	gap := d.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}

	return statusBarStyle.Render(left + strings.Repeat(" ", gap) + right)
}
