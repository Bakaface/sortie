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
		match: matchQuit,
		exec:  execQuit,
		help:  "quit",
	},
	{
		match: matchLineNumber,
		exec:  execGotoLine,
		help:  "go to line number",
	},
	{
		match: matchRunTask,
		exec:  execRunTask,
		help:  "run a predefined task",
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
		match: matchSetBranch,
		exec:  execSetBranch,
		help:  "toggle branch display",
	},
	{
		match: matchSetTarget,
		exec:  execSetTarget,
		help:  "toggle target branch display",
	},
	{
		match: matchNoh,
		exec:  execNoh,
		help:  "clear search highlights",
	},
}

// quitCommands lists all vim-style quit commands that close the TUI.
var quitCommands = map[string]bool{
	"q": true, "q!": true,
	"qa": true, "qa!": true,
	"qall": true, "qall!": true,
	"wq": true, "wq!": true,
	"wqa": true, "wqa!": true,
	"x": true, "x!": true,
	"xa": true, "xa!": true,
	"xall": true, "xall!": true,
}

// matchQuit matches vim-style quit commands (q, q!, qa, wq, x, etc.).
func matchQuit(input string) (string, bool) {
	input = strings.TrimSpace(input)
	if quitCommands[input] {
		return input, true
	}
	return "", false
}

// execQuit closes the TUI, mirroring the behavior of the "q" keybinding.
func execQuit(m Model, _ string) (tea.Model, tea.Cmd) {
	m.quitting = true
	if m.client != nil {
		m.client.Close()
	}
	return m, tea.Quit
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

// matchSetBranch matches "set branch", "set nobranch", and "set branch!" commands.
func matchSetBranch(input string) (string, bool) {
	input = strings.TrimSpace(input)
	switch input {
	case "set branch", "set nobranch", "set branch!":
		return input, true
	}
	return "", false
}

// execSetBranch enables, disables, or toggles branch display in the list view.
func execSetBranch(m Model, args string) (tea.Model, tea.Cmd) {
	switch args {
	case "set branch":
		m.list.showBranch = true
	case "set nobranch":
		m.list.showBranch = false
	case "set branch!":
		m.list.showBranch = !m.list.showBranch
	}
	return m, nil
}

// matchSetTarget matches "set target", "set notarget", and "set target!" commands.
func matchSetTarget(input string) (string, bool) {
	input = strings.TrimSpace(input)
	switch input {
	case "set target", "set notarget", "set target!":
		return input, true
	}
	return "", false
}

// execSetTarget enables, disables, or toggles target branch display in the list view.
func execSetTarget(m Model, args string) (tea.Model, tea.Cmd) {
	switch args {
	case "set target":
		m.list.showTarget = true
	case "set notarget":
		m.list.showTarget = false
	case "set target!":
		m.list.showTarget = !m.list.showTarget
	}
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

// matchRunTask matches "RunTask <name>" commands.
func matchRunTask(input string) (string, bool) {
	input = strings.TrimSpace(input)
	if input == "RunTask" {
		return "", true
	}
	if strings.HasPrefix(input, "RunTask ") {
		return strings.TrimSpace(input[len("RunTask "):]), true
	}
	return "", false
}

// execRunTask runs a predefined task by name.
func execRunTask(m Model, args string) (tea.Model, tea.Cmd) {
	if m.client == nil || m.projectPath == "" {
		m.err = fmt.Errorf("not connected to daemon")
		return m, nil
	}
	if m.cfg == nil {
		m.err = fmt.Errorf("no config loaded")
		return m, nil
	}
	if args == "" {
		m.err = fmt.Errorf("usage: RunTask <name>")
		return m, nil
	}
	taskCfg := m.cfg.GetPredefinedTask(args)
	if taskCfg == nil {
		m.err = fmt.Errorf("unknown task: %s", args)
		return m, nil
	}
	m.selectedWorkflow = "oneoff:" + taskCfg.Name
	description := taskCfg.Description
	if description == "" {
		description = taskCfg.Name
	}
	return m, m.createTaskWithPrompt("", description, "", true, nil, "", "")
}

// completeRunTask returns tab-completed command input for RunTask.
// It matches task names (including unlisted) against the partial input after "RunTask ".
func completeRunTask(m Model, input string) (string, bool) {
	if m.cfg == nil {
		return "", false
	}
	if !strings.HasPrefix(input, "RunTask") {
		return "", false
	}
	// Complete "RunTask" itself if user typed a prefix like "Run" or "RunT"
	if len(input) < len("RunTask") {
		return "", false
	}
	// If exactly "RunTask" with no space yet, add the space
	if input == "RunTask" {
		return "RunTask ", true
	}
	if !strings.HasPrefix(input, "RunTask ") {
		return "", false
	}
	partial := input[len("RunTask "):]
	partialLower := strings.ToLower(partial)
	allTasks := m.cfg.ListPredefinedTaskNames()
	var matches []string
	for _, name := range allTasks {
		if strings.HasPrefix(strings.ToLower(name), partialLower) {
			matches = append(matches, name)
		}
	}
	if len(matches) == 1 {
		return "RunTask " + matches[0], true
	}
	if len(matches) > 1 {
		// Find longest common prefix among matches
		prefix := matches[0]
		for _, m := range matches[1:] {
			prefix = commonPrefix(prefix, m)
		}
		if len(prefix) > len(partial) {
			return "RunTask " + prefix, true
		}
	}
	return "", false
}

// commonPrefix returns the longest common prefix of two strings (case-preserving).
func commonPrefix(a, b string) string {
	minLen := min(len(a), len(b))
	for i := 0; i < minLen; i++ {
		if !strings.EqualFold(string(a[i]), string(b[i])) {
			return a[:i]
		}
	}
	return a[:minLen]
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
