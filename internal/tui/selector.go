package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// selectorKind identifies which selection dialog is active.
type selectorKind int

const (
	selectorNone selectorKind = iota
	selectorWorkflow
	selectorContinueWorkflow
	selectorTask
	selectorInit
	selectorPriority
	selectorArtifact
)

// selectionResult is returned by selector.HandleKey to signal what happened.
type selectionResult int

const (
	selNone      selectionResult = iota // key consumed, no action
	selChosen                           // user picked an item
	selCancelled                        // user cancelled
)

// selector is a generic list picker that handles vim-style navigation,
// number-key quick select, and rendering.
type selector struct {
	kind         selectorKind
	title        string
	items        []string
	cursor       int
	pendingG     bool
	descriptions []string                         // optional; shown under selected item
	itemStyle    func(name string) lipgloss.Style // optional; per-item coloring for non-selected
	hint         string                           // footer override; empty uses default
	// Auxiliary data for the on-select callback
	taskID int64  // for priority, artifact
	action string // for artifact ("view"/"edit")
}

func (s *selector) IsActive() bool {
	return s.kind != selectorNone
}

func (s *selector) Reset() {
	*s = selector{}
}

func (s *selector) Selected() string {
	if s.cursor >= 0 && s.cursor < len(s.items) {
		return s.items[s.cursor]
	}
	return ""
}

// HandleKey processes a key press and returns the result.
func (s *selector) HandleKey(keyStr string) selectionResult {
	// Handle "gg" sequence for go-to-top
	if keyStr == "g" {
		if s.pendingG {
			s.pendingG = false
			s.cursor = 0
			return selNone
		}
		s.pendingG = true
		return selNone
	}
	s.pendingG = false

	switch keyStr {
	case "up", "k":
		if s.cursor > 0 {
			s.cursor--
		}
	case "down", "j":
		if s.cursor < len(s.items)-1 {
			s.cursor++
		}
	case "G":
		s.cursor = max(0, len(s.items)-1)
	case "ctrl+d", "pgdown":
		half := max(1, len(s.items)/2)
		s.cursor = min(s.cursor+half, len(s.items)-1)
	case "ctrl+u", "pgup":
		half := max(1, len(s.items)/2)
		s.cursor = max(s.cursor-half, 0)
	case "enter":
		return selChosen
	case "esc", "q":
		return selCancelled
	default:
		// Number keys for quick selection (1-9)
		if len(keyStr) == 1 && keyStr[0] >= '1' && keyStr[0] <= '9' {
			idx := int(keyStr[0] - '1')
			if idx < len(s.items) {
				s.cursor = idx
				return selChosen
			}
		}
	}
	return selNone
}

// View renders the selection dialog.
func (s *selector) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(" "+s.title+" ") + "\n\n")

	for i, name := range s.items {
		label := fmt.Sprintf("  %d. %s", i+1, name)
		if i == s.cursor {
			b.WriteString(selectedStyle.Render("> "+label) + "\n")
			if s.descriptions != nil && i < len(s.descriptions) && s.descriptions[i] != "" {
				b.WriteString(dimStyle.Render("     "+s.descriptions[i]) + "\n")
			}
		} else {
			if s.itemStyle != nil {
				b.WriteString("    " + s.itemStyle(name).Render(label) + "\n")
			} else {
				b.WriteString("    " + label + "\n")
			}
		}
	}

	hint := "j/k: navigate | enter: select | 1-9: quick select | esc: cancel"
	if s.hint != "" {
		hint = s.hint
	}
	b.WriteString("\n" + dimStyle.Render("  "+hint))
	return b.String()
}
