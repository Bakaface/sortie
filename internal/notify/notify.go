package notify

import (
	"os/exec"

	"github.com/Bakaface/sortie/internal/config"
)

type Notifier struct {
	cfg *config.NotificationsConfig
}

func New(cfg *config.NotificationsConfig) *Notifier {
	return &Notifier{cfg: cfg}
}

type Urgency string

const (
	UrgencyLow      Urgency = "low"
	UrgencyNormal   Urgency = "normal"
	UrgencyCritical Urgency = "critical"
)

func (n *Notifier) Send(title, body string, urgency Urgency) error {
	if !n.cfg.Enabled {
		return nil
	}

	args := []string{
		"--app-name=Sortie",
		"--urgency=" + string(urgency),
	}

	args = append(args, title, body)

	return exec.Command("notify-send", args...).Run()
}

func (n *Notifier) AgentCompleted(taskID, description string) error {
	if !n.cfg.OnComplete {
		return nil
	}

	return n.Send(
		"Agent Completed",
		taskID+": "+description,
		UrgencyNormal,
	)
}

func (n *Notifier) AgentFailed(taskID, description, errMsg string) error {
	if !n.cfg.OnFailed {
		return nil
	}

	body := taskID + ": " + description
	if errMsg != "" {
		body += "\nError: " + errMsg
	}

	return n.Send(
		"Agent Failed",
		body,
		UrgencyCritical,
	)
}

func (n *Notifier) AllTasksCompleted() error {
	if !n.cfg.OnComplete {
		return nil
	}

	return n.Send(
		"All Tasks Completed",
		"All tasks have finished processing.",
		UrgencyNormal,
	)
}

func (n *Notifier) AgentWaitingForInput(taskID, description string) error {
	if !n.cfg.OnWaitingInput {
		return nil
	}

	return n.Send(
		"Agent Needs Input",
		taskID+": "+description,
		UrgencyCritical,
	)
}
