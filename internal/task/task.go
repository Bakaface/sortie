package task

import (
	"regexp"
	"strings"
	"time"
)

type Status string

const (
	StatusPending          Status = "pending"
	StatusInit             Status = "init"
	StatusRunning          Status = "running"
	StatusAwaitingApproval Status = "awaiting-approval"
	StatusTmux             Status = "tmux"
	StatusSummarizing      Status = "summarizing"
	StatusCompleted        Status = "completed"
	StatusFailed           Status = "failed"
	StatusArtifactMissing  Status = "artifact-missing"
)

func (s Status) String() string {
	return string(s)
}

func (s Status) IsTerminal() bool {
	return s == StatusCompleted || s == StatusFailed
}

func (s Status) IsActive() bool {
	return s == StatusRunning || s == StatusAwaitingApproval || s == StatusTmux || s == StatusSummarizing || s == StatusArtifactMissing
}

type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityMedium Priority = "medium"
	PriorityHigh   Priority = "high"
	PriorityUrgent Priority = "urgent"
)

func (p Priority) String() string {
	return string(p)
}

// Value returns a numeric sort value for the priority (higher = more important).
func (p Priority) Value() int {
	switch p {
	case PriorityUrgent:
		return 4
	case PriorityHigh:
		return 3
	case PriorityMedium:
		return 2
	case PriorityLow:
		return 1
	default:
		return 2
	}
}

// ValidPriorities returns all valid priority levels in ascending order.
func ValidPriorities() []Priority {
	return []Priority{PriorityLow, PriorityMedium, PriorityHigh, PriorityUrgent}
}

// IsValidPriority checks if a string is a valid priority value.
func IsValidPriority(s string) bool {
	switch Priority(s) {
	case PriorityLow, PriorityMedium, PriorityHigh, PriorityUrgent:
		return true
	}
	return false
}

type Task struct {
	ID           int64
	ProjectID    int64
	Title        string
	Description  string
	Slug         string
	Workflow     string
	Status       Status
	Priority     Priority
	StepIndex    int
	CurrentStep  string
	Branch       string
	WorktreePath string
	ExitCode     *int
	ErrorMessage string
	Context      string
	BlockedBy    []int64
	Images       []string
	CreatedAt    time.Time
	StartedAt    *time.Time
	CompletedAt  *time.Time
	UpdatedAt    time.Time
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func Slugify(title string) string {
	s := strings.ToLower(title)
	s = slugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 40 {
		s = s[:40]
		s = strings.TrimRight(s, "-")
	}
	return s
}
