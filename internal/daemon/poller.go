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
		if t.Status == task.StatusArtifactMissing {
			log.Printf("%sTask #%d has missing artifact (use 'continue' command to skip)", prefix, t.ID)
		}
	}

	// Restore tmux sessions for tasks that had active tmux sessions before daemon restart
	if len(tmuxTasksToRestore) > 0 && tmux.IsAvailable() {
		for _, t := range tmuxTasksToRestore {
			prefix := s.projectLogPrefix(t.ProjectID)
			if err := s.restoreTmuxSession(t); err != nil {
				log.Printf("%sFailed to restore tmux session for task #%d: %v", prefix, t.ID, err)
			} else {
				log.Printf("%sRestored tmux session for task #%d (use /resume in claude to restore chat)", prefix, t.ID)
			}
		}
	}

	return nil
}

// restoreTmuxSession recreates a tmux session for a task whose session was
// lost during a daemon restart. It spawns an empty Claude Code session in
// the task's worktree — the user can restore their previous chat via /resume.
func (s *Server) restoreTmuxSession(t *task.Task) error {
	if t.WorktreePath == "" || !dirExists(t.WorktreePath) {
		return fmt.Errorf("worktree path does not exist: %s", t.WorktreePath)
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
		return nil
	}

	sortieDir := filepath.Join(t.WorktreePath, ".sortie")
	if err := os.MkdirAll(sortieDir, 0755); err != nil {
		return fmt.Errorf("failed to create sortie dir: %w", err)
	}

	claudeCmd := "claude"
	if pc != nil && pc.cfg.Claude.Yolo {
		claudeCmd = "claude --dangerously-skip-permissions"
	}

	scriptFile := filepath.Join(sortieDir, "run-restore.sh")
	script := fmt.Sprintf("#!/bin/bash\n%s\nexec bash\n", claudeCmd)
	if err := os.WriteFile(scriptFile, []byte(script), 0755); err != nil {
		return fmt.Errorf("failed to write wrapper script: %w", err)
	}

	if err := session.Create("bash", scriptFile); err != nil {
		return fmt.Errorf("failed to create tmux session: %w", err)
	}

	return nil
}
