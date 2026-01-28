package tui

import (
	"fmt"
	"strings"

	"github.com/aface/ralph-tamer-kit/internal/daemon"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

type detailView struct {
	task     *daemon.TaskInfo
	output   []string
	viewport viewport.Model
	width    int
	height   int
	ready    bool
}

func newDetailView() detailView {
	return detailView{
		output: make([]string, 0),
	}
}

func (d *detailView) SetTask(task *daemon.TaskInfo) {
	d.task = task
}

func (d *detailView) SetOutput(lines []string) {
	d.output = lines
	d.updateViewportContent()
}

func (d *detailView) AppendOutput(lines []string) {
	d.output = append(d.output, lines...)
	d.updateViewportContent()
	d.viewport.GotoBottom()
}

func (d *detailView) SetSize(width, height int) {
	d.width = width
	d.height = height

	headerHeight := 5
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

func (d *detailView) View() string {
	if d.task == nil {
		return "No task selected"
	}

	var b strings.Builder

	// Title
	taskTitle := d.task.Title
	if taskTitle == "" {
		taskTitle = d.task.Description
	}
	title := titleStyle.Render(fmt.Sprintf(" Task #%d: %s ", d.task.ID, taskTitle))
	b.WriteString(title)
	b.WriteString("\n\n")

	// Task info
	stateStyled := stateStyle(d.task.Status).Render(d.task.Status)
	info := fmt.Sprintf("  Status: %s", stateStyled)
	if d.task.Branch != "" {
		info += fmt.Sprintf(" | Branch: %s", d.task.Branch)
	}
	b.WriteString(dimStyle.Render(info))
	b.WriteString("\n")

	// Workflow progress
	if d.task.CurrentStep != "" {
		progress := fmt.Sprintf("  Step %d: %s", d.task.StepIndex, d.task.CurrentStep)
		b.WriteString(dimStyle.Render(progress))
		b.WriteString("\n")
	}

	// Description (if title was used above)
	if d.task.Title != "" && d.task.Description != "" {
		desc := fmt.Sprintf("  %s", d.task.Description)
		b.WriteString(dimStyle.Render(desc))
		b.WriteString("\n")
	}

	// Error message
	if d.task.ErrorMessage != "" {
		errorMsg := fmt.Sprintf("  Error: %s", d.task.ErrorMessage)
		b.WriteString(stateStyle("failed").Render(errorMsg))
		b.WriteString("\n")
	}

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
	keys := newDetailKeyMap()
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
