package daemon

import (
	"log"
	"net"
	"time"

	"github.com/Bakaface/sortie/internal/agent"
	gitpkg "github.com/Bakaface/sortie/internal/git"
	"github.com/Bakaface/sortie/internal/task"
	"github.com/Bakaface/sortie/internal/tmux"
)

// broadcastWriteTimeout bounds how long the daemon will wait when pushing a
// broadcast (agent/task/tmux updates) to a single subscriber. A healthy
// subscriber drains a buffered message in microseconds; a 2-second budget
// is conservative enough to absorb a stop-the-world GC on the consumer
// while still preventing one stalled peer from blocking the broadcast loop
// for every other subscriber. The conn is dropped on timeout (or any other
// write error) — see broadcastToSubscribers.
const broadcastWriteTimeout = 2 * time.Second

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
		if err == nil && (refreshedTask.Status == task.StatusAwaitingApproval || refreshedTask.Status == task.StatusTmux) {
			log.Printf("%sAgent %s paused task #%d for approval", prefix, a.ID, a.Task.ID)
			if err := s.notifier.AgentWaitingForInput(a.ID, taskTitle); err != nil {
				log.Printf("%sWarning: notification failed: %v", prefix, err)
			}
			return
		}
		// Mid-step suspend on spawned children: do NOT run finalization.
		// The engine left task_waits_on edges behind; the poller will resume
		// the parent at the same step once every child is terminal.
		if err == nil && refreshedTask.Status == task.StatusAwaitingChildren {
			waits, _ := s.database.GetTaskWaitsOn(a.Task.ID)
			log.Printf("%sAgent %s suspended task #%d on %d children: %v", prefix, a.ID, a.Task.ID, len(waits), waits)
			return
		}

		log.Printf("%sAgent %s completed task #%d, starting finalization", prefix, a.ID, a.Task.ID)

		// Kill tmux sessions before finalization
		if pc, err := s.getProjectContext(a.Task.ProjectID); err == nil {
			if err := tmux.KillSessionsForTask(pc.cfg.Project.Name, a.ID); err != nil {
				log.Printf("%sWarning: failed to kill tmux sessions for task %s: %v", prefix, a.ID, err)
			}
		}

		// Run merge + summarization + completion asynchronously
		go s.finalizeCompletedTask(a.Task, a.ID, taskTitle)

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

// finalizeCompletedTask handles merge, summarization, and completion for a
// task whose agent has finished running all workflow steps.
func (s *Server) finalizeCompletedTask(t *task.Task, agentID string, taskTitle string) {
	prefix := s.projectLogPrefix(t.ProjectID)

	pc, err := s.getProjectContext(t.ProjectID)
	if err != nil {
		log.Printf("%sWarning: failed to get project context for task #%d, marking completed: %v", prefix, t.ID, err)
		if err := s.database.UpdateTaskStatus(t.ID, task.StatusCompleted); err != nil {
			log.Printf("%sFailed to update task status: %v", prefix, err)
		}
		s.broadcastTaskUpdate(t.ID)
		if err := s.notifier.AgentCompleted(agentID, taskTitle); err != nil {
			log.Printf("%sWarning: notification failed: %v", prefix, err)
		}
		s.checkProjectTasksDone(t.ProjectID)
		return
	}

	// Fast-track: if no meaningful changes, skip finalization
	if t.WorktreePath != "" && t.Worktree {
		hasChanges, err := gitpkg.HasMeaningfulChanges(t.WorktreePath, noiseFiles)
		if err != nil {
			log.Printf("%sWarning: failed to check for meaningful changes for task #%d: %v", prefix, t.ID, err)
		} else if !hasChanges {
			log.Printf("%sTask #%d: no meaningful changes detected, fast-tracking to completed", prefix, t.ID)
			s.cleanupWorktreeAndBranch(pc, t)
			if err := s.database.UpdateTaskStatus(t.ID, task.StatusCompleted); err != nil {
				log.Printf("%sError: failed to mark task #%d as completed: %v", prefix, t.ID, err)
			}
			s.broadcastTaskUpdate(t.ID)
			if err := s.notifier.AgentCompleted(agentID, taskTitle); err != nil {
				log.Printf("%sWarning: notification failed: %v", prefix, err)
			}
			s.checkProjectTasksDone(t.ProjectID)
			return
		}
	}

	// Set finalizing status
	if err := s.database.UpdateTaskStatus(t.ID, task.StatusFinalizing); err != nil {
		log.Printf("%sWarning: failed to set finalizing status for task #%d: %v", prefix, t.ID, err)
	}
	s.broadcastTaskUpdate(t.ID)

	// Run merge → summarize → cleanup → complete
	s.runFinalization(t, pc)

	log.Printf("%sAgent %s completed task #%d", prefix, agentID, t.ID)
	if err := s.notifier.AgentCompleted(agentID, taskTitle); err != nil {
		log.Printf("%sWarning: notification failed: %v", prefix, err)
	}
	s.checkProjectTasksDone(t.ProjectID)
}

func (s *Server) checkProjectTasksDone(projectID int64) {
	tasks, err := s.database.GetTasksByProject(projectID)
	if err != nil || len(tasks) == 0 {
		return
	}
	for _, t := range tasks {
		switch t.Status {
		case task.StatusPending, task.StatusRunning, task.StatusAwaitingApproval, task.StatusAwaitingChildren, task.StatusTmux, task.StatusFinalizing, task.StatusSummarizing, task.StatusSummarizingStep, task.StatusMergeBlocked, task.StatusResolvingConflicts, task.StatusInit:
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

	// Collect dead conns during iteration, then drop them in one critical
	// section after the loop. Deleting-while-iterating from s.subscribers
	// here would race against handleConnection's own cleanup defer.
	var failed []net.Conn
	for _, conn := range subs {
		if err := s.broadcastSend(conn, msgType, payload); err != nil {
			log.Printf("daemon: client write failed, dropping conn: %v", err)
			failed = append(failed, conn)
		}
	}

	if len(failed) > 0 {
		s.dropDeadConns(failed)
	}
}

// broadcastSend writes a single broadcast message to one subscriber with
// a write deadline. Returns the write error so the caller can drop the
// conn from s.subscribers / s.clients. The deadline is cleared after the
// write so subsequent RPC writes on the same conn (handled by handleMessage)
// are not subject to it — broadcasts are the only push-from-daemon path.
func (s *Server) broadcastSend(conn net.Conn, msgType MessageType, payload any) error {
	msg, err := NewMessage(msgType, payload)
	if err != nil {
		log.Printf("Failed to create broadcast message: %v", err)
		return err
	}
	data, err := EncodeMessage(msg)
	if err != nil {
		log.Printf("Failed to encode broadcast message: %v", err)
		return err
	}
	_ = conn.SetWriteDeadline(time.Now().Add(broadcastWriteTimeout))
	_, writeErr := conn.Write(data)
	_ = conn.SetWriteDeadline(time.Time{})
	return writeErr
}

// dropDeadConns removes the given conns from s.clients and s.subscribers
// and closes each one. Idempotent w.r.t. handleConnection's own deferred
// cleanup: delete on a missing key is a no-op and net.Conn.Close on an
// already-closed conn is benign.
func (s *Server) dropDeadConns(conns []net.Conn) {
	s.mu.Lock()
	for _, conn := range conns {
		delete(s.clients, conn)
		delete(s.subscribers, conn)
	}
	s.mu.Unlock()
	for _, conn := range conns {
		conn.Close()
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

// sendMessage writes a 1:1 RPC response to conn. No write deadline is applied
// — RPC responses are naturally bounded by a specific in-flight request, and
// handleConnection's scanner loop will detect a dead peer on the next read.
// The write error is surfaced so callers can log it; the returned value is
// intentionally ignored by most callers because (a) there's nothing actionable
// for an RPC handler to do once the reply has failed, and (b) the broken
// conn will be cleaned up by handleConnection's exit-defer.
func (s *Server) sendMessage(conn net.Conn, msgType MessageType, payload any) error {
	msg, err := NewMessage(msgType, payload)
	if err != nil {
		log.Printf("Failed to create message: %v", err)
		return err
	}

	data, err := EncodeMessage(msg)
	if err != nil {
		log.Printf("Failed to encode message: %v", err)
		return err
	}

	if _, writeErr := conn.Write(data); writeErr != nil {
		log.Printf("daemon: RPC write to client failed (type=%s): %v", msgType, writeErr)
		return writeErr
	}
	return nil
}

func (s *Server) sendError(conn net.Conn, message string) {
	// sendMessage already logs the underlying write error; we don't propagate
	// it because there's no path back to the caller (no Go return).
	_ = s.sendMessage(conn, MsgError, ErrorResponse{Message: message})
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
		ID:               t.ID,
		ProjectID:        t.ProjectID,
		Title:            t.Title,
		Description:      t.Description,
		Slug:             t.Slug,
		Workflow:         t.Workflow,
		Status:           string(t.Status),
		Priority:         string(t.Priority),
		StepIndex:        t.StepIndex,
		CurrentStep:      t.CurrentStep,
		LoopIteration:    t.LoopIteration,
		BranchName:       t.BranchName,
		Branch:           t.Branch,
		TargetBranch:     t.TargetBranch,
		CheckoutBranch:   t.CheckoutBranch,
		Worktree:         t.Worktree,
		WorktreePath:     t.WorktreePath,
		WorktreeDetached: t.WorktreeDetached,
		ErrorMessage:     t.ErrorMessage,
		Context:          t.Context,
		Images:           t.Images,
		Commits:          t.Commits,
		BlockedBy:        t.BlockedBy,
		CreatedAt:        t.CreatedAt,
		StartedAt:        t.StartedAt,
		CompletedAt:      t.CompletedAt,
	}

	// Populate waits-on for tasks suspended on spawned children so the TUI
	// (and other clients) can surface "task #N awaiting [#A, #B]".
	if t.Status == task.StatusAwaitingChildren {
		if waits, err := s.database.GetTaskWaitsOn(t.ID); err == nil {
			info.WaitsOn = waits
		}
	}

	if proj, err := s.database.GetProject(t.ProjectID); err == nil {
		info.ProjectName = proj.Name
		info.ProjectPath = proj.Path
	}

	// Populate TargetBranch with effective base branch (and StepHuman / a
	// best-effort CurrentStep for paused tmux tasks) by peeking the cached
	// project config. We intentionally do not trigger a load here — the
	// serializer must not issue DB queries; missing cache simply means these
	// optional fields stay at their defaults.
	if info.Worktree || t.Status == task.StatusTmux {
		s.projectsMu.RLock()
		if pc, ok := s.projects[t.ProjectID]; ok {
			if info.TargetBranch == "" && info.Worktree && pc.cfg.Git.BaseBranch != "" {
				info.TargetBranch = pc.cfg.Git.BaseBranch
			}
			if t.Status == task.StatusTmux && t.Workflow != "" {
				if wf := pc.cfg.GetWorkflow(t.Workflow); wf != nil {
					// The engine clears CurrentStep and bumps StepIndex past
					// the tmux step before pausing, so the step that owns the
					// tmux session is at StepIndex-1.
					idx := t.StepIndex - 1
					if idx >= 0 && idx < len(wf.Steps) {
						step := wf.Steps[idx]
						info.StepHuman = step.Human
						if info.CurrentStep == "" {
							info.CurrentStep = step.Name
						}
					}
				}
			}
		}
		s.projectsMu.RUnlock()
	}

	// tmux_direct tasks have no workflow step but are inherently interactive
	// (the user is driving the tmux session), so treat them as human.
	if t.Status == task.StatusTmux && t.Workflow == "" {
		info.StepHuman = true
	}

	s.mu.RLock()
	if activity, ok := s.tmuxActivity[t.ID]; ok {
		info.TmuxActivity = activity
	}
	s.mu.RUnlock()

	// Populate latest chat info
	if chat, err := s.database.GetLatestChat(t.ID); err == nil && chat != nil {
		info.LatestChat = &ChatInfo{
			SessionID:       chat.SessionID,
			TmuxSessionName: chat.TmuxSessionName,
			StepName:        chat.StepName,
		}
	}

	return info
}
