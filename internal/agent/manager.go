package agent

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/aface/ralph-tamer-kit/internal/task"
)

var (
	ErrTaskAlreadyTracked = errors.New("task already tracked")
	ErrAgentNotFound      = errors.New("agent not found")
	ErrNoWorkDir          = errors.New("task has no workdir")
)

type StateChangeCallback func(agent *Agent, oldState, newState State)

type Manager struct {
	mu             sync.RWMutex
	agents         map[string]*Agent
	knownTasks     map[int64]bool
	pendingQueue   []string
	pendingRunners map[string]func(ctx context.Context) error
	cancelFuncs    map[string]context.CancelFunc
	maxConcurrent  int
	bufferSize     int

	onStateChange StateChangeCallback
}

func NewManager(maxConcurrent, bufferSize int) *Manager {
	return &Manager{
		agents:         make(map[string]*Agent),
		knownTasks:     make(map[int64]bool),
		pendingQueue:   make([]string, 0),
		pendingRunners: make(map[string]func(ctx context.Context) error),
		cancelFuncs:    make(map[string]context.CancelFunc),
		maxConcurrent:  maxConcurrent,
		bufferSize:     bufferSize,
	}
}

func (m *Manager) SetStateChangeCallback(cb StateChangeCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onStateChange = cb
}

func (m *Manager) StartAgent(t *task.Task, workDir string, runner func(ctx context.Context) error) (*Agent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.knownTasks[t.ID] {
		return nil, ErrTaskAlreadyTracked
	}

	if workDir == "" {
		return nil, ErrNoWorkDir
	}

	agent := New(t, m.bufferSize)
	agent.WorkDir = workDir
	m.agents[agent.ID] = agent
	m.knownTasks[t.ID] = true

	if m.canStartMore() {
		return agent, m.startAgentLocked(agent, runner)
	}

	m.pendingRunners[agent.ID] = runner
	m.pendingQueue = append(m.pendingQueue, agent.ID)
	return agent, nil
}

func (m *Manager) canStartMore() bool {
	if m.maxConcurrent <= 0 {
		return true
	}

	activeCount := 0
	for _, agent := range m.agents {
		if agent.GetState().IsActive() {
			activeCount++
		}
	}

	return activeCount < m.maxConcurrent
}

func (m *Manager) startAgentLocked(agent *Agent, runner func(ctx context.Context) error) error {
	oldState := agent.GetState()
	agent.SetState(StateStarting)
	agent.StartedAt = time.Now()

	if m.onStateChange != nil {
		m.onStateChange(agent, oldState, StateStarting)
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancelFuncs[agent.ID] = cancel

	agent.SetState(StateRunning)
	if m.onStateChange != nil {
		m.onStateChange(agent, StateStarting, StateRunning)
	}

	go func() {
		err := runner(ctx)

		m.mu.Lock()
		defer m.mu.Unlock()

		oldState := agent.GetState()
		if err != nil {
			agent.SetError(err.Error())
		} else {
			agent.SetState(StateCompleted)
		}
		delete(m.cancelFuncs, agent.ID)

		if m.onStateChange != nil {
			m.onStateChange(agent, oldState, agent.GetState())
		}

		m.processQueueLocked()
	}()

	return nil
}

func (m *Manager) StopAgent(agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	agent, exists := m.agents[agentID]
	if !exists {
		return ErrAgentNotFound
	}

	oldState := agent.GetState()
	if oldState.IsTerminal() {
		return nil
	}

	if cancel, exists := m.cancelFuncs[agentID]; exists {
		cancel()
		delete(m.cancelFuncs, agentID)
	}

	agent.SetState(StateStopped)

	if m.onStateChange != nil {
		m.onStateChange(agent, oldState, StateStopped)
	}

	m.processQueueLocked()

	return nil
}

func (m *Manager) processQueueLocked() {
	if len(m.pendingQueue) == 0 || !m.canStartMore() {
		return
	}

	agentID := m.pendingQueue[0]
	m.pendingQueue = m.pendingQueue[1:]

	if agent, exists := m.agents[agentID]; exists {
		if agent.GetState() == StatePending {
			if runner, ok := m.pendingRunners[agentID]; ok {
				delete(m.pendingRunners, agentID)
				m.startAgentLocked(agent, runner)
			}
		}
	}
}

func (m *Manager) GetAgent(agentID string) (*Agent, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	agent, exists := m.agents[agentID]
	return agent, exists
}

func (m *Manager) GetAgentByTaskID(taskID int64) (*Agent, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, agent := range m.agents {
		if agent.Task.ID == taskID {
			return agent, true
		}
	}
	return nil, false
}

func (m *Manager) ListAgents() []*Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agents := make([]*Agent, 0, len(m.agents))
	for _, agent := range m.agents {
		agents = append(agents, agent)
	}
	return agents
}

func (m *Manager) IsTaskKnown(taskID int64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.knownTasks[taskID]
}

// RecoverSessions is reserved for future manual mode with tmux sessions.
// In automatic mode, processes are one-shot and don't persist across restarts.
func (m *Manager) RecoverSessions() error {
	// No-op in automatic mode
	// Future: Recover tmux sessions for manual mode
	return nil
}

func (m *Manager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, cancel := range m.cancelFuncs {
		cancel()
		delete(m.cancelFuncs, id)
	}
}

func (m *Manager) GetOutput(agentID string, fromLine int) ([]string, int, error) {
	m.mu.RLock()
	agent, exists := m.agents[agentID]
	m.mu.RUnlock()

	if !exists {
		return nil, 0, ErrAgentNotFound
	}

	lines, total := agent.GetOutput(fromLine)
	return lines, total, nil
}

// SendInput is reserved for future manual mode with tmux sessions.
// In automatic mode, processes run with -p flag and cannot accept interactive input.
func (m *Manager) SendInput(agentID, input string) error {
	return errors.New("SendInput not available in automatic mode")
}
