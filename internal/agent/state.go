package agent

type State string

// State is a concurrency-slot marker for the Manager's scheduling machinery
// (maxConcurrent gating, pending queue) — it answers "is this agent's slot
// occupied, and did it end cleanly?" It carries no domain meaning about what
// the task is actually doing; that's task.Status's job (the daemon always
// re-fetches the task row on StateCompleted/StateFailed to find out — see
// onAgentStateChange). StateWaitingForInput was removed: nothing in the
// Manager ever transitioned an Agent into it (the analogous "paused,
// awaiting human input" domain state is carried entirely by
// task.StatusAwaitingApproval / task.StatusTmux).
const (
	StatePending   State = "pending"
	StateStarting  State = "starting"
	StateRunning   State = "running"
	StateCompleted State = "completed"
	StateFailed    State = "failed"
	StateStopped   State = "stopped"
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
	case StateStarting, StateRunning:
		return true
	default:
		return false
	}
}

func (s State) String() string {
	return string(s)
}
