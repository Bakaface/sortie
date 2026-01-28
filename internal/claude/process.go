package claude

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/aface/ralph-tamer-kit/internal/config"
)

// Process represents a Claude Code CLI process running in automatic mode.
// Uses -p flag for one-shot execution (Ralphy-style approach).
//
// Future: Manual mode will use tmux sessions for interactive workflows
// where humans can attach, test, and continue conversations.
type Process struct {
	TaskID     string
	WorkDir    string
	OutputFile string
	cfg        *config.ClaudeConfig

	mu         sync.RWMutex
	cmd        *exec.Cmd
	outputLines []string
	exitCode   int
	exited     bool
	exitErr    error
}

func NewProcess(taskID, workDir string, cfg *config.ClaudeConfig) *Process {
	outputFile := filepath.Join(workDir, ".claude-output.log")
	return &Process{
		TaskID:     taskID,
		WorkDir:    workDir,
		OutputFile: outputFile,
		cfg:        cfg,
		exitCode:   -1,
	}
}

// StartWithPrompt runs Claude Code with -p flag (one-shot mode).
// The process exits automatically when the task is complete.
func (p *Process) StartWithPrompt(prompt string) error {
	args := append([]string{}, p.cfg.DefaultArgs...)
	args = append(args, "-p", prompt)

	p.cmd = exec.Command(p.cfg.Command, args...)
	p.cmd.Dir = p.WorkDir

	// Create output file
	outFile, err := os.Create(p.OutputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}

	p.cmd.Stdout = outFile
	p.cmd.Stderr = outFile

	if err := p.cmd.Start(); err != nil {
		outFile.Close()
		return fmt.Errorf("failed to start claude process: %w", err)
	}

	// Monitor process in background
	go p.waitForExit(outFile)

	return nil
}

func (p *Process) waitForExit(outFile *os.File) {
	err := p.cmd.Wait()
	outFile.Close()

	p.mu.Lock()
	defer p.mu.Unlock()

	p.exited = true
	p.exitErr = err

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			p.exitCode = exitErr.ExitCode()
		} else {
			p.exitCode = 1
		}
	} else {
		p.exitCode = 0
	}
}

func (p *Process) Stop() error {
	p.mu.RLock()
	if p.exited || p.cmd == nil || p.cmd.Process == nil {
		p.mu.RUnlock()
		return nil
	}
	p.mu.RUnlock()

	// Send SIGTERM for graceful shutdown
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		// If SIGTERM fails, try SIGKILL
		return p.cmd.Process.Kill()
	}
	return nil
}

func (p *Process) IsRunning() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return !p.exited && p.cmd != nil && p.cmd.Process != nil
}

// ExitCode returns the exit code, or -1 if still running
func (p *Process) ExitCode() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.exitCode
}

// HasExited returns true if the process has terminated
func (p *Process) HasExited() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.exited
}

// IsSuccess returns true if process exited with code 0
func (p *Process) IsSuccess() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.exited && p.exitCode == 0
}

// CaptureOutput reads the output file and returns lines
func (p *Process) CaptureOutput(maxLines int) ([]string, error) {
	file, err := os.Open(p.OutputFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if maxLines > 0 && len(lines) >= maxLines {
			break
		}
	}

	return lines, scanner.Err()
}

// PID returns the process ID, or 0 if not running
func (p *Process) PID() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}
