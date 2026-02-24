package daemon

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/aface/sortie/internal/agent"
	"github.com/aface/sortie/internal/task"
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
		log.Printf("Starting agent for task #%d: %s", t.ID, title)
		if err := s.startTaskAgent(t); err != nil {
			log.Printf("Failed to start agent for task #%d: %v", t.ID, err)
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

	for _, t := range runningTasks {
		log.Printf("Recovering orphaned task #%d, resetting to pending", t.ID)
		if err := s.database.ResetTaskForRetry(t.ID); err != nil {
			log.Printf("Failed to reset task #%d: %v", t.ID, err)
		}
	}

	allTasks, err := s.database.GetAllTasks()
	if err != nil {
		return nil
	}
	for _, t := range allTasks {
		if t.Status == task.StatusInit {
			log.Printf("Recovering task #%d stuck in init, resetting to pending", t.ID)
			if err := s.database.UpdateTaskStatus(t.ID, task.StatusPending); err != nil {
				log.Printf("Failed to reset task #%d: %v", t.ID, err)
			}
		}
		if t.Status == task.StatusFinalizing {
			log.Printf("Recovering task #%d stuck in finalizing, resetting to tmux", t.ID)
			if err := s.database.UpdateTaskStatus(t.ID, task.StatusTmux); err != nil {
				log.Printf("Failed to reset task #%d: %v", t.ID, err)
			}
		}
		if t.Status == task.StatusSummarizing {
			log.Printf("Recovering task #%d stuck in summarizing, resetting to pending", t.ID)
			if err := s.database.ResetTaskForRetry(t.ID); err != nil {
				log.Printf("Failed to reset task #%d: %v", t.ID, err)
			}
		}
		if t.Status == task.StatusAwaitingApproval {
			log.Printf("Task #%d is awaiting approval (use 'continue' command)", t.ID)
		}
		if t.Status == task.StatusTmux {
			log.Printf("Task #%d has tmux session running (use 'continue' command)", t.ID)
		}
		if t.Status == task.StatusArtifactMissing {
			log.Printf("Task #%d has missing artifact (use 'continue' command to skip)", t.ID)
		}
	}

	return nil
}
