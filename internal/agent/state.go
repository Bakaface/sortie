package agent

type State string

const (
	StatePending         State = "pending"
	StateStarting        State = "starting"
	StateRunning         State = "running"
	StateWaitingForInput State = "waiting_for_input"
	StateCompleted       State = "completed"
	StateFailed          State = "failed"
	StateStopped         State = "stopped"
)

func (s State) IsTerminal() bool {
	switch s {
	case StateCompleted, StateFailed, StateStopped:
		return true
	default:
		return false
	}
}

func (s State) IsActive() bool {
	switch s {
	case StateStarting, StateRunning, StateWaitingForInput:
		return true
	default:
		return false
	}
}

func (s State) String() string {
	return string(s)
}
