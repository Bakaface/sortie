package tui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	subtle    = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}
	highlight = lipgloss.AdaptiveColor{Light: "#4078C0", Dark: "#6CA0DC"}
	special   = lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"}

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(highlight).
			Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(highlight).
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(subtle)

	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(highlight)

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FAFAFA"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626262"))

	stateStyles = map[string]lipgloss.Style{
		"pending":           lipgloss.NewStyle().Foreground(lipgloss.Color("#626262")),
		"init":              lipgloss.NewStyle().Foreground(lipgloss.Color("#FFCC00")).Italic(true),
		"starting":          lipgloss.NewStyle().Foreground(lipgloss.Color("#FFCC00")),
		"running":           lipgloss.NewStyle().Foreground(lipgloss.Color("#73F59F")),
		"waiting_for_input": lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B")).Bold(true),
		"awaiting-approval": lipgloss.NewStyle().Foreground(lipgloss.Color("#FFCC00")).Bold(true),
		"tmux":              lipgloss.NewStyle().Foreground(lipgloss.Color("#FF69B4")).Bold(true),
		"completed":         lipgloss.NewStyle().Foreground(lipgloss.Color("#43BF6D")),
		"failed":            lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")),
		"stopped":           lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")),
	}

	helpStyle = lipgloss.NewStyle().
			Foreground(dimStyle.GetForeground())

	viewportStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(subtle).
			Padding(0, 1)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#343433", Dark: "#C1C6B2"}).
			Background(lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#353533"})
)

func stateStyle(state string) lipgloss.Style {
	if style, ok := stateStyles[state]; ok {
		return style
	}
	return normalStyle
}
