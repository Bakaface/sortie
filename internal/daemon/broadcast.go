package daemon

import (
	"log"
	"net"

	"github.com/aface/sortie/internal/agent"
	"github.com/aface/sortie/internal/task"
	"github.com/aface/sortie/internal/tmux"
)

func (s *Server) onAgentStateChange(a *agent.Agent, oldState, newState agent.State) {
	info := agentToInfo(a)
	s.broadcastToSubscribers(MsgAgentUpdate, AgentUpdateResponse{Agent: info})

	prefix := s.projectLogPrefix(a.Task.ProjectID)
	taskTitle := a.Task.Title
	if taskTitle == "" {
		taskTitle = a.Task.Description
	}

	switch newState {
	case agent.StateCompleted:
		refreshedTask, err := s.database.GetTask(a.Task.ID)
		if err == nil && (refreshedTask.Status == task.StatusAwaitingApproval || refreshedTask.Status == task.StatusTmux || refreshedTask.Status == task.StatusArtifactMissing) {
			log.Printf("%sAgent %s paused task #%d for approval", prefix, a.ID, a.Task.ID)
			if err := s.notifier.AgentWaitingForInput(a.ID, taskTitle); err != nil {
				log.Printf("%sWarning: notification failed: %v", prefix, err)
			}
			return
		}

		log.Printf("%sAgent %s completed task #%d", prefix, a.ID, a.Task.ID)
		if err := s.database.UpdateTaskStatus(a.Task.ID, task.StatusCompleted); err != nil {
			log.Printf("%sFailed to update task status: %v", prefix, err)
		}
		if pc, err := s.getProjectContext(a.Task.ProjectID); err == nil {
			if err := tmux.KillSessionsForTask(pc.cfg.Project.Name, a.ID); err != nil {
				log.Printf("%sWarning: failed to kill tmux sessions for task %s: %v", prefix, a.ID, err)
			}
		}
		if err := s.notifier.AgentCompleted(a.ID, taskTitle); err != nil {
			log.Printf("%sWarning: notification failed: %v", prefix, err)
		}

		s.checkProjectTasksDone(a.Task.ProjectID)

	case agent.StateFailed:
		log.Printf("%sAgent %s failed task #%d: %s", prefix, a.ID, a.Task.ID, a.Error)
		if err := s.database.UpdateTaskError(a.Task.ID, a.Error); err != nil {
			log.Printf("%sFailed to update task error: %v", prefix, err)
		}
		if pc, err := s.getProjectContext(a.Task.ProjectID); err == nil {
			if err := tmux.KillSessionsForTask(pc.cfg.Project.Name, a.ID); err != nil {
				log.Printf("%sWarning: failed to kill tmux sessions for task %s: %v", prefix, a.ID, err)
			}
		}
		if err := s.notifier.AgentFailed(a.ID, taskTitle, a.Error); err != nil {
			log.Printf("%sWarning: notification failed: %v", prefix, err)
		}

		s.checkProjectTasksDone(a.Task.ProjectID)

	case agent.StateWaitingForInput:
		if err := s.notifier.AgentWaitingForInput(a.ID, taskTitle); err != nil {
			log.Printf("%sWarning: notification failed: %v", prefix, err)
		}
	}
}

func (s *Server) checkProjectTasksDone(projectID int64) {
	tasks, err := s.database.GetTasksByProject(projectID)
	if err != nil || len(tasks) == 0 {
		return
	}
	for _, t := range tasks {
		switch t.Status {
		case task.StatusPending, task.StatusRunning, task.StatusAwaitingApproval, task.StatusTmux, task.StatusFinalizing, task.StatusSummarizing, task.StatusMergeBlocked, task.StatusInit, task.StatusArtifactMissing:
			return
		}
	}
	log.Printf("%sAll tasks completed for project %d", s.projectLogPrefix(projectID), projectID)
	if err := s.notifier.AllTasksCompleted(); err != nil {
		log.Printf("%sWarning: notification failed: %v", s.projectLogPrefix(projectID), err)
	}
}

func (s *Server) broadcastToSubscribers(msgType MessageType, payload any) {
	s.mu.RLock()
	subs := make([]net.Conn, 0, len(s.subscribers))
	for conn := range s.subscribers {
		subs = append(subs, conn)
	}
	s.mu.RUnlock()

	for _, conn := range subs {
		s.sendMessage(conn, msgType, payload)
	}
}

func (s *Server) broadcastTaskUpdate(taskID int64) {
	t, err := s.database.GetTask(taskID)
	if err != nil {
		log.Printf("Failed to re-fetch task #%d for broadcast: %v", taskID, err)
		return
	}
	s.broadcastToSubscribers(MsgTaskUpdate, TaskUpdateResponse{Task: s.taskToInfo(t)})
}

func (s *Server) sendMessage(conn net.Conn, msgType MessageType, payload any) {
	msg, err := NewMessage(msgType, payload)
	if err != nil {
		log.Printf("Failed to create message: %v", err)
		return
	}

	data, err := EncodeMessage(msg)
	if err != nil {
		log.Printf("Failed to encode message: %v", err)
		return
	}

	conn.Write(data)
}

func (s *Server) sendError(conn net.Conn, message string) {
	s.sendMessage(conn, MsgError, ErrorResponse{Message: message})
}

func agentToInfo(a *agent.Agent) AgentInfo {
	return AgentInfo{
		ID:          a.ID,
		TaskID:      a.Task.ID,
		Description: a.Task.Description,
		WorkDir:     a.WorkDir,
		State:       AgentState(a.GetState()),
		StartedAt:   a.StartedAt,
		Error:       a.Error,
	}
}

func (s *Server) taskToInfo(t *task.Task) TaskInfo {
	info := TaskInfo{
		ID:            t.ID,
		ProjectID:     t.ProjectID,
		Title:         t.Title,
		Description:   t.Description,
		Slug:          t.Slug,
		Workflow:      t.Workflow,
		Status:        string(t.Status),
		Priority:      string(t.Priority),
		StepIndex:     t.StepIndex,
		CurrentStep:   t.CurrentStep,
		LoopIteration: t.LoopIteration,
		BranchName:     t.BranchName,
		Branch:         t.Branch,
		TargetBranch:   t.TargetBranch,
		CheckoutBranch: t.CheckoutBranch,
		Worktree:      t.Worktree,
		WorktreePath:  t.WorktreePath,
		ErrorMessage:  t.ErrorMessage,
		Context:       t.Context,
		Images:        t.Images,
		Commits:       t.Commits,
		BlockedBy:     t.BlockedBy,
		CreatedAt:     t.CreatedAt,
		StartedAt:     t.StartedAt,
		CompletedAt:   t.CompletedAt,
	}

	if proj, err := s.database.GetProject(t.ProjectID); err == nil {
		info.ProjectName = proj.Name
		info.ProjectPath = proj.Path
	}

	s.mu.RLock()
	if activity, ok := s.tmuxActivity[t.ID]; ok {
		info.TmuxActivity = activity
	}
	s.mu.RUnlock()

	return info
}
