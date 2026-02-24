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
	StatusFinalizing       Status = "finalizing"
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
	return s == StatusRunning || s == StatusAwaitingApproval || s == StatusTmux || s == StatusFinalizing || s == StatusSummarizing || s == StatusArtifactMissing
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
	StepIndex     int
	CurrentStep   string
	LoopIteration int
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

// MaxTitleLength is the maximum allowed length for a task title.
const MaxTitleLength = 80

// SanitizeTitle cleans a title string by taking only the first line,
// collapsing whitespace, removing control characters, and truncating
// to MaxTitleLength.
func SanitizeTitle(title string) string {
	// Take only the first line (split on any line break)
	if idx := strings.IndexAny(title, "\n\r"); idx != -1 {
		title = title[:idx]
	}

	// Replace control characters: tabs become spaces, others are removed
	var b strings.Builder
	for _, r := range title {
		if r == '\t' {
			b.WriteByte(' ')
		} else if r >= ' ' {
			b.WriteRune(r)
		}
	}
	title = b.String()

	// Collapse multiple spaces and trim
	title = strings.Join(strings.Fields(title), " ")

	// Truncate to max length
	if len(title) > MaxTitleLength {
		title = title[:MaxTitleLength]
		// Avoid cutting in the middle of a word — trim back to last space
		if idx := strings.LastIndexByte(title, ' '); idx > MaxTitleLength/2 {
			title = title[:idx]
		}
	}

	return title
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
