package agent

import (
	"fmt"
	"sync"
	"time"

	"github.com/aface/ralph-tamer-kit/internal/task"
)

type Agent struct {
	mu sync.RWMutex

	ID          string
	Task        *task.Task
	WorkDir     string
	State       State
	PID         int // Process ID of claude CLI
	StartedAt   time.Time
	EndedAt     time.Time
	Error       string
	CurrentStep string
	StepIndex   int

	outputBuffer *RingBuffer
}

func New(t *task.Task, bufferSize int) *Agent {
	return &Agent{
		ID:           fmt.Sprintf("%d", t.ID),
		Task:         t,
		WorkDir:      t.WorktreePath,
		State:        StatePending,
		outputBuffer: NewRingBuffer(bufferSize),
	}
}

func (a *Agent) GetState() State {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.State
}

func (a *Agent) SetState(state State) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.State = state
	if state.IsTerminal() && a.EndedAt.IsZero() {
		a.EndedAt = time.Now()
	}
}

func (a *Agent) SetError(err string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Error = err
	a.State = StateFailed
	if a.EndedAt.IsZero() {
		a.EndedAt = time.Now()
	}
}

func (a *Agent) SetPID(pid int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.PID = pid
}

func (a *Agent) GetPID() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.PID
}

func (a *Agent) SetWorkDir(workDir string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.WorkDir = workDir
}

func (a *Agent) AppendOutput(lines []string) {
	a.outputBuffer.Append(lines)
}

func (a *Agent) GetOutput(fromLine int) ([]string, int) {
	return a.outputBuffer.GetFrom(fromLine)
}

func (a *Agent) GetAllOutput() []string {
	return a.outputBuffer.GetAll()
}

func (a *Agent) Duration() time.Duration {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.StartedAt.IsZero() {
		return 0
	}

	if !a.EndedAt.IsZero() {
		return a.EndedAt.Sub(a.StartedAt)
	}

	return time.Since(a.StartedAt)
}

type RingBuffer struct {
	mu       sync.RWMutex
	lines    []string
	capacity int
	start    int
	count    int
	total    int
}

func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		lines:    make([]string, capacity),
		capacity: capacity,
	}
}

func (rb *RingBuffer) Append(lines []string) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	for _, line := range lines {
		idx := (rb.start + rb.count) % rb.capacity
		rb.lines[idx] = line
		if rb.count < rb.capacity {
			rb.count++
		} else {
			rb.start = (rb.start + 1) % rb.capacity
		}
		rb.total++
	}
}

func (rb *RingBuffer) GetFrom(fromLine int) ([]string, int) {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if fromLine >= rb.total {
		return nil, rb.total
	}

	oldestAvailable := rb.total - rb.count
	if fromLine < oldestAvailable {
		fromLine = oldestAvailable
	}

	startOffset := fromLine - oldestAvailable
	numLines := rb.count - startOffset

	result := make([]string, numLines)
	for i := 0; i < numLines; i++ {
		idx := (rb.start + startOffset + i) % rb.capacity
		result[i] = rb.lines[idx]
	}

	return result, rb.total
}

func (rb *RingBuffer) GetAll() []string {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	result := make([]string, rb.count)
	for i := 0; i < rb.count; i++ {
		idx := (rb.start + i) % rb.capacity
		result[i] = rb.lines[idx]
	}
	return result
}
