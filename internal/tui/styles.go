package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// AppTitle is the display title for Sortie, with an airplane (✈) reflecting
// the aviation origin of the word "sortie" — a short combat mission.
const AppTitle = "✈ Sortie"

// PromptPrefix is the airplane character used as a prompt prefix for task input,
// similar to Claude Code's ❯ character.
const PromptPrefix = "✈ "

var (
	subtle    = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#3A3A3A"}
	highlight = lipgloss.AdaptiveColor{Light: "#2D7A4F", Dark: "#5BA87A"}

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#E8E8E8")).
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
			Foreground(lipgloss.Color("#E8E8E8")).
			Background(highlight)

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#E8E8E8"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B6B6B"))

	stateStyles = map[string]lipgloss.Style{
		"pending":           lipgloss.NewStyle().Foreground(lipgloss.Color("#6B6B6B")),
		"init":              lipgloss.NewStyle().Foreground(lipgloss.Color("#D4A843")).Italic(true),
		"starting":          lipgloss.NewStyle().Foreground(lipgloss.Color("#D4A843")),
		"running":           lipgloss.NewStyle().Foreground(lipgloss.Color("#D4A843")),
		"waiting_for_input": lipgloss.NewStyle().Foreground(lipgloss.Color("#C97054")).Bold(true),
		"awaiting-approval": lipgloss.NewStyle().Foreground(lipgloss.Color("#D4A843")).Bold(true),
		"tmux":              lipgloss.NewStyle().Foreground(lipgloss.Color("#B07AAD")).Bold(true),
		"finalizing":        lipgloss.NewStyle().Foreground(lipgloss.Color("#5BA87A")),
		"summarizing":       lipgloss.NewStyle().Foreground(lipgloss.Color("#5BA87A")),
		"merge-blocked":     lipgloss.NewStyle().Foreground(lipgloss.Color("#C97054")).Bold(true),
		"completed":         lipgloss.NewStyle().Foreground(lipgloss.Color("#5BA87A")),
		"failed":            lipgloss.NewStyle().Foreground(lipgloss.Color("#D94F4F")),
		"stopped":           lipgloss.NewStyle().Foreground(lipgloss.Color("#7A7A7A")),
		"artifact-missing":  lipgloss.NewStyle().Foreground(lipgloss.Color("#C97054")).Bold(true),
	}

	priorityStyles = map[string]lipgloss.Style{
		"low":    lipgloss.NewStyle().Foreground(lipgloss.Color("#6B6B6B")),
		"medium": lipgloss.NewStyle().Foreground(lipgloss.Color("#5BA87A")),
		"high":   lipgloss.NewStyle().Foreground(lipgloss.Color("#D4A843")),
		"urgent": lipgloss.NewStyle().Foreground(lipgloss.Color("#D94F4F")).Bold(true),
	}

	lineNumStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B6B6B"))

	helpStyle = lipgloss.NewStyle().
			Foreground(dimStyle.GetForeground())

	viewportStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(subtle).
			Padding(0, 1)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#3A3A3A", Dark: "#B0B0A8"}).
			Background(lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#333333"})

	searchMatchStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#3A3A3A")).
				Foreground(lipgloss.Color("#E8E8E8"))

	projectIndicatorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#6B6B6B"))

	subHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E8E8E8"))
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
