package agent

import "sync"

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
