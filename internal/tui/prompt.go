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
	promptFieldDescription  promptField = iota
	promptFieldBranch
	promptFieldCheckout
	promptFieldTargetBranch
)

type branchMode int

const (
	branchModeNew      branchMode = iota // create new branch (default)
	branchModeExisting                    // checkout existing branch
)

type promptView struct {
	textarea          textarea.Model
	branchInput       textinput.Model
	checkoutInput     textinput.Model
	targetBranchInput textinput.Model
	focusField        promptField
	worktree          bool
	branchMode        branchMode
	defaultBaseBranch string
	images            []string
	workflowName      string
	blockingTaskID    int64 // when non-zero, new task blocks this task
	width             int
	height            int
	showHelp          bool
}

func newPromptView(defaultWorktree bool, defaultBaseBranch string) promptView {
	ta := textarea.New()
	ta.Prompt = PromptPrefix
	ta.Placeholder = "Describe the task..."
	ta.Focus()
	ta.CharLimit = 0 // unlimited
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("ctrl+j"), key.WithHelp("ctrl+j", "new line"))
	ta.KeyMap.WordForward = key.NewBinding(key.WithKeys("alt+right", "ctrl+right", "alt+f"), key.WithHelp("ctrl+right", "word forward"))
	ta.KeyMap.WordBackward = key.NewBinding(key.WithKeys("alt+left", "ctrl+left", "alt+b"), key.WithHelp("ctrl+left", "word backward"))

	bi := textinput.New()
	bi.Placeholder = "optional, e.g. feature/{{task.title}}"
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
		branchInput:       bi,
		checkoutInput:     ci,
		targetBranchInput: ti,
		focusField:        promptFieldDescription,
		worktree:          defaultWorktree,
		defaultBaseBranch: defaultBaseBranch,
		images:            make([]string, 0),
	}
}

func (p *promptView) SetSize(width, height int) {
	p.width = width
	p.height = height
	p.textarea.SetWidth(width - 4)
	p.branchInput.Width = width - 4 - lipgloss.Width("Branch: ")
	p.checkoutInput.Width = width - 4 - lipgloss.Width("Checkout: ")
	p.targetBranchInput.Width = width - 4 - lipgloss.Width("Target: ")
	p.recalcHeight()
}

// maxHeight returns the maximum textarea height available within the terminal.
func (p *promptView) maxHeight() int {
	h := p.height - 12 // reserve space for title(2) + worktree(2) + branch/checkout(2) + target(2) + images + help(2)
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
	p.branchInput.Reset()
	p.checkoutInput.Reset()
	p.targetBranchInput.Reset()
	// Keep worktree and branchMode state — persists across task creation within a session
	p.images = make([]string, 0)
	p.blockingTaskID = 0
	p.focusField = promptFieldDescription
	p.textarea.Focus()
	p.branchInput.Blur()
	p.checkoutInput.Blur()
	p.targetBranchInput.Blur()
	p.recalcHeight()
}

func (p *promptView) Value() string {
	return strings.TrimSpace(p.textarea.Value())
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

func (p *promptView) ToggleWorktree() {
	p.worktree = !p.worktree
	if !p.worktree && p.focusField != promptFieldDescription {
		p.focusField = promptFieldDescription
		p.branchInput.Blur()
		p.checkoutInput.Blur()
		p.targetBranchInput.Blur()
		p.textarea.Focus()
	}
}

func (p *promptView) ToggleBranchMode() {
	if p.branchMode == branchModeNew {
		p.branchMode = branchModeExisting
	} else {
		p.branchMode = branchModeNew
	}
	// Reset focus to description when switching modes
	p.focusField = promptFieldDescription
	p.textarea.Focus()
	p.branchInput.Blur()
	p.checkoutInput.Blur()
	p.targetBranchInput.Blur()
}

// Update passes the message to the active input and checks for image paths.
func (p *promptView) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch p.focusField {
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
// When worktree mode is off, branch inputs are hidden so tab is a no-op.
func (p *promptView) SwitchFocus() {
	if !p.worktree {
		return // branch inputs are hidden
	}

	// Blur all
	p.textarea.Blur()
	p.branchInput.Blur()
	p.checkoutInput.Blur()
	p.targetBranchInput.Blur()

	if p.branchMode == branchModeNew {
		// Cycle: description → branch → targetBranch → description
		switch p.focusField {
		case promptFieldDescription:
			p.focusField = promptFieldBranch
			p.branchInput.Focus()
		case promptFieldBranch:
			p.focusField = promptFieldTargetBranch
			p.targetBranchInput.Focus()
		default:
			p.focusField = promptFieldDescription
			p.textarea.Focus()
		}
	} else {
		// Cycle: description → checkout → targetBranch → description
		switch p.focusField {
		case promptFieldDescription:
			p.focusField = promptFieldCheckout
			p.checkoutInput.Focus()
		case promptFieldCheckout:
			p.focusField = promptFieldTargetBranch
			p.targetBranchInput.Focus()
		default:
			p.focusField = promptFieldDescription
			p.textarea.Focus()
		}
	}
}

// Focus focuses the currently active input
func (p *promptView) Focus() {
	switch p.focusField {
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

// Blur unfocuses all inputs
func (p *promptView) Blur() {
	p.textarea.Blur()
	p.branchInput.Blur()
	p.checkoutInput.Blur()
	p.targetBranchInput.Blur()
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

func (p *promptView) View() string {
	var b strings.Builder

	// Title with optional blocking indicator and workflow indicator
	titleText := " New Task "
	if p.blockingTaskID != 0 {
		titleText = fmt.Sprintf(" New Task (blocks #%d) ", p.blockingTaskID)
	}
	title := titleStyle.Render(titleText)
	if p.workflowName != "" && p.width > 0 {
		workflowWidget := projectIndicatorStyle.Render("[" + p.workflowName + "]")
		gap := p.width - lipgloss.Width(title) - lipgloss.Width(workflowWidget)
		if gap < 0 {
			gap = 0
		}
		b.WriteString(title + strings.Repeat(" ", gap) + workflowWidget)
	} else {
		b.WriteString(title)
	}
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

	taStyle := lipgloss.NewStyle().PaddingLeft(2)
	b.WriteString(taStyle.Render(strings.Join(lines, "\n")))
	b.WriteString("\n")

	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(highlight)

	// Worktree mode indicator
	b.WriteString("\n")
	b.WriteString("  ")
	b.WriteString(labelStyle.Render("Worktree: "))
	if p.worktree {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#7EC99D")).Render("on"))
	} else {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#E88388")).Render("off"))
		b.WriteString(dimStyle.Render(" (runs in current directory)"))
	}
	b.WriteString("\n")

	// Branch/Checkout inputs (hidden when worktree is off)
	if p.worktree {
		// Mode indicator
		b.WriteString("  ")
		modeLabel := "new"
		if p.branchMode == branchModeExisting {
			modeLabel = "existing"
		}
		b.WriteString(labelStyle.Render("Mode: "))
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#A8C8E8")).Render(modeLabel))
		b.WriteString(dimStyle.Render(" (alt+m)"))
		b.WriteString("\n")

		if p.branchMode == branchModeNew {
			// New branch mode: show branch template input
			b.WriteString("\n")
			b.WriteString("  ")
			b.WriteString(labelStyle.Render("Branch: "))
			b.WriteString(p.branchInput.View())
			b.WriteString("\n")
		} else {
			// Existing branch mode: show checkout input
			b.WriteString("\n")
			b.WriteString("  ")
			b.WriteString(labelStyle.Render("Checkout: "))
			b.WriteString(p.checkoutInput.View())
			b.WriteString("\n")
		}

		// Target branch (shown in both modes)
		b.WriteString("\n")
		b.WriteString("  ")
		b.WriteString(labelStyle.Render("Target: "))
		b.WriteString(p.targetBranchInput.View())
		b.WriteString("\n")
	}

	// Attached images
	if len(p.images) > 0 {
		b.WriteString("\n")
		b.WriteString("  ")
		b.WriteString(labelStyle.Render("Attached images:"))
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
