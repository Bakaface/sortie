package tui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	subtle    = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}
	highlight = lipgloss.AdaptiveColor{Light: "#4078C0", Dark: "#6CA0DC"}

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
		"summarizing":       lipgloss.NewStyle().Foreground(lipgloss.Color("#6CA0DC")),
		"completed":         lipgloss.NewStyle().Foreground(lipgloss.Color("#43BF6D")),
		"failed":            lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")),
		"stopped":           lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")),
		"artifact-missing":  lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B")).Bold(true),
	}

	priorityStyles = map[string]lipgloss.Style{
		"low":    lipgloss.NewStyle().Foreground(lipgloss.Color("#626262")),
		"medium": lipgloss.NewStyle().Foreground(lipgloss.Color("#6CA0DC")),
		"high":   lipgloss.NewStyle().Foreground(lipgloss.Color("#FFCC00")),
		"urgent": lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Bold(true),
	}

	lineNumStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626262"))

	helpStyle = lipgloss.NewStyle().
			Foreground(dimStyle.GetForeground())

	viewportStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(subtle).
			Padding(0, 1)

	subHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FAFAFA"))
)

func stateStyle(state string) lipgloss.Style {
	if style, ok := stateStyles[state]; ok {
		return style
	}
	return normalStyle
}

func priorityStyle(priority string) lipgloss.Style {
	if style, ok := priorityStyles[priority]; ok {
		return style
	}
	return normalStyle
}

func priorityBadge(priority string) string {
	switch priority {
	case "urgent":
		return "U"
	case "high":
		return "H"
	case "medium":
		return "M"
	case "low":
		return "L"
	default:
		return "M"
	}
}
