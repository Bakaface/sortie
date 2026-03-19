package tui

import (
	"github.com/charmbracelet/bubbles/key"
)

type keyMap struct {
	Up              key.Binding
	Down            key.Binding
	Enter           key.Binding
	Logs            key.Binding
	Stop            key.Binding
	Retry           key.Binding
	RunTask         key.Binding
	InitWorkflow    key.Binding
	Delete          key.Binding
	NewTask         key.Binding
	NewBlockingTask key.Binding
	Continue        key.Binding
	ChangePriority  key.Binding
	Attach         key.Binding
	OpenArtifact   key.Binding
	EditArtifact   key.Binding
	EditDesc       key.Binding
	EditTitle      key.Binding
	EditContext    key.Binding
	Revert         key.Binding
	BranchTask     key.Binding
	DetachBranch   key.Binding
	AttachBranch   key.Binding
	Refresh        key.Binding
	Back           key.Binding
	Quit           key.Binding
	Help           key.Binding
	PageUp         key.Binding
	PageDown       key.Binding
	GotoTop        key.Binding
	GotoBottom     key.Binding
	GotoTask       key.Binding
	SearchForward  key.Binding
	SearchBackward key.Binding
	NextMatch      key.Binding
	PrevMatch      key.Binding
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
		Retry: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "retry"),
		),
		RunTask: key.NewBinding(
			key.WithKeys("x"),
			key.WithHelp("x", "run task"),
		),
		InitWorkflow: key.NewBinding(
			key.WithKeys("i"),
			key.WithHelp("i", "init"),
		),
		Delete: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("dd", "delete"),
		),
		NewTask: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "new task"),
		),
		NewBlockingTask: key.NewBinding(
			key.WithKeys("N"),
			key.WithHelp("N", "new blocking task"),
		),
		Continue: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "continue"),
		),
		ChangePriority: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "set priority"),
		),
		Attach: key.NewBinding(
			key.WithKeys("t"),
			key.WithHelp("t", "tmux attach"),
		),
		OpenArtifact: key.NewBinding(
			key.WithKeys("o"),
			key.WithHelp("oa", "open artifact"),
		),
		EditArtifact: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("ea", "edit artifact"),
		),
		EditDesc: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("ed", "edit desc"),
		),
		EditTitle: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("et", "edit title"),
		),
		EditContext: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("ec", "edit context"),
		),
		Revert: key.NewBinding(
			key.WithKeys("R"),
			key.WithHelp("R", "revert"),
		),
		BranchTask: key.NewBinding(
			key.WithKeys("b"),
			key.WithHelp("b", "branch task"),
		),
		DetachBranch: key.NewBinding(
			key.WithKeys("D"),
			key.WithHelp("D", "detach branch"),
		),
		AttachBranch: key.NewBinding(
			key.WithKeys("A"),
			key.WithHelp("A", "attach branch"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("ctrl+r"),
			key.WithHelp("ctrl+r", "refresh"),
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
			key.WithKeys("ctrl+h"),
			key.WithHelp("ctrl+h", "help"),
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
		GotoTask: key.NewBinding(
			key.WithKeys(":"),
			key.WithHelp(":n", "go to line"),
		),
		SearchForward: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search forward"),
		),
		SearchBackward: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "search backward"),
		),
		NextMatch: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "next match"),
		),
		PrevMatch: key.NewBinding(
			key.WithKeys("N"),
			key.WithHelp("N", "prev match"),
		),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Enter, k.Logs, k.NewTask, k.InitWorkflow, k.Continue, k.Stop, k.Quit, k.Help}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown, k.GotoTop, k.GotoBottom, k.GotoTask, k.SearchForward, k.SearchBackward, k.NextMatch, k.PrevMatch, k.Enter, k.Logs},
		{k.NewTask, k.NewBlockingTask, k.BranchTask, k.RunTask, k.InitWorkflow, k.Stop, k.Retry, k.Revert, k.Continue, k.ChangePriority, k.Delete, k.Attach, k.DetachBranch, k.AttachBranch, k.OpenArtifact, k.EditArtifact, k.EditDesc, k.EditTitle, k.EditContext, k.Refresh},
		{k.Back, k.Quit, k.Help},
	}
}

type detailFollowKeyMap struct {
	ExitFollow key.Binding
	Back       key.Binding
	Stop       key.Binding
	Attach     key.Binding
	EditLog    key.Binding
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
		EditLog: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "open log"),
		),
	}
}

func (k detailFollowKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.ExitFollow, k.Back, k.Stop, k.Attach, k.EditLog}
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
	EditLog    key.Binding
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
			key.WithKeys("ctrl+u", "pgup"),
			key.WithHelp("ctrl+u", "half up"),
		),
		HalfDown: key.NewBinding(
			key.WithKeys("ctrl+d", "pgdown"),
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
		EditLog: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "open log"),
		),
	}
}

func (k detailNormalKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.GotoTop, k.GotoBottom, k.Down, k.Up, k.HalfDown, k.HalfUp, k.Follow, k.Attach, k.EditLog, k.Back}
}

type taskInfoKeyMap struct {
	Up           key.Binding
	Down         key.Binding
	HalfUp       key.Binding
	HalfDown     key.Binding
	GotoTop      key.Binding
	GotoBtm      key.Binding
	Logs         key.Binding
	Back         key.Binding
	Stop         key.Binding
	Attach       key.Binding
	OpenArtifact key.Binding
	EditArtifact key.Binding
	EditDesc     key.Binding
	EditTitle    key.Binding
	EditContext  key.Binding
	YankDesc     key.Binding
	YankContext  key.Binding
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
			key.WithKeys("ctrl+u", "pgup"),
			key.WithHelp("ctrl+u", "half up"),
		),
		HalfDown: key.NewBinding(
			key.WithKeys("ctrl+d", "pgdown"),
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
		OpenArtifact: key.NewBinding(
			key.WithKeys("o"),
			key.WithHelp("oa", "open artifact"),
		),
		EditArtifact: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("ea", "edit artifact"),
		),
		EditDesc: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("ed", "edit desc"),
		),
		EditTitle: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("et", "edit title"),
		),
		EditContext: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("ec", "edit context"),
		),
		YankDesc: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("yd", "copy desc"),
		),
		YankContext: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("yc", "copy context"),
		),
	}
}

func (k taskInfoKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.GotoTop, k.GotoBtm, k.Logs, k.Attach, k.OpenArtifact, k.EditArtifact, k.EditDesc, k.EditTitle, k.EditContext, k.YankDesc, k.YankContext, k.Back}
}

type promptKeyMap struct {
	Submit      key.Binding
	SwitchField key.Binding
	Newline     key.Binding
	Cancel      key.Binding
	RemoveImage key.Binding
	Worktree    key.Binding
	BranchMode  key.Binding
	Editor      key.Binding
	Help        key.Binding
}

func newPromptKeyMap() promptKeyMap {
	return promptKeyMap{
		Submit: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "submit"),
		),
		SwitchField: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "switch field"),
		),
		Newline: key.NewBinding(
			key.WithKeys("ctrl+j"),
			key.WithHelp("ctrl+j", "newline"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
		RemoveImage: key.NewBinding(
			key.WithKeys("ctrl+x"),
			key.WithHelp("ctrl+x", "remove last image"),
		),
		Worktree: key.NewBinding(
			key.WithKeys("alt+w"),
			key.WithHelp("alt+w", "worktree"),
		),
		BranchMode: key.NewBinding(
			key.WithKeys("alt+m"),
			key.WithHelp("alt+m", "branch mode"),
		),
		Editor: key.NewBinding(
			key.WithKeys("ctrl+g"),
			key.WithHelp("ctrl+g", "editor"),
		),
		Help: key.NewBinding(
			key.WithKeys("ctrl+h"),
			key.WithHelp("ctrl+h", "help"),
		),
	}
}

func (k promptKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Submit, k.Cancel, k.Newline, k.Help}
}

func (k promptKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Submit, k.Cancel, k.SwitchField, k.Newline},
		{k.Worktree, k.BranchMode, k.Editor, k.RemoveImage},
		{k.Help},
	}
}

// Pre-allocated key maps to avoid allocations on every renderHelp() call.
var (
	cachedDetailFollowKeyMap = newDetailFollowKeyMap()
	cachedDetailNormalKeyMap = newDetailNormalKeyMap()
	cachedPromptKeyMap       = newPromptKeyMap()
)
