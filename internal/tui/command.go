package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// command represents a registered command that can be executed in command mode.
type command struct {
	// match returns true if the input matches this command, returning any arguments.
	match func(input string) (args string, ok bool)
	// exec executes the command with the given arguments, returning updated model and cmd.
	exec func(m Model, args string) (tea.Model, tea.Cmd)
	// help is a short description for the command.
	help string
}

// commands is the registry of all available commands.
var commands = []command{
	{
		match: matchLineNumber,
		exec:  execGotoLine,
		help:  "go to line number",
	},
	{
		match: matchSetNumber,
		exec:  execSetNumber,
		help:  "toggle line numbers",
	},
	{
		match: matchSetFinished,
		exec:  execSetFinished,
		help:  "toggle finished tasks",
	},
	{
		match: matchNoh,
		exec:  execNoh,
		help:  "clear search highlights",
	},
}

// matchLineNumber matches a bare positive number input (e.g. "3", "12").
func matchLineNumber(input string) (string, bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", false
	}
	if n, err := strconv.Atoi(input); err == nil && n > 0 {
		return input, true
	}
	return "", false
}

// execGotoLine jumps the cursor to the given 1-based line number.
func execGotoLine(m Model, args string) (tea.Model, tea.Cmd) {
	n, _ := strconv.Atoi(args)
	// Convert from 1-based (displayed) to 0-based (internal)
	m.list.GotoIndex(n - 1)
	return m, nil
}

// matchSetNumber matches "set number", "set nonumber", and "set number!" commands.
func matchSetNumber(input string) (string, bool) {
	input = strings.TrimSpace(input)
	switch input {
	case "set number", "set nonumber", "set number!":
		return input, true
	}
	return "", false
}

// execSetNumber enables, disables, or toggles line numbers in the list view.
func execSetNumber(m Model, args string) (tea.Model, tea.Cmd) {
	switch args {
	case "set number":
		m.list.showLineNumbers = true
	case "set nonumber":
		m.list.showLineNumbers = false
	case "set number!":
		m.list.showLineNumbers = !m.list.showLineNumbers
	}
	return m, nil
}

// matchSetFinished matches "set finished", "set nofinished", and "set finished!" commands.
func matchSetFinished(input string) (string, bool) {
	input = strings.TrimSpace(input)
	switch input {
	case "set finished", "set nofinished", "set finished!":
		return input, true
	}
	return "", false
}

// execSetFinished enables, disables, or toggles display of finished tasks in the list view.
func execSetFinished(m Model, args string) (tea.Model, tea.Cmd) {
	switch args {
	case "set finished":
		m.list.showFinished = true
	case "set nofinished":
		m.list.showFinished = false
	case "set finished!":
		m.list.showFinished = !m.list.showFinished
	}
	m.list.applyFilter()
	return m, nil
}

// matchNoh matches the "noh" or "nohlsearch" commands.
func matchNoh(input string) (string, bool) {
	input = strings.TrimSpace(input)
	if input == "noh" || input == "nohlsearch" {
		return input, true
	}
	return "", false
}

// execNoh clears search highlights.
func execNoh(m Model, _ string) (tea.Model, tea.Cmd) {
	m.list.matchedIndices = nil
	m.list.currentMatchIdx = 0
	m.searchQuery = ""
	return m, nil
}

// executeCommand finds and runs the first matching command for the given input.
func executeCommand(m Model, input string) (tea.Model, tea.Cmd) {
	input = strings.TrimSpace(input)
	if input == "" {
		return m, nil
	}
	for _, cmd := range commands {
		if args, ok := cmd.match(input); ok {
			return cmd.exec(m, args)
		}
	}
	m.err = fmt.Errorf("unknown command: %s", input)
	return m, nil
}
