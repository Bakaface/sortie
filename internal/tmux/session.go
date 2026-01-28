package tmux

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

const SessionPrefix = "ralph-tamer-kit-"

type Session struct {
	Name    string
	WorkDir string
}

func NewSession(taskID, workDir string) *Session {
	return &Session{
		Name:    SessionPrefix + taskID,
		WorkDir: workDir,
	}
}

func (s *Session) Create(command string, args ...string) error {
	cmdArgs := []string{
		"new-session",
		"-d",
		"-s", s.Name,
		"-c", s.WorkDir,
	}

	if command != "" {
		// Append command and args as separate arguments
		// tmux will execute: command arg1 arg2 ...
		cmdArgs = append(cmdArgs, command)
		cmdArgs = append(cmdArgs, args...)
	}

	cmd := exec.Command("tmux", cmdArgs...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create tmux session: %w (stderr: %s)", err, stderr.String())
	}

	return nil
}

func (s *Session) Exists() bool {
	cmd := exec.Command("tmux", "has-session", "-t", s.Name)
	return cmd.Run() == nil
}

func (s *Session) Kill() error {
	cmd := exec.Command("tmux", "kill-session", "-t", s.Name)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to kill tmux session: %w (stderr: %s)", err, stderr.String())
	}

	return nil
}

func (s *Session) CapturePane(scrollbackLines int) ([]string, error) {
	args := []string{
		"capture-pane",
		"-t", s.Name,
		"-p",
		"-S", fmt.Sprintf("-%d", scrollbackLines),
	}

	cmd := exec.Command("tmux", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to capture pane: %w (stderr: %s)", err, stderr.String())
	}

	output := stdout.String()
	if output == "" {
		return nil, nil
	}

	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	return lines, nil
}

func (s *Session) SendKeys(keys string) error {
	// Use -l for literal text (handles special characters correctly)
	cmd := exec.Command("tmux", "send-keys", "-t", s.Name, "-l", keys)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to send keys: %w (stderr: %s)", err, stderr.String())
	}

	// Send Enter separately to submit
	enterCmd := exec.Command("tmux", "send-keys", "-t", s.Name, "Enter")
	enterCmd.Stderr = &stderr

	if err := enterCmd.Run(); err != nil {
		return fmt.Errorf("failed to send Enter: %w (stderr: %s)", err, stderr.String())
	}

	return nil
}

func (s *Session) IsAlive() bool {
	cmd := exec.Command("tmux", "list-panes", "-t", s.Name, "-F", "#{pane_dead}")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return false
	}

	return strings.TrimSpace(stdout.String()) == "0"
}

func ListSessions(prefix string) ([]*Session, error) {
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), "no server running") ||
			strings.Contains(stderr.String(), "no sessions") {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list sessions: %w (stderr: %s)", err, stderr.String())
	}

	var sessions []*Session
	for _, name := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if name == "" {
			continue
		}
		if prefix == "" || strings.HasPrefix(name, prefix) {
			sessions = append(sessions, &Session{Name: name})
		}
	}

	return sessions, nil
}

func ExtractTaskID(sessionName string) string {
	if strings.HasPrefix(sessionName, SessionPrefix) {
		return sessionName[len(SessionPrefix):]
	}
	return sessionName
}

func AttachCommand(sessionName string) *exec.Cmd {
	return exec.Command("tmux", "attach-session", "-t", sessionName)
}
