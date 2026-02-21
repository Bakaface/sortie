package tui

import (
	"github.com/charmbracelet/bubbles/key"
)

type keyMap struct {
	Up         key.Binding
	Down       key.Binding
	Enter      key.Binding
	Logs       key.Binding
	Stop       key.Binding
	Approve    key.Binding
	Reject     key.Binding
	Retry      key.Binding
	Delete     key.Binding
	NewTask    key.Binding
	Continue   key.Binding
	Attach     key.Binding
	Refresh    key.Binding
	Back       key.Binding
	Quit       key.Binding
	Help       key.Binding
	PageUp     key.Binding
	PageDown   key.Binding
	GotoTop    key.Binding
	GotoBottom key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "task info"),
		),
		Logs: key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", "logs"),
		),
		Stop: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "stop"),
		),
		Approve: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "approve"),
		),
		Reject: key.NewBinding(
			key.WithKeys("x"),
			key.WithHelp("x", "reject"),
		),
		Retry: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "retry"),
		),
		Delete: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("dd", "delete"),
		),
		NewTask: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "new task"),
		),
		Continue: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "continue"),
		),
		Attach: key.NewBinding(
			key.WithKeys("t"),
			key.WithHelp("t", "tmux attach"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("R"),
			key.WithHelp("R", "refresh"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "toggle help"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup", "ctrl+u"),
			key.WithHelp("ctrl+u", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown", "ctrl+d"),
			key.WithHelp("ctrl+d", "page down"),
		),
		GotoTop: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("gg", "top"),
		),
		GotoBottom: key.NewBinding(
			key.WithKeys("G"),
			key.WithHelp("G", "bottom"),
		),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Enter, k.Logs, k.NewTask, k.Approve, k.Reject, k.Retry, k.Continue, k.Delete, k.Stop, k.Attach, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown, k.GotoTop, k.GotoBottom, k.Enter, k.Logs},
		{k.NewTask, k.Stop, k.Approve, k.Reject, k.Retry, k.Continue, k.Delete, k.Attach, k.Refresh},
		{k.Back, k.Quit, k.Help},
	}
}

type detailKeyMap struct {
	Up       key.Binding
	Down     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Stop     key.Binding
	Back     key.Binding
}

func newDetailKeyMap() detailKeyMap {
	return detailKeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "scroll up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "scroll down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup", "ctrl+u"),
			key.WithHelp("pgup", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown", "ctrl+d"),
			key.WithHelp("pgdown", "page down"),
		),
		Stop: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "stop agent"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc", "q"),
			key.WithHelp("esc/q", "back to list"),
		),
	}
}

func (k detailKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Back}
}

func (k detailKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown},
		{k.Stop},
		{k.Back},
	}
}

type detailFollowKeyMap struct {
	ExitFollow key.Binding
	Back       key.Binding
	Stop       key.Binding
	Attach     key.Binding
}

func newDetailFollowKeyMap() detailFollowKeyMap {
	return detailFollowKeyMap{
		ExitFollow: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "normal mode"),
		),
		Back: key.NewBinding(
			key.WithKeys("q"),
			key.WithHelp("q", "back to list"),
		),
		Stop: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "stop"),
		),
		Attach: key.NewBinding(
			key.WithKeys("t"),
			key.WithHelp("t", "tmux attach"),
		),
	}
}

func (k detailFollowKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.ExitFollow, k.Back, k.Stop, k.Attach}
}

type detailNormalKeyMap struct {
	GotoTop    key.Binding
	GotoBottom key.Binding
	Up         key.Binding
	Down       key.Binding
	HalfUp     key.Binding
	HalfDown   key.Binding
	Follow     key.Binding
	Back       key.Binding
	Stop       key.Binding
	Attach     key.Binding
}

func newDetailNormalKeyMap() detailNormalKeyMap {
	return detailNormalKeyMap{
		GotoTop: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("gg", "top"),
		),
		GotoBottom: key.NewBinding(
			key.WithKeys("G"),
			key.WithHelp("G", "bottom"),
		),
		Up: key.NewBinding(
			key.WithKeys("k"),
			key.WithHelp("k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("j"),
			key.WithHelp("j", "down"),
		),
		HalfUp: key.NewBinding(
			key.WithKeys("ctrl+u"),
			key.WithHelp("ctrl+u", "half up"),
		),
		HalfDown: key.NewBinding(
			key.WithKeys("ctrl+d"),
			key.WithHelp("ctrl+d", "half down"),
		),
		Follow: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "follow"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc", "q"),
			key.WithHelp("esc/q", "back"),
		),
		Stop: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "stop"),
		),
		Attach: key.NewBinding(
			key.WithKeys("t"),
			key.WithHelp("t", "tmux attach"),
		),
	}
}

func (k detailNormalKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.GotoTop, k.GotoBottom, k.Down, k.Up, k.HalfDown, k.HalfUp, k.Follow, k.Attach, k.Back}
}

type taskInfoKeyMap struct {
	Up       key.Binding
	Down     key.Binding
	HalfUp   key.Binding
	HalfDown key.Binding
	GotoTop  key.Binding
	GotoBtm  key.Binding
	Logs     key.Binding
	Back     key.Binding
	Stop     key.Binding
	Attach   key.Binding
}

func newTaskInfoKeyMap() taskInfoKeyMap {
	return taskInfoKeyMap{
		Up: key.NewBinding(
			key.WithKeys("k", "up"),
			key.WithHelp("k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("j", "down"),
			key.WithHelp("j", "down"),
		),
		HalfUp: key.NewBinding(
			key.WithKeys("ctrl+u"),
			key.WithHelp("ctrl+u", "half up"),
		),
		HalfDown: key.NewBinding(
			key.WithKeys("ctrl+d"),
			key.WithHelp("ctrl+d", "half down"),
		),
		GotoTop: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("gg", "top"),
		),
		GotoBtm: key.NewBinding(
			key.WithKeys("G"),
			key.WithHelp("G", "bottom"),
		),
		Logs: key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", "logs"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc", "q"),
			key.WithHelp("esc/q", "back"),
		),
		Stop: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "stop"),
		),
		Attach: key.NewBinding(
			key.WithKeys("t"),
			key.WithHelp("t", "tmux attach"),
		),
	}
}

func (k taskInfoKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.GotoTop, k.GotoBtm, k.Logs, k.Attach, k.Back}
}

// Pre-allocated key maps to avoid allocations on every renderHelp() call.
var (
	cachedDetailFollowKeyMap = newDetailFollowKeyMap()
	cachedDetailNormalKeyMap = newDetailNormalKeyMap()
)
