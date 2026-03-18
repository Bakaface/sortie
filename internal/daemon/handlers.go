package daemon

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	gitpkg "github.com/aface/sortie/internal/git"
	"github.com/aface/sortie/internal/task"
	"github.com/aface/sortie/internal/tmux"
	"github.com/aface/sortie/internal/workflow"
	"github.com/aface/sortie/internal/config"
)

const (
	// titleGenerationTimeout is the maximum time allowed for AI-based task title generation.
	titleGenerationTimeout = 30 * time.Second
)

// noiseFiles are files that don't count as meaningful changes when checking
// whether a task produced real output (e.g. when fast-tracking to completed).
var noiseFiles = []string{".claude-output.log", "CLAUDE.md"}

func (s *Server) handleListAgents(conn net.Conn) {
	agents := s.manager.ListAgents()
	infos := make([]AgentInfo, len(agents))

	for i, a := range agents {
		infos[i] = agentToInfo(a)
	}

	s.sendMessage(conn, MsgAgentList, AgentListResponse{Agents: infos})
}

func (s *Server) handleListTasks(conn net.Conn, req ListTasksRequest) {
	var tasks []*task.Task
	var err error

	if req.ProjectID > 0 {
		tasks, err = s.database.GetTasksByProject(req.ProjectID)
	} else if req.ProjectName != "" {
		tasks, err = s.database.GetTasksByProjectName(req.ProjectName)
	} else {
		tasks, err = s.database.GetAllTasks()
	}
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to get tasks: %v", err))
		return
	}

	infos := make([]TaskInfo, len(tasks))
	for i, t := range tasks {
		infos[i] = s.taskToInfo(t)
	}

	s.sendMessage(conn, MsgTaskList, TaskListResponse{Tasks: infos})
}

func (s *Server) handleStartAgent(conn net.Conn, req StartAgentRequest) {
	t, err := s.database.GetTask(req.TaskID)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to get task: %v", err))
		return
	}

	if t.Status != task.StatusPending && t.Status != task.StatusAwaitingApproval && t.Status != task.StatusTmux {
		s.sendError(conn, fmt.Sprintf("task is not startable (status: %s)", t.Status))
		return
	}

	if err := s.startTaskAgent(t); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to start agent: %v", err))
		return
	}

	s.sendMessage(conn, MsgOK, OKResponse{Message: "agent started"})
}

func (s *Server) handleStopAgent(conn net.Conn, req StopAgentRequest) {
	if err := s.manager.StopAgent(req.AgentID); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to stop agent: %v", err))
		return
	}

	s.sendMessage(conn, MsgOK, OKResponse{Message: "agent stopped"})
}

func (s *Server) handleSubscribe(conn net.Conn) {
	s.mu.Lock()
	s.subscribers[conn] = true
	s.mu.Unlock()

	s.sendMessage(conn, MsgOK, OKResponse{Message: "subscribed"})
}

func (s *Server) handleUnsubscribe(conn net.Conn) {
	s.mu.Lock()
	delete(s.subscribers, conn)
	s.mu.Unlock()

	s.sendMessage(conn, MsgOK, OKResponse{Message: "unsubscribed"})
}

func (s *Server) handleGetOutput(conn net.Conn, req GetOutputRequest) {
	lines, total, err := s.manager.GetOutput(req.AgentID, req.FromLine)
	if err != nil {
		taskID, parseErr := strconv.ParseInt(req.AgentID, 10, 64)
		if parseErr == nil {
			t, getErr := s.database.GetTask(taskID)
			if getErr == nil {
				dataDir := s.getProjectDataDir(t)
				logsDir := workflow.ProjectLogsDir(dataDir, taskID)
				entries, readErr := os.ReadDir(logsDir)
				if readErr == nil {
					var allLines []string
					for _, entry := range entries {
						if entry.IsDir() || filepath.Ext(entry.Name()) != ".log" {
							continue
						}
						allLines = append(allLines, readLogFile(filepath.Join(logsDir, entry.Name()))...)
					}
					total = len(allLines)
					if req.FromLine < total {
						lines = allLines[req.FromLine:]
					}
				}
			}
		}
	}

	s.sendMessage(conn, MsgOutputChunk, OutputChunkResponse{
		AgentID:    req.AgentID,
		Lines:      lines,
		TotalLines: total,
	})
}

func (s *Server) handleGetTask(conn net.Conn, req GetTaskRequest) {
	t, err := s.database.GetTask(req.TaskID)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to get task: %v", err))
		return
	}

	info := s.taskToInfo(t)
	s.sendMessage(conn, MsgGetTask, GetTaskResponse{Task: info})
}

func (s *Server) handleRetryTask(conn net.Conn, req RetryTaskRequest) {
	// Kill any stale tmux sessions for this task
	agentID := fmt.Sprintf("%d", req.TaskID)
	if t, err := s.database.GetTask(req.TaskID); err == nil {
		if pc, err := s.getProjectContext(t.ProjectID); err == nil {
			if err := tmux.KillSessionsForTask(pc.cfg.Project.Name, agentID); err != nil {
				log.Printf("%sWarning: failed to kill tmux sessions for task #%d: %v", s.projectLogPrefix(t.ProjectID), req.TaskID, err)
			}
		}
	}

	// Stop any running agent for this task
	_ = s.manager.StopAgent(agentID)

	if err := s.database.ResetTaskForRetry(req.TaskID); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to reset task: %v", err))
		return
	}

	s.broadcastTaskUpdate(req.TaskID)
	s.sendMessage(conn, MsgOK, OKResponse{Message: "task reset for retry"})
}

func (s *Server) handleContinueTask(conn net.Conn, req ContinueTaskRequest) {
	t, err := s.database.GetTask(req.TaskID)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to get task: %v", err))
		return
	}

	if t.Status == task.StatusAwaitingApproval || t.Status == task.StatusTmux {
		agentID := fmt.Sprintf("%d", t.ID)
		pc, pcErr := s.getProjectContext(t.ProjectID)
		if pcErr == nil {
			if err := tmux.KillSessionsForTask(pc.cfg.Project.Name, agentID); err != nil {
				log.Printf("%sWarning: failed to kill tmux sessions for task #%d: %v", s.projectLogPrefix(t.ProjectID), t.ID, err)
			}
		}

		// Ensure worktree exists for worktree tasks before continuing.
		// The worktree may have been cleaned up after a previous completion/merge.
		if pcErr == nil {
			if err := s.ensureWorktree(t, pc); err != nil {
				s.sendError(conn, err.Error())
				return
			}
		}

		origStatus := t.Status

		if err := s.database.UpdateTaskStatus(t.ID, task.StatusRunning); err != nil {
			s.sendError(conn, fmt.Sprintf("failed to update task status: %v", err))
			return
		}

		if err := s.startTaskAgent(t); err != nil {
			_ = s.database.UpdateTaskStatus(t.ID, origStatus)
			s.sendError(conn, fmt.Sprintf("failed to start agent: %v", err))
			return
		}

		s.sendMessage(conn, MsgOK, OKResponse{Message: "task continued and resumed"})
		return
	}

	if t.Status == task.StatusArtifactMissing {
		if err := s.database.UpdateTaskStep(t.ID, t.StepIndex+1, ""); err != nil {
			s.sendError(conn, fmt.Sprintf("failed to advance task step: %v", err))
			return
		}
		if err := s.database.UpdateTaskStatus(t.ID, task.StatusPending); err != nil {
			s.sendError(conn, fmt.Sprintf("failed to update task status: %v", err))
			return
		}
		s.broadcastTaskUpdate(t.ID)
		log.Printf("%sTask #%d continued past artifact-missing at step %d", s.projectLogPrefix(t.ProjectID), t.ID, t.StepIndex)
		s.sendMessage(conn, MsgOK, OKResponse{Message: fmt.Sprintf("task #%d continued past missing artifact", t.ID)})
		return
	}

	if !t.Status.IsTerminal() {
		s.sendError(conn, fmt.Sprintf("task is not in a continuable state (status: %s)", t.Status))
		return
	}

	// Terminal tasks (completed/failed that made it here) - workflow selected by user
	if req.Workflow != "" {
		// User selected a workflow — reset task to run through it
		if err := s.database.ResetTaskForContinue(t.ID, req.Workflow, req.Prompt); err != nil {
			s.sendError(conn, fmt.Sprintf("failed to reset task for continue: %v", err))
			return
		}
		s.broadcastTaskUpdate(t.ID)
		log.Printf("%sTask #%d continuing with workflow %q", s.projectLogPrefix(t.ProjectID), t.ID, req.Workflow)
		s.sendMessage(conn, MsgOK, OKResponse{Message: fmt.Sprintf("task #%d continuing with workflow %q", t.ID, req.Workflow)})
		return
	}

	// Fallback: no workflow specified — use tmux (legacy behavior)
	if t.Status == task.StatusFailed {
		if err := s.database.ResetTaskForRetryFromStep(req.TaskID); err != nil {
			s.sendError(conn, fmt.Sprintf("failed to reset task: %v", err))
			return
		}
		s.broadcastTaskUpdate(t.ID)
		log.Printf("%sTask #%d retrying from step %d", s.projectLogPrefix(t.ProjectID), t.ID, t.StepIndex)
		s.sendMessage(conn, MsgOK, OKResponse{Message: fmt.Sprintf("task #%d retrying from step %d", t.ID, t.StepIndex)})
		return
	}

	if !tmux.IsAvailable() {
		s.sendError(conn, "tmux is not installed or not in PATH")
		return
	}

	pc, err := s.getProjectContext(t.ProjectID)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to get project context: %v", err))
		return
	}

	if !t.Worktree {
		// No-worktree mode: run in project root
		t.WorktreePath = pc.repoRoot
	} else if err := s.ensureWorktree(t, pc); err != nil {
		s.sendError(conn, err.Error())
		return
	}

	var claudeMD strings.Builder
	fmt.Fprintf(&claudeMD, "# Continue Task #%d: %s\n\n", t.ID, t.Title)
	claudeMD.WriteString("You are continuing work on a previously completed task.\n\n")
	claudeMD.WriteString("## Task Description\n\n")
	claudeMD.WriteString(t.Description)
	claudeMD.WriteString("\n\n")
	if t.Context != "" {
		claudeMD.WriteString("## Previous Context\n\n")
		claudeMD.WriteString(t.Context)
		claudeMD.WriteString("\n\n")
	}
	claudeMD.WriteString("The user wants to continue working on this task. Help them with whatever they need.\n")

	claudeMDPath := filepath.Join(t.WorktreePath, "CLAUDE.md")
	if err := os.WriteFile(claudeMDPath, []byte(claudeMD.String()), 0644); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to write CLAUDE.md: %v", err))
		return
	}

	taskID := fmt.Sprintf("%d", t.ID)
	session := tmux.NewSession(pc.cfg.Project.Name, taskID, t.WorktreePath)

	if session.Exists() {
		session.Kill()
	}

	sortieDir := filepath.Join(t.WorktreePath, ".sortie")
	if err := os.MkdirAll(sortieDir, 0755); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to create sortie dir: %v", err))
		return
	}
	scriptFile := filepath.Join(sortieDir, "run-continue.sh")

	if err := writeClaudeScript(scriptFile, pc.cfg.Claude.Yolo); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to write wrapper script: %v", err))
		return
	}

	if err := session.Create("bash", scriptFile); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to create tmux session: %v", err))
		return
	}

	// Run tmux setup command if configured
	if pc.cfg.TmuxSetupCommand != "" {
		if err := session.RunSetupCommand(pc.cfg.TmuxSetupCommand); err != nil {
			log.Printf("%sWarning: tmux setup command failed for task #%d: %v", s.projectLogPrefix(t.ProjectID), t.ID, err)
		}
	}

	if err := s.database.UpdateTaskStatus(t.ID, task.StatusTmux); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to update task status: %v", err))
		return
	}

	s.broadcastTaskUpdate(t.ID)

	log.Printf("%sContinue session started for task #%d (tmux: %s)", s.projectLogPrefix(t.ProjectID), t.ID, session.Name)
	s.sendMessage(conn, MsgOK, OKResponse{Message: fmt.Sprintf("continue session started for task #%d", t.ID)})
}

func (s *Server) handleFinalizeTask(conn net.Conn, req FinalizeTaskRequest) {
	t, err := s.database.GetTask(req.TaskID)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to get task: %v", err))
		return
	}

	if t.Status != task.StatusTmux {
		s.sendError(conn, fmt.Sprintf("task is not in tmux state (status: %s)", t.Status))
		return
	}

	pc, err := s.getProjectContext(t.ProjectID)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to get project context: %v", err))
		return
	}

	// Kill tmux sessions
	agentID := fmt.Sprintf("%d", t.ID)
	if err := tmux.KillSessionsForTask(pc.cfg.Project.Name, agentID); err != nil {
		log.Printf("%sWarning: failed to kill tmux sessions for task #%d: %v", s.projectLogPrefix(t.ProjectID), t.ID, err)
	}

	// Fast-track: if no meaningful changes were made, skip full finalization
	// and go straight to completed, cleaning up worktree and branch.
	if t.WorktreePath != "" && t.Worktree {
		hasChanges, err := gitpkg.HasMeaningfulChanges(t.WorktreePath, noiseFiles)
		if err != nil {
			log.Printf("%sWarning: failed to check for meaningful changes for task #%d: %v", s.projectLogPrefix(t.ProjectID), t.ID, err)
		} else if !hasChanges {
			log.Printf("%sTask #%d: no meaningful changes detected, fast-tracking to completed", s.projectLogPrefix(t.ProjectID), t.ID)
			s.cleanupWorktreeAndBranch(pc, t)
			if err := s.database.UpdateTaskStatus(t.ID, task.StatusCompleted); err != nil {
				log.Printf("%sError: failed to mark task #%d as completed: %v", s.projectLogPrefix(t.ProjectID), t.ID, err)
			}
			s.broadcastTaskUpdate(t.ID)
			s.sendMessage(conn, MsgOK, OKResponse{Message: fmt.Sprintf("task #%d fast-tracked to completed (no changes)", t.ID)})
			return
		}
	}

	// Set finalizing status while we run summarizer + on_complete
	if err := s.database.UpdateTaskStatus(t.ID, task.StatusFinalizing); err != nil {
		log.Printf("%sWarning: failed to set finalizing status for task #%d: %v", s.projectLogPrefix(t.ProjectID), t.ID, err)
	}
	s.broadcastTaskUpdate(t.ID)

	// Respond immediately so the TUI is unblocked and can refresh
	s.sendMessage(conn, MsgOK, OKResponse{Message: fmt.Sprintf("task #%d finalizing", t.ID)})

	// Run summarizer + on_complete asynchronously
	go s.runFinalization(t, pc)
}

func (s *Server) runFinalization(t *task.Task, pc *projectContext) {
	repoRoot := s.getProjectRepoRoot(t)
	if err := pc.engine.FinalizeTask(s.ctx, t); err != nil {
		log.Printf("%sWarning: finalize failed for task #%d: %v", s.projectLogPrefix(t.ProjectID), t.ID, err)
		// Don't fail the whole operation — still mark as completed.
		// Best-effort cleanup of worktree and branch so they don't linger.
		if t.Worktree && repoRoot != "" {
			s.cleanupWorktreeAndBranch(pc, t)
		}
	}

	// Mark task as completed
	if err := s.database.UpdateTaskStatus(t.ID, task.StatusCompleted); err != nil {
		log.Printf("%sError: failed to mark task #%d as completed: %v", s.projectLogPrefix(t.ProjectID), t.ID, err)
		return
	}

	s.broadcastTaskUpdate(t.ID)
	log.Printf("%sTask #%d finalized from tmux continue session", s.projectLogPrefix(t.ProjectID), t.ID)
}

// ensureWorktree ensures a task's worktree exists, creating it if needed.
// It resolves the branch name, creates the worktree, updates the DB, and runs the setup command.
// Returns an error if worktree creation fails.
func (s *Server) ensureWorktree(t *task.Task, pc *projectContext) error {
	if !t.Worktree {
		return nil
	}
	if t.WorktreePath != "" && dirExists(t.WorktreePath) {
		return nil
	}

	if t.CheckoutBranch != "" {
		// Checkout existing branch mode
		if t.Branch == "" {
			t.Branch = t.CheckoutBranch
			if err := s.database.UpdateTaskBranch(t.ID, t.Branch); err != nil {
				log.Printf("%sWarning: failed to persist branch for task #%d: %v", s.projectLogPrefix(t.ProjectID), t.ID, err)
			}
		}
		worktree, err := gitpkg.CheckoutWorktree(pc.repoRoot, t.ID, t.CheckoutBranch)
		if err != nil {
			return fmt.Errorf("failed to checkout worktree for task #%d: %v", t.ID, err)
		}
		t.WorktreePath = worktree.Path
	} else {
		if t.Branch == "" {
			t.Branch = pc.cfg.ResolveBranchForTask(t.ID, t.Title, t.Slug, t.BranchName)
			if err := s.database.UpdateTaskBranch(t.ID, t.Branch); err != nil {
				log.Printf("%sWarning: failed to persist branch for task #%d: %v", s.projectLogPrefix(t.ProjectID), t.ID, err)
			}
		}

		baseBranch := pc.cfg.Git.BaseBranch
		if t.TargetBranch != "" {
			baseBranch = t.TargetBranch
		}
		worktree, err := gitpkg.CreateWorktree(pc.repoRoot, t.ID, baseBranch, t.Branch)
		if err != nil {
			return fmt.Errorf("failed to create worktree for task #%d: %v", t.ID, err)
		}
		t.WorktreePath = worktree.Path
	}
	if err := s.database.UpdateTaskWorktreePath(t.ID, t.WorktreePath); err != nil {
		log.Printf("%sWarning: failed to update worktree path for task #%d: %v", s.projectLogPrefix(t.ProjectID), t.ID, err)
	}
	log.Printf("%sRecreated worktree for task #%d at %s", s.projectLogPrefix(t.ProjectID), t.ID, t.WorktreePath)

	if setupCmd := pc.cfg.GetWorktreeSetupCommand(nil); setupCmd != "" {
		if err := workflow.RunWorktreeSetupCommand(context.Background(), pc.repoRoot, t.WorktreePath, setupCmd); err != nil {
			log.Printf("%sWarning: worktree setup command failed for task #%d: %v", s.projectLogPrefix(t.ProjectID), t.ID, err)
		}
	}

	return nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// writeClaudeScript writes a bash wrapper script that runs claude and drops to a shell.
func writeClaudeScript(scriptPath string, yolo bool) error {
	claudeCmd := "claude"
	if yolo {
		claudeCmd = "claude --dangerously-skip-permissions"
	}
	script := fmt.Sprintf("#!/bin/bash\n%s\nexec bash\n", claudeCmd)
	return os.WriteFile(scriptPath, []byte(script), 0755)
}

// cleanupWorktreeAndBranch removes the worktree and branch for a task while holding the merge lock.
// It logs warnings on errors rather than returning them, since cleanup is best-effort.
func (s *Server) cleanupWorktreeAndBranch(pc *projectContext, t *task.Task) {
	pc.engine.AcquireMergeLock()
	defer pc.engine.ReleaseMergeLock()

	if t.WorktreePath != "" {
		if err := gitpkg.RemoveWorktree(pc.repoRoot, t.WorktreePath); err != nil {
			log.Printf("%sWarning: failed to remove worktree for task #%d: %v", s.projectLogPrefix(t.ProjectID), t.ID, err)
		}
		gitpkg.CleanupWorktrees(pc.repoRoot)
	}
	// Only delete branches that sortie created; preserve user-provided branches
	if t.Branch != "" && t.CheckoutBranch == "" {
		if err := gitpkg.ForceDeleteBranch(pc.repoRoot, t.Branch); err != nil {
			log.Printf("%sWarning: failed to delete branch for task #%d: %v", s.projectLogPrefix(t.ProjectID), t.ID, err)
		}
	}
	if t.WorktreePath != "" {
		if err := s.database.ClearWorktreePath(t.ID); err != nil {
			log.Printf("%sWarning: failed to clear worktree path for task #%d: %v", s.projectLogPrefix(t.ProjectID), t.ID, err)
		}
	}
}

func (s *Server) handleGetLogs(conn net.Conn, req GetLogsRequest) {
	t, err := s.database.GetTask(req.TaskID)
	if err != nil {
		s.sendMessage(conn, MsgGetLogs, GetLogsResponse{
			TaskID: req.TaskID,
			Lines:  []string{},
		})
		return
	}

	dataDir := s.getProjectDataDir(t)

	if req.Step != "" {
		logPath := workflow.ProjectLogPath(dataDir, req.TaskID, req.Step)
		lines := readLogFile(logPath)

		if req.Tail > 0 && len(lines) > req.Tail {
			lines = lines[len(lines)-req.Tail:]
		}

		s.sendMessage(conn, MsgGetLogs, GetLogsResponse{
			TaskID: req.TaskID,
			Step:   req.Step,
			Lines:  lines,
		})
		return
	}

	logsDir := workflow.ProjectLogsDir(dataDir, req.TaskID)
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		s.sendMessage(conn, MsgGetLogs, GetLogsResponse{
			TaskID: req.TaskID,
			Lines:  []string{},
		})
		return
	}

	var allLines []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".log" {
			continue
		}
		lines := readLogFile(filepath.Join(logsDir, entry.Name()))
		allLines = append(allLines, lines...)
	}

	if req.Tail > 0 && len(allLines) > req.Tail {
		allLines = allLines[len(allLines)-req.Tail:]
	}

	s.sendMessage(conn, MsgGetLogs, GetLogsResponse{
		TaskID: req.TaskID,
		Lines:  allLines,
	})
}

func readLogFile(path string) []string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}

func (s *Server) handleCreateTask(conn net.Conn, req CreateTaskRequest) {
	description := strings.TrimSpace(req.Description)
	if description == "" && req.CheckoutBranch == "" {
		s.sendError(conn, "description cannot be empty")
		return
	}

	projectPath := req.ProjectPath
	if projectPath == "" {
		s.sendError(conn, "project_path is required")
		return
	}

	proj, err := s.database.GetOrCreateProject(projectPath)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to resolve project: %v", err))
		return
	}

	// When using existing branch with empty description, generate title from branch name
	var title string
	if description == "" && req.CheckoutBranch != "" {
		title = "Branch " + req.CheckoutBranch
	} else {
		title = task.SanitizeTitle(description)
	}

	slug := task.Slugify(title)

	priority := task.PriorityMedium
	if req.Priority != "" && task.IsValidPriority(req.Priority) {
		priority = task.Priority(req.Priority)
	} else if proj.DefaultPriority != "" {
		priority = proj.DefaultPriority
	}

	if req.CheckoutBranch != "" && req.BranchName != "" {
		s.sendError(conn, "cannot specify both --checkout and --branch")
		return
	}

	worktree := proj.DefaultWorktree
	if req.Worktree != nil {
		worktree = *req.Worktree
	}

	// Persist worktree preference for this project
	if err := s.database.UpdateProjectDefaultWorktree(proj.ID, worktree); err != nil {
		log.Printf("%sFailed to update default worktree for project %d: %v", s.projectLogPrefix(proj.ID), proj.ID, err)
	}

	t, err := s.database.CreateTaskWithPriority(proj.ID, title, description, slug, req.Workflow, req.BranchName, "", req.TargetBranch, req.CheckoutBranch, task.StatusInit, priority, worktree, req.Images)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to create task: %v", err))
		return
	}

	// Set task dependencies if provided
	if len(req.BlockedBy) > 0 {
		if err := s.database.SetTaskDependencies(t.ID, req.BlockedBy); err != nil {
			log.Printf("%sFailed to set dependencies for task #%d: %v", s.projectLogPrefix(proj.ID), t.ID, err)
		} else {
			// Re-fetch task to include dependencies in response
			if updated, err := s.database.GetTask(t.ID); err == nil {
				t = updated
			}
		}
	}

	s.broadcastToSubscribers(MsgTaskUpdate, TaskUpdateResponse{Task: s.taskToInfo(t)})

	s.sendMessage(conn, MsgCreateTask, CreateTaskResponse{Task: s.taskToInfo(t)})

	go s.refineTaskTitle(t.ID, t.ProjectID, t.BranchName, t.Worktree, t.CheckoutBranch, description, title)
}

func (s *Server) refineTaskTitle(taskID, projectID int64, branchName string, worktree bool, checkoutBranch string, description string, initialTitle string) {
	projCfg := s.cfg
	if pc, err := s.getProjectContext(projectID); err == nil {
		projCfg = pc.cfg
	}

	var title string

	// Skip AI title generation when description is empty (existing branch with no prompt)
	if description == "" {
		title = initialTitle
	} else {
		ctx, cancel := context.WithTimeout(s.ctx, titleGenerationTimeout)
		defer cancel()

		var err error
		title, err = s.generateTitle(ctx, description, &projCfg.Claude)
		if err != nil {
			log.Printf("%sFailed to generate AI title for task #%d: %v", s.projectLogPrefix(projectID), taskID, err)
			if err := s.database.UpdateTaskStatus(taskID, task.StatusPending); err != nil {
				log.Printf("%sFailed to transition task #%d to pending: %v", s.projectLogPrefix(projectID), taskID, err)
			}
			s.broadcastTaskUpdate(taskID)
			return
		}
	}

	slug := task.Slugify(title)

	// Skip branch resolution for no-worktree tasks
	var branch string
	if worktree {
		if checkoutBranch != "" {
			branch = checkoutBranch
		} else {
			branch = projCfg.ResolveBranchForTask(taskID, title, slug, branchName)
		}
	}

	if err := s.database.FinalizeTaskIdentity(taskID, title, slug, branch); err != nil {
		log.Printf("%sFailed to update title for task #%d: %v", s.projectLogPrefix(projectID), taskID, err)
		if err := s.database.UpdateTaskStatus(taskID, task.StatusPending); err != nil {
			log.Printf("%sFailed to transition task #%d to pending: %v", s.projectLogPrefix(projectID), taskID, err)
		}
		s.broadcastTaskUpdate(taskID)
		return
	}

	if err := s.database.UpdateTaskStatus(taskID, task.StatusPending); err != nil {
		log.Printf("%sFailed to transition task #%d to pending: %v", s.projectLogPrefix(projectID), taskID, err)
		return
	}

	s.broadcastTaskUpdate(taskID)
	log.Printf("%sAI title for task #%d: %s (branch: %s)", s.projectLogPrefix(projectID), taskID, title, branch)
}

func (s *Server) generateTitle(ctx context.Context, description string, claude *config.ClaudeConfig) (string, error) {
	prompt := fmt.Sprintf(
		"Generate a concise task title (one short sentence, max 80 characters, no quotes, no prefix like 'Title:') for the following task description:\n\n%s",
		description,
	)

	args := []string{"-p", prompt, "--output-format", "text", "--model", "haiku"}
	args = append(args, claude.DefaultArgs...)

	cmd := exec.CommandContext(ctx, claude.Command, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("claude command failed: %w (stderr: %s)", err, stderr.String())
	}

	title := task.SanitizeTitle(stdout.String())
	if title == "" {
		return "", fmt.Errorf("claude returned empty title")
	}

	return title, nil
}

func (s *Server) handleUpdatePriority(conn net.Conn, req UpdatePriorityRequest) {
	if !task.IsValidPriority(req.Priority) {
		s.sendError(conn, fmt.Sprintf("invalid priority: %s", req.Priority))
		return
	}

	if err := s.database.UpdateTaskPriority(req.TaskID, task.Priority(req.Priority)); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to update priority: %v", err))
		return
	}

	s.broadcastTaskUpdate(req.TaskID)
	s.sendMessage(conn, MsgOK, OKResponse{Message: "priority updated"})
}

func (s *Server) handleUpdateField(conn net.Conn, req UpdateFieldRequest) {
	var err error
	switch req.Field {
	case "title":
		err = s.database.UpdateTaskTitle(req.TaskID, req.Value)
	case "description":
		err = s.database.UpdateTaskDescription(req.TaskID, req.Value)
	case "context":
		err = s.database.UpdateTaskContext(req.TaskID, req.Value)
	default:
		s.sendError(conn, fmt.Sprintf("unknown field: %s", req.Field))
		return
	}
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to update %s: %v", req.Field, err))
		return
	}
	s.broadcastTaskUpdate(req.TaskID)
	s.sendMessage(conn, MsgOK, OKResponse{Message: fmt.Sprintf("%s updated", req.Field)})
}

func (s *Server) handleRevertTask(conn net.Conn, req RevertTaskRequest) {
	t, err := s.database.GetTask(req.TaskID)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to get task: %v", err))
		return
	}

	if !t.Status.IsTerminal() {
		s.sendError(conn, fmt.Sprintf("task must be completed or failed to revert (status: %s)", t.Status))
		return
	}

	commits, err := s.database.GetTaskCommits(req.TaskID)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to get task commits: %v", err))
		return
	}

	if len(commits) == 0 {
		s.sendError(conn, "no commits found for this task")
		return
	}

	pc, err := s.getProjectContext(t.ProjectID)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to get project context: %v", err))
		return
	}

	// Acquire merge mutex to prevent concurrent merge/revert operations
	pc.engine.AcquireMergeLock()
	defer pc.engine.ReleaseMergeLock()

	if err := gitpkg.RevertCommits(pc.repoRoot, commits); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to revert commits: %v", err))
		return
	}

	log.Printf("%sTask #%d reverted (%d commits)", s.projectLogPrefix(t.ProjectID), t.ID, len(commits))
	s.sendMessage(conn, MsgOK, OKResponse{Message: fmt.Sprintf("task #%d reverted (%d commits)", t.ID, len(commits))})
}

func (s *Server) handleUpdateDependency(conn net.Conn, req UpdateDependencyRequest) {
	// Validate both tasks exist
	if _, err := s.database.GetTask(req.TaskID); err != nil {
		s.sendError(conn, fmt.Sprintf("task #%d not found: %v", req.TaskID, err))
		return
	}
	if _, err := s.database.GetTask(req.BlockedBy); err != nil {
		s.sendError(conn, fmt.Sprintf("task #%d not found: %v", req.BlockedBy, err))
		return
	}

	switch req.Action {
	case "add":
		// Check for circular dependency
		circular, err := s.database.HasCircularDependency(req.TaskID, req.BlockedBy)
		if err != nil {
			s.sendError(conn, fmt.Sprintf("failed to check circular dependency: %v", err))
			return
		}
		if circular {
			s.sendError(conn, "adding this dependency would create a cycle")
			return
		}
		if err := s.database.AddTaskDependency(req.TaskID, req.BlockedBy); err != nil {
			s.sendError(conn, fmt.Sprintf("failed to add dependency: %v", err))
			return
		}
	case "remove":
		if err := s.database.RemoveTaskDependency(req.TaskID, req.BlockedBy); err != nil {
			s.sendError(conn, fmt.Sprintf("failed to remove dependency: %v", err))
			return
		}
	default:
		s.sendError(conn, fmt.Sprintf("invalid action: %s (must be 'add' or 'remove')", req.Action))
		return
	}

	s.broadcastTaskUpdate(req.TaskID)
	s.sendMessage(conn, MsgOK, OKResponse{Message: fmt.Sprintf("dependency updated for task #%d", req.TaskID)})
}

func (s *Server) handleDetachBranch(conn net.Conn, req DetachBranchRequest) {
	t, err := s.database.GetTask(req.TaskID)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to get task: %v", err))
		return
	}

	if !t.Worktree || t.WorktreePath == "" {
		s.sendError(conn, "task does not have a worktree")
		return
	}

	if t.Status == task.StatusRunning || t.Status == task.StatusInit || t.Status == task.StatusPending || t.Status == task.StatusFinalizing || t.Status == task.StatusSummarizing {
		s.sendError(conn, fmt.Sprintf("cannot detach while agent is active (status: %s)", t.Status))
		return
	}

	if t.WorktreeDetached {
		s.sendError(conn, "worktree branch is already detached")
		return
	}

	if err := gitpkg.DetachWorktreeHead(t.WorktreePath); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to detach worktree HEAD: %v", err))
		return
	}

	if err := s.database.SetWorktreeDetached(t.ID, true); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to update detached state: %v", err))
		return
	}

	s.broadcastTaskUpdate(t.ID)
	s.sendMessage(conn, MsgOK, OKResponse{Message: fmt.Sprintf("branch %s detached from worktree", t.Branch)})
}

func (s *Server) handleAttachBranch(conn net.Conn, req AttachBranchRequest) {
	t, err := s.database.GetTask(req.TaskID)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to get task: %v", err))
		return
	}

	if !t.Worktree || t.WorktreePath == "" {
		s.sendError(conn, "task does not have a worktree")
		return
	}

	if !t.WorktreeDetached {
		s.sendError(conn, "worktree branch is not detached")
		return
	}

	// Auto-checkout the default branch on root repo before reattaching,
	// in case root is currently on the task's branch (which would prevent reattach)
	if repoRoot := s.getProjectRepoRoot(t); repoRoot != "" {
		defaultBranch := gitpkg.GetDefaultBranch(repoRoot)
		if err := gitpkg.CheckoutBranch(repoRoot, defaultBranch); err != nil {
			log.Printf("%sWarning: failed to checkout %s on root before reattach: %v", s.projectLogPrefix(t.ProjectID), defaultBranch, err)
		}
	}

	if err := gitpkg.ReattachWorktreeBranch(t.WorktreePath, t.Branch); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to reattach branch: %v", err))
		return
	}

	if err := s.database.SetWorktreeDetached(t.ID, false); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to update detached state: %v", err))
		return
	}

	s.broadcastTaskUpdate(t.ID)
	s.sendMessage(conn, MsgOK, OKResponse{Message: fmt.Sprintf("branch %s reattached to worktree", t.Branch)})
}

func (s *Server) handleDeleteTask(conn net.Conn, req DeleteTaskRequest) {
	t, err := s.database.GetTask(req.TaskID)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to get task: %v", err))
		return
	}

	agentID := fmt.Sprintf("%d", t.ID)
	_ = s.manager.StopAgent(agentID)

	if pc, err := s.getProjectContext(t.ProjectID); err == nil {
		if err := tmux.KillSessionsForTask(pc.cfg.Project.Name, agentID); err != nil {
			log.Printf("%sWarning: failed to kill tmux sessions for task #%d: %v", s.projectLogPrefix(t.ProjectID), t.ID, err)
		}
	}

	repoRoot := s.getProjectRepoRoot(t)

	if t.Worktree && repoRoot != "" {
		if pc, err := s.getProjectContext(t.ProjectID); err == nil {
			s.cleanupWorktreeAndBranch(pc, t)
		}
	}

	dataDir := s.getProjectDataDir(t)
	logDir := workflow.ProjectLogsDir(dataDir, t.ID)
	if err := os.RemoveAll(logDir); err != nil {
		log.Printf("%sWarning: failed to remove log dir for task #%d: %v", s.projectLogPrefix(t.ProjectID), t.ID, err)
	}

	if err := s.database.DeleteTask(t.ID); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to delete task: %v", err))
		return
	}

	s.sendMessage(conn, MsgOK, OKResponse{Message: fmt.Sprintf("task #%d deleted", t.ID)})
}
