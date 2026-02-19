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
)

func (s Status) String() string {
	return string(s)
}

func (s Status) IsTerminal() bool {
	return s == StatusCompleted || s == StatusFailed
}

func (s Status) IsActive() bool {
	return s == StatusRunning || s == StatusAwaitingApproval || s == StatusTmux || s == StatusSummarizing
}

type Task struct {
	ID           int64
	ProjectID    int64
	Title        string
	Description  string
	Slug         string
	Workflow     string
	Status       Status
	StepIndex    int
	CurrentStep  string
	Branch       string
	WorktreePath string
	ExitCode     *int
	ErrorMessage string
	Context      string
	BlockedBy    []int64
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
