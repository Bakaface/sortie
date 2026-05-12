package daemon

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/aface/sortie/internal/agent"
	gitpkg "github.com/aface/sortie/internal/git"
	"github.com/aface/sortie/internal/task"
	"github.com/aface/sortie/internal/tmux"
)

func (s *Server) taskPollerLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.cfg.Daemon.PollInterval)
	defer ticker.Stop()

	s.checkPendingTasks()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.checkPendingTasks()
		}
	}
}

func (s *Server) checkPendingTasks() {
	tasks, err := s.database.GetClaimableTasks()
	if err != nil {
		log.Printf("Failed to get claimable tasks: %v", err)
		return
	}

	for _, t := range tasks {
		if s.manager.IsTaskKnown(t.ID) {
			continue
		}

		title := t.Title
		if title == "" {
			title = t.Description
		}
		log.Printf("%sStarting agent for task #%d: %s", s.projectLogPrefix(t.ProjectID), t.ID, title)
		if err := s.startTaskAgent(t); err != nil {
			log.Printf("%sFailed to start agent for task #%d: %v", s.projectLogPrefix(t.ProjectID), t.ID, err)
		}
	}
}

func (s *Server) startTaskAgent(t *task.Task) error {
	if t.Status == task.StatusPending {
		claimed, err := s.database.ClaimTask(t.ID)
		if err != nil {
			return fmt.Errorf("failed to claim task: %w", err)
		}
		if !claimed {
			return agent.ErrTaskAlreadyTracked
		}
	}

	pc, err := s.getProjectContext(t.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to get project context: %w", err)
	}

	workDir := t.WorktreePath
	if workDir == "" {
		workDir = pc.repoRoot
	}

	engine := pc.engine
	manager := s.manager
	runner := func(ctx context.Context) error {
		var outputFn func([]string)
		if a, ok := manager.GetAgentByTaskID(t.ID); ok {
			outputFn = a.AppendOutput
		}
		return engine.RunTask(ctx, t, outputFn)
	}

	if _, err := s.manager.StartAgent(t, workDir, runner); err != nil {
		return err
	}

	return nil
}

func (s *Server) recoverOrphanedTasks() error {
	runningTasks, err := s.database.GetRunningTasks()
	if err != nil {
		return err
	}

	// Clean repo state for any projects that had running tasks — a previous
	// daemon crash may have left staged changes from an interrupted merge.
	cleanedRepos := make(map[string]bool)
	for _, t := range runningTasks {
		if repoRoot := s.getProjectRepoRoot(t); repoRoot != "" && !cleanedRepos[repoRoot] {
			cleanedRepos[repoRoot] = true
			if dirty, err := gitpkg.HasChanges(repoRoot); err == nil && dirty {
				log.Printf("Cleaning up dirty repo state at %s from previous run", repoRoot)
				if err := gitpkg.CleanRepoState(repoRoot); err != nil {
					log.Printf("Warning: failed to clean repo state at %s: %v", repoRoot, err)
				}
			}
		}
	}

	for _, t := range runningTasks {
		log.Printf("%sRecovering orphaned task #%d, resetting to pending", s.projectLogPrefix(t.ProjectID), t.ID)
		if err := s.database.ResetTaskForRetry(t.ID); err != nil {
			log.Printf("%sFailed to reset task #%d: %v", s.projectLogPrefix(t.ProjectID), t.ID, err)
		}
	}

	allTasks, err := s.database.GetAllTasks()
	if err != nil {
		return nil
	}
	var tmuxTasksToRestore []*task.Task

	for _, t := range allTasks {
		prefix := s.projectLogPrefix(t.ProjectID)
		if t.Status == task.StatusInit {
			log.Printf("%sRecovering task #%d stuck in init, resetting to pending", prefix, t.ID)
			if err := s.database.UpdateTaskStatus(t.ID, task.StatusPending); err != nil {
				log.Printf("%sFailed to reset task #%d: %v", prefix, t.ID, err)
			}
		}
		if t.Status == task.StatusFinalizing {
			log.Printf("%sRecovering task #%d stuck in finalizing, resetting to tmux", prefix, t.ID)
			if err := s.database.UpdateTaskStatus(t.ID, task.StatusTmux); err != nil {
				log.Printf("%sFailed to reset task #%d: %v", prefix, t.ID, err)
			} else {
				tmuxTasksToRestore = append(tmuxTasksToRestore, t)
			}
		}
		if t.Status == task.StatusSummarizing {
			log.Printf("%sRecovering task #%d stuck in %s, resetting to pending", prefix, t.ID, t.Status)
			if err := s.database.ResetTaskForRetry(t.ID); err != nil {
				log.Printf("%sFailed to reset task #%d: %v", prefix, t.ID, err)
			}
		}
		if t.Status == task.StatusMergeBlocked {
			log.Printf("%sRecovering merge-blocked task #%d, restarting merge agent", prefix, t.ID)
			if err := s.startTaskAgent(t); err != nil {
				log.Printf("%sFailed to restart merge-blocked task #%d: %v", prefix, t.ID, err)
			}
		}
		if t.Status == task.StatusAwaitingApproval {
			log.Printf("%sTask #%d is awaiting approval (use 'continue' command)", prefix, t.ID)
		}
		if t.Status == task.StatusTmux {
			tmuxTasksToRestore = append(tmuxTasksToRestore, t)
		}
	}

	// Restore tmux sessions for tasks that had active tmux sessions before daemon restart
	if len(tmuxTasksToRestore) > 0 && tmux.IsAvailable() {
		for _, t := range tmuxTasksToRestore {
			prefix := s.projectLogPrefix(t.ProjectID)
			resumed, err := s.restoreTmuxSession(t)
			if err != nil {
				log.Printf("%sFailed to restore tmux session for task #%d: %v", prefix, t.ID, err)
			} else if resumed {
				log.Printf("%sRestored tmux session for task #%d (auto-resumed previous Claude chat)", prefix, t.ID)
			} else {
				log.Printf("%sRestored tmux session for task #%d (no prior chat found; started fresh — use /resume to restore manually)", prefix, t.ID)
			}
		}
	}

	return nil
}

// restoreTmuxSession recreates a tmux session for a task whose session was
// lost during a daemon restart. If a previous Claude chat session ID is
// recorded for the task in the chats table, the spawned Claude process is
// invoked with `--resume <id>` so the chat is automatically restored.
// Returns (resumed, error) where resumed indicates whether a prior session
// was found and resumed.
func (s *Server) restoreTmuxSession(t *task.Task) (bool, error) {
	if t.WorktreePath == "" || !dirExists(t.WorktreePath) {
		return false, fmt.Errorf("worktree path does not exist: %s", t.WorktreePath)
	}

	taskID := fmt.Sprintf("%d", t.ID)

	pc, _ := s.getProjectContext(t.ProjectID)
	projectName := ""
	if pc != nil {
		projectName = pc.cfg.Project.Name
	}

	session := tmux.NewSession(projectName, taskID, t.WorktreePath)

	if session.Exists() {
		log.Printf("%sTmux session already exists for task #%d, skipping restore", s.projectLogPrefix(t.ProjectID), t.ID)
		return false, nil
	}

	// Look up the most recent Claude chat session for this task so we can
	// auto-resume it. Failures here are non-fatal: we fall back to a fresh
	// `claude` invocation, matching the pre-resume behavior.
	var resumeSessionID string
	if chat, err := s.database.GetLatestChat(t.ID); err != nil {
		log.Printf("%sWarning: failed to look up chat for task #%d: %v", s.projectLogPrefix(t.ProjectID), t.ID, err)
	} else if chat != nil {
		resumeSessionID = chat.SessionID
	}

	sortieDir := filepath.Join(t.WorktreePath, ".sortie")
	if err := os.MkdirAll(sortieDir, 0755); err != nil {
		return false, fmt.Errorf("failed to create sortie dir: %w", err)
	}

	yolo := pc != nil && pc.cfg.Claude.Yolo
	scriptFile := filepath.Join(sortieDir, "run-restore.sh")
	if err := writeClaudeScript(scriptFile, yolo, resumeSessionID); err != nil {
		return false, fmt.Errorf("failed to write wrapper script: %w", err)
	}

	var setupCmd string
	if pc != nil {
		setupCmd = pc.cfg.TmuxSetupCommand
	}

	if tmux.SetupCommandControlsAgent(setupCmd) {
		if err := session.Create(""); err != nil {
			return false, fmt.Errorf("failed to create tmux session: %w", err)
		}
	} else {
		if err := session.Create("bash", scriptFile); err != nil {
			return false, fmt.Errorf("failed to create tmux session: %w", err)
		}
	}

	// Run tmux setup command if configured
	if setupCmd != "" {
		vars := &tmux.SetupVars{
			ClaudeCommand: buildClaudeCommand(yolo, resumeSessionID),
			RunAgent:      scriptFile,
		}
		if err := session.RunSetupCommand(setupCmd, vars); err != nil {
			log.Printf("%sWarning: tmux setup command failed for restored task #%d: %v", s.projectLogPrefix(t.ProjectID), t.ID, err)
		}
	}

	return resumeSessionID != "", nil
}
