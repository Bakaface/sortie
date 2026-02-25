package tui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
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

type promptView struct {
	textarea textarea.Model
	images   []string
	width    int
	height   int
}

func newPromptView() promptView {
	ta := textarea.New()
	ta.Prompt = PromptPrefix
	ta.Placeholder = "Describe the task..."
	ta.Focus()
	ta.CharLimit = 0 // unlimited
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("ctrl+j"), key.WithHelp("ctrl+j", "new line"))
	ta.KeyMap.WordForward = key.NewBinding(key.WithKeys("alt+right", "ctrl+right", "alt+f"), key.WithHelp("ctrl+right", "word forward"))
	ta.KeyMap.WordBackward = key.NewBinding(key.WithKeys("alt+left", "ctrl+left", "alt+b"), key.WithHelp("ctrl+left", "word backward"))
	return promptView{
		textarea: ta,
		images:   make([]string, 0),
	}
}

func (p *promptView) SetSize(width, height int) {
	p.width = width
	p.height = height
	p.textarea.SetWidth(width - 4)
	p.recalcHeight()
}

// recalcHeight adjusts the textarea height to fit the current content,
// starting at 1 line and growing as the user types.
func (p *promptView) recalcHeight() {
	taHeight := p.visualLineCount()
	maxHeight := p.height - 6
	if maxHeight < 1 {
		maxHeight = 1
	}
	if taHeight > maxHeight {
		taHeight = maxHeight
	}
	if taHeight < 1 {
		taHeight = 1
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
	p.images = make([]string, 0)
	p.textarea.Focus()
	p.recalcHeight()
}

func (p *promptView) Value() string {
	return strings.TrimSpace(p.textarea.Value())
}

func (p *promptView) Images() []string {
	return p.images
}

// Update passes the message to the textarea and checks for image paths.
func (p *promptView) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	p.textarea, cmd = p.textarea.Update(msg)
	p.detectImages()
	p.recalcHeight()
	return cmd
}

// Focus focuses the textarea
func (p *promptView) Focus() {
	p.textarea.Focus()
}

// Blur unfocuses the textarea
func (p *promptView) Blur() {
	p.textarea.Blur()
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

	// Title
	b.WriteString(titleStyle.Render(" New Task "))
	b.WriteString("\n\n")

	// Textarea (use lipgloss padding for consistent left margin on all lines)
	taStyle := lipgloss.NewStyle().PaddingLeft(2)
	b.WriteString(taStyle.Render(p.textarea.View()))
	b.WriteString("\n")

	// Attached images
	if len(p.images) > 0 {
		b.WriteString("\n")
		labelStyle := lipgloss.NewStyle().Bold(true).Foreground(highlight)
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
	b.WriteString(helpStyle.Render("  "))
	b.WriteString(dimStyle.Render("enter"))
	b.WriteString(helpStyle.Render(" submit"))
	b.WriteString(helpStyle.Render(" | "))
	b.WriteString(dimStyle.Render("ctrl+j"))
	b.WriteString(helpStyle.Render(" newline"))
	b.WriteString(helpStyle.Render(" | "))
	b.WriteString(dimStyle.Render("esc"))
	b.WriteString(helpStyle.Render(" cancel"))
	if len(p.images) > 0 {
		b.WriteString(helpStyle.Render(" | "))
		b.WriteString(dimStyle.Render("ctrl+x"))
		b.WriteString(helpStyle.Render(" remove last image"))
	}
	b.WriteString(helpStyle.Render(" | "))
	b.WriteString(dimStyle.Render("ctrl+g"))
	b.WriteString(helpStyle.Render(" editor"))
	b.WriteString(helpStyle.Render(" | "))
	b.WriteString(dimStyle.Render("paste image path"))
	b.WriteString(helpStyle.Render(" to attach"))

	return b.String()
}
