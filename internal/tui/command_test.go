package tui

import (
	"testing"
)

func TestMatchQuit(t *testing.T) {
	validCmds := []string{
		"q", "q!",
		"qa", "qa!",
		"qall", "qall!",
		"wq", "wq!",
		"wqa", "wqa!",
		"x", "x!",
		"xa", "xa!",
		"xall", "xall!",
	}
	for _, cmd := range validCmds {
		t.Run(cmd, func(t *testing.T) {
			args, ok := matchQuit(cmd)
			if !ok {
				t.Errorf("matchQuit(%q) should match", cmd)
			}
			if args != cmd {
				t.Errorf("matchQuit(%q) args = %q, want %q", cmd, args, cmd)
			}
		})
	}

	// With leading/trailing whitespace
	args, ok := matchQuit("  q!  ")
	if !ok {
		t.Error("matchQuit with whitespace should match")
	}
	if args != "q!" {
		t.Errorf("matchQuit with whitespace args = %q, want %q", args, "q!")
	}
}

func TestMatchQuit_Invalid(t *testing.T) {
	invalidCmds := []string{
		"", "quit", "exit", "Q", "close", "ww", "qq",
	}
	for _, cmd := range invalidCmds {
		t.Run(cmd, func(t *testing.T) {
			_, ok := matchQuit(cmd)
			if ok {
				t.Errorf("matchQuit(%q) should not match", cmd)
			}
		})
	}
}

func TestExecQuit(t *testing.T) {
	m := Model{
		keys: newKeyMap(),
		list: newListView(false, ""),
	}

	result, cmd := execQuit(m, "q")
	updated := result.(Model)

	if !updated.quitting {
		t.Error("expected quitting to be true after execQuit")
	}
	if cmd == nil {
		t.Error("expected tea.Quit command, got nil")
	}
}

func TestExecuteCommand_QuitCommands(t *testing.T) {
	quitCmds := []string{"q", "q!", "qa", "wq", "x", "xa!"}
	for _, cmd := range quitCmds {
		t.Run(cmd, func(t *testing.T) {
			m := Model{
				keys: newKeyMap(),
				list: newListView(false, ""),
			}
			result, teaCmd := executeCommand(m, cmd)
			updated := result.(Model)

			if !updated.quitting {
				t.Errorf("executeCommand(%q) should set quitting to true", cmd)
			}
			if teaCmd == nil {
				t.Errorf("executeCommand(%q) should return tea.Quit", cmd)
			}
		})
	}
}
