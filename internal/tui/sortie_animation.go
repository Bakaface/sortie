package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type sortieTickMsg time.Time

func sortieTickCmd() tea.Cmd {
	return tea.Tick(60*time.Millisecond, func(t time.Time) tea.Msg {
		return sortieTickMsg(t)
	})
}

type planeState struct {
	x          int
	y          int
	startDelay int
}

type sortieAnimation struct {
	planes []planeState
	width  int
	height int
	frame  int
	done   bool
}

func newSortieAnimation(planeCount, width, height int) sortieAnimation {
	planes := make([]planeState, planeCount)

	// Center the group of planes vertically, spaced 2 rows apart
	totalRows := (planeCount-1)*2 + 1
	startY := (height - totalRows) / 2
	if startY < 0 {
		startY = 0
	}

	for i := range planes {
		planes[i] = planeState{
			x:          2,
			y:          startY + i*2,
			startDelay: i * 2,
		}
	}

	return sortieAnimation{
		planes: planes,
		width:  width,
		height: height,
		frame:  0,
		done:   false,
	}
}

func (a sortieAnimation) Update() sortieAnimation {
	a.frame++

	allDone := true
	for i := range a.planes {
		if a.frame >= a.planes[i].startDelay {
			a.planes[i].x += 2
		}
		if a.planes[i].x <= a.width {
			allDone = false
		}
	}

	if allDone {
		a.done = true
	}

	return a
}

func (a sortieAnimation) View() string {
	if a.width == 0 || a.height == 0 {
		return ""
	}

	// Build a grid of spaces
	rows := make([][]rune, a.height)
	for i := range rows {
		rows[i] = []rune(strings.Repeat(" ", a.width))
	}

	planeStyle := lipgloss.NewStyle().Foreground(highlight)
	trailSolidStyle := lipgloss.NewStyle().Foreground(highlight)
	trailFadeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B6B6B"))

	var sb strings.Builder

	for row := 0; row < a.height; row++ {
		if row > 0 {
			sb.WriteByte('\n')
		}

		// Check if any plane occupies this row
		planeOnRow := -1
		for i, p := range a.planes {
			if p.y == row && a.frame >= p.startDelay {
				planeOnRow = i
				break
			}
		}

		if planeOnRow == -1 {
			// No plane on this row — output blank line
			sb.WriteString(string(rows[row]))
			continue
		}

		p := a.planes[planeOnRow]

		// Build the line with trail + plane overlaid
		// We work left-to-right and emit styled segments

		// Trail positions (relative to plane):
		//   x-1, x-2, x-3 => solid '─'
		//   x-4, x-5, x-6 => fading '·'
		// The plane sits at p.x

		line := make([]byte, a.width)
		for i := range line {
			line[i] = ' '
		}

		// Write the line segment by segment for efficiency
		// Find: leading spaces, fade trail, solid trail, plane, trailing spaces

		fadeStart := p.x - 6
		solidStart := p.x - 3
		planePos := p.x

		// Clamp
		if fadeStart < 0 {
			fadeStart = 0
		}
		if solidStart < 0 {
			solidStart = 0
		}

		// Emit leading spaces up to fade trail
		if fadeStart > 0 {
			sb.WriteString(strings.Repeat(" ", fadeStart))
		}

		// Emit fade trail
		fadeCount := solidStart - fadeStart
		if fadeCount > 0 && fadeStart < a.width {
			end := solidStart
			if end > a.width {
				end = a.width
			}
			sb.WriteString(trailFadeStyle.Render(strings.Repeat("·", end-fadeStart)))
		}

		// Emit solid trail
		solidCount := planePos - solidStart
		if solidCount > 0 && solidStart < a.width {
			end := planePos
			if end > a.width {
				end = a.width
			}
			sb.WriteString(trailSolidStyle.Render(strings.Repeat("─", end-solidStart)))
		}

		// Emit plane character
		if planePos >= 0 && planePos < a.width {
			sb.WriteString(planeStyle.Render("✈"))
		}

		// Emit trailing spaces
		trailingStart := planePos + 1
		if trailingStart < 0 {
			trailingStart = 0
		}
		if trailingStart < a.width {
			sb.WriteString(strings.Repeat(" ", a.width-trailingStart))
		}

		_ = line // unused now, but kept for clarity
	}

	return sb.String()
}
