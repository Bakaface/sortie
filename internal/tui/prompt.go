package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// imageExtensions contains the file extensions we recognize as images
var imageExtensions = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".webp": true,
}

type promptField int

const (
	promptFieldTitle       promptField = iota
	promptFieldDescription
	promptFieldBranch
	promptFieldCheckout
	promptFieldTargetBranch
)

type branchMode int

const (
	branchModeNew      branchMode = iota // create new branch (default)
	branchModeExisting                    // checkout existing branch
)

type promptPane int

const (
	paneTask     promptPane = iota // left pane: title, description, git fields
	paneWorkflow                   // right pane: workflow list
)

type promptView struct {
	textarea          textarea.Model
	titleInput        textinput.Model
	branchInput       textinput.Model
	checkoutInput     textinput.Model
	targetBranchInput textinput.Model
	focusField        promptField
	worktree          bool
	branchMode        branchMode
	defaultBaseBranch string
	defaultWorkflow   string // saved default workflow name (persists across Reset)
	images            []string
	workflowName      string
	workflows         []string // available workflow names for cycling
	blockingTaskID    int64    // when non-zero, new task blocks this task
	blockingTaskTitle string   // title of the blocking task for display
	width             int
	height            int
	showHelp          bool
	validationError   string // shown after failed submit attempt
	activePane        promptPane
	workflowCursor    int
}

func newPromptView(defaultWorktree bool, defaultBranchMode branchMode, defaultBaseBranch string) promptView {
	ta := textarea.New()
	ta.Prompt = PromptPrefix
	ta.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(promptColor)
	ta.BlurredStyle.Prompt = lipgloss.NewStyle().Foreground(promptColor)
	ta.Placeholder = "Describe the task..."
	ta.Focus()
	ta.CharLimit = 0 // unlimited
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("ctrl+j"), key.WithHelp("ctrl+j", "new line"))
	ta.KeyMap.WordForward = key.NewBinding(key.WithKeys("alt+right", "ctrl+right", "alt+f"), key.WithHelp("ctrl+right", "word forward"))
	ta.KeyMap.WordBackward = key.NewBinding(key.WithKeys("alt+left", "ctrl+left", "alt+b"), key.WithHelp("ctrl+left", "word backward"))

	titleIn := textinput.New()
	titleIn.Placeholder = "auto-generated if left blank"
	titleIn.CharLimit = 200

	bi := textinput.New()
	bi.Placeholder = "sortie/{{task_id}}-{{task_slug}}"
	bi.CharLimit = 200

	ci := textinput.New()
	ci.Placeholder = "existing branch name"
	ci.CharLimit = 200

	ti := textinput.New()
	ti.Placeholder = defaultBaseBranch
	if ti.Placeholder == "" {
		ti.Placeholder = "main"
	}
	ti.CharLimit = 200

	return promptView{
		textarea:          ta,
		titleInput:        titleIn,
		branchInput:       bi,
		checkoutInput:     ci,
		targetBranchInput: ti,
		focusField:        promptFieldDescription,
		worktree:          defaultWorktree,
		branchMode:        defaultBranchMode,
		defaultBaseBranch: defaultBaseBranch,
		images:            make([]string, 0),
	}
}

func (p *promptView) SetSize(width, height int) {
	p.width = width
	p.height = height
	p.textarea.SetWidth(width - 4)
	// Account for "▸ " / "  " prefix (2 chars) before the label
	prefix := 2
	p.titleInput.Width = width - 4 - prefix - lipgloss.Width("Title: ") - lipgloss.Width(p.titleInput.Prompt) - 1
	// Git inputs are inside a frame: border(2) + paddingLeft(1) = 3 chars overhead.
	// textinput.View() renders at Width + promptWidth + 1 (cursor), so subtract that too.
	frameOuterWidth := width - 1 // matches innerWidth in View()
	gitFrameWidth := frameOuterWidth
	if len(p.workflows) > 1 && width >= 60 {
		gitFrameWidth = frameOuterWidth * 2 / 3
	}
	gitContentWidth := gitFrameWidth - 3 // frame overhead
	tiOverhead := lipgloss.Width(p.branchInput.Prompt) + 1 // textinput prompt + cursor
	p.branchInput.Width = gitContentWidth - prefix - lipgloss.Width("Branch: ") - tiOverhead
	p.checkoutInput.Width = gitContentWidth - prefix - lipgloss.Width("Checkout: ") - tiOverhead
	p.targetBranchInput.Width = gitContentWidth - prefix - lipgloss.Width("Target: ") - tiOverhead
	p.recalcHeight()
}

// maxHeight returns the maximum textarea height available within the terminal.
func (p *promptView) maxHeight() int {
	// Reserve lines for non-textarea content:
	// title bar(1) + blank(1) + titleInput(1) + blank(1) +
	// [textarea goes here] + blank(1) +
	// git frame top(1) + padding(1) + worktree(1) + mode(1) + blank(1) + branch(1) + target(1) + git frame bottom(1) +
	// blank(1) + help(1)
	reserved := 14
	if !p.worktree {
		reserved -= 4 // no mode/blank/branch/target lines
	}
	if len(p.workflows) > 1 && p.width > 0 && p.width < 60 {
		reserved += len(p.workflows) + 2 // workflow frame top + items + bottom
	}
	h := p.height - reserved
	if h < 1 {
		h = 1
	}
	return h
}

// recalcHeight adjusts the textarea height to fit the current content,
// starting at 1 line and growing as the user types.
func (p *promptView) recalcHeight() {
	taHeight := p.visualLineCount()
	maxHeight := p.maxHeight()
	if taHeight > maxHeight {
		taHeight = maxHeight
	}
	p.textarea.SetHeight(taHeight)
}

// visualLineCount returns the number of visual lines the current textarea
// content occupies, accounting for soft-wrapping at the content width.
func (p *promptView) visualLineCount() int {
	val := p.textarea.Value()
	if val == "" {
		return 1
	}

	// Content width is textarea width minus prompt characters.
	// The textarea SetWidth(w) sets internal content width to w - promptWidth.
	// We pass (width - 4), so content width = (width - 4) - promptWidth.
	promptWidth := lipgloss.Width(p.textarea.Prompt)
	contentWidth := (p.width - 4) - promptWidth
	if contentWidth < 1 {
		contentWidth = 1
	}

	lines := strings.Split(val, "\n")
	visual := 0
	for _, line := range lines {
		lineWidth := lipgloss.Width(line)
		if lineWidth == 0 {
			visual++
		} else {
			visual += (lineWidth + contentWidth - 1) / contentWidth
		}
	}
	return visual
}

func (p *promptView) Reset() {
	p.textarea.Reset()
	p.titleInput.Reset()
	p.branchInput.Reset()
	p.checkoutInput.Reset()
	p.targetBranchInput.Reset()
	// Keep worktree and branchMode state — persists across task creation within a session
	p.images = make([]string, 0)
	p.blockingTaskID = 0
	p.blockingTaskTitle = ""
	p.validationError = ""
	// Restore workflowCursor to the saved default workflow position
	p.workflowCursor = p.defaultWorkflowCursor()
	p.focusInput(promptFieldDescription)
	p.recalcHeight()
}

// defaultWorkflowCursor returns the index of the defaultWorkflow in p.workflows,
// falling back to 0 if not found or no default is set.
func (p *promptView) defaultWorkflowCursor() int {
	if p.defaultWorkflow == "" {
		return 0
	}
	for i, name := range p.workflows {
		if name == p.defaultWorkflow {
			return i
		}
	}
	return 0
}

func (p *promptView) Value() string {
	return strings.TrimSpace(p.textarea.Value())
}

func (p *promptView) TitleValue() string {
	return strings.TrimSpace(p.titleInput.Value())
}

func (p *promptView) BranchName() string {
	return strings.TrimSpace(p.branchInput.Value())
}

func (p *promptView) CheckoutBranch() string {
	return strings.TrimSpace(p.checkoutInput.Value())
}

func (p *promptView) TargetBranch() string {
	return strings.TrimSpace(p.targetBranchInput.Value())
}

func (p *promptView) Images() []string {
	return p.images
}

func (p *promptView) Worktree() bool {
	return p.worktree
}

// blurAll unfocuses every input widget.
func (p *promptView) blurAll() {
	p.textarea.Blur()
	p.titleInput.Blur()
	p.branchInput.Blur()
	p.checkoutInput.Blur()
	p.targetBranchInput.Blur()
}

// focusInput blurs all inputs, then focuses the one matching field.
func (p *promptView) focusInput(field promptField) {
	p.blurAll()
	p.activePane = paneTask
	p.focusField = field
	switch field {
	case promptFieldTitle:
		p.titleInput.Focus()
	case promptFieldDescription:
		p.textarea.Focus()
	case promptFieldBranch:
		p.branchInput.Focus()
	case promptFieldCheckout:
		p.checkoutInput.Focus()
	case promptFieldTargetBranch:
		p.targetBranchInput.Focus()
	}
}

// visibleFields returns the ordered list of tab-cyclable fields
// based on the current worktree and branch mode state.
func (p *promptView) visibleFields() []promptField {
	fields := []promptField{promptFieldTitle, promptFieldDescription}
	if p.worktree {
		if p.branchMode == branchModeNew {
			fields = append(fields, promptFieldBranch)
		} else {
			fields = append(fields, promptFieldCheckout)
		}
		fields = append(fields, promptFieldTargetBranch)
	}
	return fields
}

func (p *promptView) ToggleWorktree() {
	p.worktree = !p.worktree
	if !p.worktree && p.focusField != promptFieldDescription && p.focusField != promptFieldTitle {
		p.focusInput(promptFieldDescription)
	}
}

func (p *promptView) ToggleBranchMode() {
	if p.branchMode == branchModeNew {
		p.branchMode = branchModeExisting
	} else {
		p.branchMode = branchModeNew
	}
	p.focusInput(promptFieldDescription)
}

// Update passes the message to the active input and checks for image paths.
func (p *promptView) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch p.focusField {
	case promptFieldTitle:
		p.titleInput, cmd = p.titleInput.Update(msg)
	case promptFieldDescription:
		// Pre-expand textarea to max height so the internal viewport doesn't
		// scroll when content grows beyond the current height.
		maxHeight := p.maxHeight()
		p.textarea.SetHeight(maxHeight)
		p.textarea, cmd = p.textarea.Update(msg)
		p.detectImages()
		p.recalcHeight()
	case promptFieldBranch:
		p.branchInput, cmd = p.branchInput.Update(msg)
	case promptFieldCheckout:
		p.checkoutInput, cmd = p.checkoutInput.Update(msg)
	case promptFieldTargetBranch:
		p.targetBranchInput, cmd = p.targetBranchInput.Update(msg)
	}
	return cmd
}

// SwitchFocus cycles through the visible fields based on current branch mode.
// forward=true moves to the next field (Tab), forward=false moves to the previous (Shift+Tab).
// The cycle order includes the workflow pane (if multiple workflows exist) as a
// virtual stop after the last field and before the first.
func (p *promptView) SwitchFocus(forward bool) {
	fields := p.visibleFields()
	hasWorkflows := len(p.workflows) > 1

	// Workflow pane sits between the last and first field in the cycle.
	if p.activePane == paneWorkflow {
		if forward {
			p.focusInput(fields[0])
		} else {
			p.focusInput(fields[len(fields)-1])
		}
		return
	}

	// Find current field index.
	idx := -1
	for i, f := range fields {
		if f == p.focusField {
			idx = i
			break
		}
	}
	if idx == -1 {
		p.focusInput(fields[0])
		return
	}

	if forward {
		next := idx + 1
		if next >= len(fields) {
			if hasWorkflows {
				p.blurAll()
				p.activePane = paneWorkflow
			} else {
				p.focusInput(fields[0])
			}
		} else {
			p.focusInput(fields[next])
		}
	} else {
		prev := idx - 1
		if prev < 0 {
			if hasWorkflows {
				p.blurAll()
				p.activePane = paneWorkflow
			} else {
				p.focusInput(fields[len(fields)-1])
			}
		} else {
			p.focusInput(fields[prev])
		}
	}
}

// FocusOn jumps directly to the given field, switching panes if necessary.
func (p *promptView) FocusOn(field promptField) {
	p.focusInput(field)
}

// FocusWorkflowPane jumps directly to the workflow pane.
func (p *promptView) FocusWorkflowPane() {
	if len(p.workflows) <= 1 {
		return
	}
	p.blurAll()
	p.activePane = paneWorkflow
}

// FocusGitSection jumps to the first visible git field based on current mode.
// If worktree is off, this is a no-op.
func (p *promptView) FocusGitSection() {
	if !p.worktree {
		return
	}
	if p.branchMode == branchModeNew {
		p.FocusOn(promptFieldBranch)
	} else {
		p.FocusOn(promptFieldCheckout)
	}
}

// CyclePane cycles through sections: main inputs ↔ git ↔ workflow.
// Skips git if worktree is off, skips workflow if only one workflow.
// forward=true goes main→git→workflow; forward=false reverses.
func (p *promptView) CyclePane(forward bool) {
	isMainField := p.activePane == paneTask &&
		(p.focusField == promptFieldTitle || p.focusField == promptFieldDescription)
	isGitField := p.activePane == paneTask &&
		(p.focusField == promptFieldBranch || p.focusField == promptFieldCheckout || p.focusField == promptFieldTargetBranch)
	hasGit := p.worktree
	hasWorkflows := len(p.workflows) > 1

	// Order: main → git → workflow (forward), reverse for backward.
	// next/prev pick the adjacent section, wrapping around and skipping unavailable ones.
	switch {
	case isMainField:
		if forward {
			if hasGit {
				p.FocusGitSection()
			} else if hasWorkflows {
				p.FocusWorkflowPane()
			}
		} else {
			if hasWorkflows {
				p.FocusWorkflowPane()
			} else if hasGit {
				p.FocusGitSection()
			}
		}
	case isGitField:
		if forward {
			if hasWorkflows {
				p.FocusWorkflowPane()
			} else {
				p.FocusOn(promptFieldDescription)
			}
		} else {
			p.FocusOn(promptFieldDescription)
		}
	case p.activePane == paneWorkflow:
		if forward {
			p.FocusOn(promptFieldDescription)
		} else {
			if hasGit {
				p.FocusGitSection()
			} else {
				p.FocusOn(promptFieldDescription)
			}
		}
	}
}

// Focus focuses the currently active input.
func (p *promptView) Focus() {
	p.focusInput(p.focusField)
}

// Blur unfocuses all inputs.
func (p *promptView) Blur() {
	p.blurAll()
}

// RemoveLastImage removes the most recently attached image
func (p *promptView) RemoveLastImage() {
	if len(p.images) > 0 {
		p.images = p.images[:len(p.images)-1]
		p.SetSize(p.width, p.height)
	}
}

// detectImages checks each line in the textarea for image file paths.
// If a line is a valid path to an existing image file, it's extracted
// from the textarea and added to the images list.
func (p *promptView) detectImages() {
	val := p.textarea.Value()
	lines := strings.Split(val, "\n")
	var remaining []string
	changed := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			remaining = append(remaining, line)
			continue
		}

		if isImagePath(trimmed) {
			// Resolve home directory
			path := trimmed
			if strings.HasPrefix(path, "~") {
				if home, err := os.UserHomeDir(); err == nil {
					path = filepath.Join(home, path[1:])
				}
			}

			// Check if file exists and isn't already attached
			if _, err := os.Stat(path); err == nil && !p.hasImage(path) {
				p.images = append(p.images, path)
				changed = true
				p.SetSize(p.width, p.height) // recalc textarea height
				continue                      // don't add to remaining lines
			}
		}
		remaining = append(remaining, line)
	}

	if changed {
		newVal := strings.Join(remaining, "\n")
		// Trim trailing newlines that were left behind
		newVal = strings.TrimRight(newVal, "\n")
		p.textarea.SetValue(newVal)
	}
}

func (p *promptView) hasImage(path string) bool {
	for _, img := range p.images {
		if img == path {
			return true
		}
	}
	return false
}

// isImagePath checks if a string looks like a path to an image file.
func isImagePath(s string) bool {
	// Must look like a file path
	if !strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "~") && !strings.HasPrefix(s, ".") {
		return false
	}

	// Must not contain spaces typical of prose (allows paths with escaped spaces)
	// Actually, file paths can have spaces, so we just check extension
	ext := strings.ToLower(filepath.Ext(s))
	return imageExtensions[ext]
}

// renderWorkflowList renders the workflow selector list for the right pane.
func (p *promptView) renderWorkflowList() string {
	activeHighlight := selectedStyle
	inactiveHighlight := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#E8E8E8")).
		Background(lipgloss.Color("#3A3A3A"))

	var wf strings.Builder
	for i, name := range p.workflows {
		label := fmt.Sprintf("%d. %s", i+1, name)
		if i == p.workflowCursor {
			if p.activePane == paneWorkflow {
				wf.WriteString(activeHighlight.Render("> " + label))
			} else {
				wf.WriteString(inactiveHighlight.Render("  " + label))
			}
		} else {
			wf.WriteString("  " + label)
		}
		if i < len(p.workflows)-1 {
			wf.WriteString("\n")
		}
	}
	return wf.String()
}

func (p *promptView) View() string {
	var b strings.Builder

	focusedLabel := lipgloss.NewStyle().Bold(true).Foreground(highlight)
	unfocusedLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B6B6B"))

	// underlineMnemonic renders a label with its first character underlined as a keyboard hint.
	underlineMnemonic := func(label string, base lipgloss.Style) string {
		ul := base.Underline(true)
		return ul.Render(string(label[0])) + base.Render(label[1:])
	}

	fieldLabel := func(label string, field promptField, mnemonic bool) string {
		if p.focusField == field {
			if mnemonic {
				return focusedLabel.Render("▸ ") + underlineMnemonic(label, focusedLabel)
			}
			return focusedLabel.Render("▸ " + label)
		}
		if mnemonic {
			return unfocusedLabel.Render("  ") + underlineMnemonic(label, unfocusedLabel)
		}
		return unfocusedLabel.Render("  " + label)
	}

	// Git section frame — border brightens when it contains the focused field
	gitHasFocus := p.activePane == paneTask && (p.focusField == promptFieldBranch || p.focusField == promptFieldCheckout || p.focusField == promptFieldTargetBranch)

	activeBorder := lipgloss.Color("#5F8AB3")
	inactiveBorder := lipgloss.Color("#3A3A3A")

	gitBorderColor := inactiveBorder
	if gitHasFocus {
		gitBorderColor = activeBorder
	}

	// Inner width for framed sections (1-space left prefix applied in output)
	innerWidth := p.width - 1
	if innerWidth < 10 {
		innerWidth = 10
	}

	// Title bar with optional blocking indicator and workflow indicator
	titleText := " New Task "
	if p.blockingTaskID != 0 {
		blockInfo := fmt.Sprintf("#%d", p.blockingTaskID)
		if p.blockingTaskTitle != "" {
			blockInfo = fmt.Sprintf("#%d %s", p.blockingTaskID, p.blockingTaskTitle)
		}
		titleText = fmt.Sprintf(" New Task (blocks %s) ", blockInfo)
	}
	title := titleStyle.Render(titleText)
	b.WriteString(title)
	b.WriteString("\n\n")

	// Title input
	b.WriteString(fieldLabel("Title: ", promptFieldTitle, true))
	b.WriteString(p.titleInput.View())
	b.WriteString("\n\n")

	// Pre-expand textarea to max height so its internal viewport doesn't
	// scroll, then truncate the rendered output to show only the lines
	// that contain actual content, achieving the auto-grow visual effect.
	maxH := p.maxHeight()
	p.textarea.SetHeight(maxH)
	taView := p.textarea.View()
	p.recalcHeight() // restore to content-fitting height
	visLines := p.visualLineCount()
	if visLines > maxH {
		visLines = maxH
	}
	lines := strings.Split(taView, "\n")
	if visLines < len(lines) {
		lines = lines[:visLines]
	}

	// When the placeholder is showing, underline the first "D" in
	// "Describe the task..." to indicate the alt+d shortcut, mirroring the
	// underlined "G" in "Git" and "W" in "Workflow" labels.
	if p.textarea.Value() == "" && p.focusField == promptFieldDescription && len(lines) > 0 {
		lines[0] = underlineDInPlaceholder(lines[0])
	}

	taStyle := lipgloss.NewStyle().PaddingLeft(2)
	b.WriteString(taStyle.Render(strings.Join(lines, "\n")))
	b.WriteString("\n")

	// Validation error
	if p.validationError != "" {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#E88388"))
		b.WriteString("  " + errStyle.Render(p.validationError))
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// ── Git section (framed) ──
	var gitContent strings.Builder

	// Worktree mode indicator
	gitContent.WriteString(focusedLabel.Render("Worktree: "))
	if p.worktree {
		gitContent.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#7EC99D")).Render("on"))
	} else {
		gitContent.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#E88388")).Render("off"))
		gitContent.WriteString(dimStyle.Render(" (runs in current directory)"))
	}
	gitContent.WriteString(dimStyle.Render(" (alt+W)"))

	// Branch/Checkout inputs (hidden when worktree is off)
	if p.worktree {
		gitContent.WriteString("\n")
		// Mode indicator
		modeLabel := "new branch"
		if p.branchMode == branchModeExisting {
			modeLabel = "existing branch"
		}
		gitContent.WriteString(focusedLabel.Render("Mode: "))
		gitContent.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#A8C8E8")).Render(modeLabel))
		gitContent.WriteString(dimStyle.Render(" (alt+M)"))
		gitContent.WriteString("\n\n")

		if p.branchMode == branchModeNew {
			gitContent.WriteString(fieldLabel("Branch: ", promptFieldBranch, false))
			gitContent.WriteString(p.branchInput.View())
		} else {
			gitContent.WriteString(fieldLabel("Checkout: ", promptFieldCheckout, false))
			gitContent.WriteString(p.checkoutInput.View())
		}

		gitContent.WriteString("\n")
		gitContent.WriteString(fieldLabel("Target: ", promptFieldTargetBranch, false))
		gitContent.WriteString(p.targetBranchInput.View())
	}

	if len(p.workflows) > 1 {
		workflowContent := p.renderWorkflowList()
		workflowBorderColor := inactiveBorder
		if p.activePane == paneWorkflow {
			workflowBorderColor = activeBorder
		}

		const minSideBySide = 60
		if p.width >= minSideBySide {
			leftWidth := innerWidth * 2 / 3
			rightWidth := innerWidth - leftWidth // no gap — frames share the border
			gitFrame := p.renderFramedSection("Git", gitBorderColor, gitContent.String(), leftWidth, true)
			wfFrame := p.renderFramedSection("Workflow", workflowBorderColor, workflowContent, rightWidth, true)
			b.WriteString(indentBlock(" ", joinFramesHorizontal(gitFrame, wfFrame, leftWidth, rightWidth)))
		} else {
			b.WriteString(indentBlock(" ", p.renderFramedSection("Git", gitBorderColor, gitContent.String(), innerWidth, true)))
			b.WriteString("\n")
			b.WriteString(indentBlock(" ", p.renderFramedSection("Workflow", workflowBorderColor, workflowContent, innerWidth, true)))
		}
	} else {
		b.WriteString(indentBlock(" ", p.renderFramedSection("Git", gitBorderColor, gitContent.String(), innerWidth, true)))
	}
	b.WriteString("\n")

	// Attached images
	if len(p.images) > 0 {
		b.WriteString("  ")
		b.WriteString(focusedLabel.Render("Attached images:"))
		b.WriteString("\n")

		imgStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#7EC99D"))
		for _, img := range p.images {
			name := filepath.Base(img)
			b.WriteString("  ")
			b.WriteString(imgStyle.Render("  ■ " + name))
			b.WriteString(dimStyle.Render(" (" + img + ")"))
			b.WriteString("\n")
		}
	}

	// Help
	b.WriteString("\n")
	b.WriteString(p.renderHelp())

	return b.String()
}

// renderFramedSection renders content inside a rounded border with a label
// embedded in the top border line: ╭─ Label ────────────╮
// Width is the total outer width including border characters.
// When mnemonic is true, the first character of the label is underlined as a keyboard hint.
func (p *promptView) renderFramedSection(label string, borderColor lipgloss.Color, content string, width int, mnemonic bool) string {
	bs := lipgloss.NewStyle().Foreground(borderColor)
	ls := lipgloss.NewStyle().Bold(true).Foreground(borderColor)

	// Inner content width = outer - │(1) - space(1) - │(1)
	contentWidth := width - 3
	if contentWidth < 1 {
		contentWidth = 1
	}

	// ── Top border: ╭─ Label ─────╮ ──
	var labelText string
	if mnemonic && len(label) > 0 {
		ul := ls.Underline(true)
		labelText = ls.Render(" ") + ul.Render(string(label[0])) + ls.Render(label[1:]+" ")
	} else {
		labelText = ls.Render(" " + label + " ")
	}
	labelW := lipgloss.Width(labelText)
	topFill := width - 2 - labelW - 1 // 2 for "╭─", 1 for "╮"
	if topFill < 0 {
		topFill = 0
	}
	top := bs.Render("╭─") + labelText + bs.Render(strings.Repeat("─", topFill)+"╮")

	// ── Content lines with side borders: │ content  │ ──
	var out strings.Builder
	out.WriteString(top)
	out.WriteByte('\n')

	// Top padding inside the frame
	out.WriteString(bs.Render("│") + " " + strings.Repeat(" ", contentWidth) + bs.Render("│"))
	out.WriteByte('\n')

	truncStyle := lipgloss.NewStyle().MaxWidth(contentWidth)
	for _, line := range strings.Split(content, "\n") {
		lw := lipgloss.Width(line)
		if lw > contentWidth {
			line = truncStyle.Render(line)
			lw = contentWidth
		}
		pad := contentWidth - lw
		out.WriteString(bs.Render("│") + " " + line + strings.Repeat(" ", pad) + bs.Render("│"))
		out.WriteByte('\n')
	}

	// ── Bottom border: ╰─────╯ ──
	botFill := width - 2 // 1 for "╰", 1 for "╯"
	if botFill < 0 {
		botFill = 0
	}
	out.WriteString(bs.Render("╰" + strings.Repeat("─", botFill) + "╯"))

	return out.String()
}

// underlineDInPlaceholder injects ANSI underline codes around the first 'D'
// byte in the textarea's rendered placeholder line. This indicates the alt+d
// keyboard shortcut, matching the underlined "G" in "Git" and "W" in
// "Workflow" labels. The injection works regardless of cursor blink state
// because the underline codes (CSI 4m / CSI 24m) compose with whatever SGR
// sequence the textarea wraps "D" with.
func underlineDInPlaceholder(line string) string {
	idx := strings.IndexByte(line, 'D')
	if idx < 0 {
		return line
	}
	return line[:idx] + "\x1b[4mD\x1b[24m" + line[idx+1:]
}

// indentBlock prepends prefix to every line of a multi-line string.
func indentBlock(prefix, block string) string {
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

// joinFramesHorizontal joins two frame strings side-by-side, forcing each line
// of the left frame to exactly leftWidth and the right frame to rightWidth.
// This avoids lipgloss.JoinHorizontal's max-width padding that causes glitches
// when ANSI-styled content has inconsistent visual widths.
func joinFramesHorizontal(left, right string, leftWidth, rightWidth int) string {
	leftLines := strings.Split(left, "\n")
	rightLines := strings.Split(right, "\n")

	// Equalize line counts
	maxLines := len(leftLines)
	if len(rightLines) > maxLines {
		maxLines = len(rightLines)
	}
	for len(leftLines) < maxLines {
		leftLines = append(leftLines, "")
	}
	for len(rightLines) < maxLines {
		rightLines = append(rightLines, "")
	}

	var out strings.Builder
	for i := range leftLines {
		// Pad or truncate left line to exactly leftWidth
		lw := lipgloss.Width(leftLines[i])
		if lw < leftWidth {
			leftLines[i] += strings.Repeat(" ", leftWidth-lw)
		} else if lw > leftWidth {
			leftLines[i] = lipgloss.NewStyle().MaxWidth(leftWidth).Render(leftLines[i])
		}
		if i > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(leftLines[i])
		out.WriteString(rightLines[i])
	}
	return out.String()
}

func (p *promptView) renderHelp() string {
	var help strings.Builder
	help.WriteString(helpStyle.Render("  "))

	bindings := cachedPromptKeyMap.ShortHelp()
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
