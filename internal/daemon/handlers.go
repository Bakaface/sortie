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

func (s *Server) handleSendInput(conn net.Conn, req SendInputRequest) {
	if err := s.manager.SendInput(req.AgentID, req.Input); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to send input: %v", err))
		return
	}

	s.sendMessage(conn, MsgOK, OKResponse{Message: "input sent"})
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
	if err := s.database.ResetTaskForRetry(req.TaskID); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to reset task: %v", err))
		return
	}

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
		if err := tmux.KillSessionsForTask(agentID); err != nil {
			log.Printf("Warning: failed to kill tmux sessions for task #%d: %v", t.ID, err)
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
		log.Printf("Task #%d continued past artifact-missing at step %d", t.ID, t.StepIndex)
		s.sendMessage(conn, MsgOK, OKResponse{Message: fmt.Sprintf("task #%d continued past missing artifact", t.ID)})
		return
	}

	if !t.Status.IsTerminal() {
		s.sendError(conn, fmt.Sprintf("task is not in a continuable state (status: %s)", t.Status))
		return
	}

	if t.Status == task.StatusFailed {
		if err := s.database.ResetTaskForRetryFromStep(req.TaskID); err != nil {
			s.sendError(conn, fmt.Sprintf("failed to reset task: %v", err))
			return
		}
		s.broadcastTaskUpdate(t.ID)
		log.Printf("Task #%d retrying from step %d", t.ID, t.StepIndex)
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

	if t.WorktreePath == "" || !dirExists(t.WorktreePath) {
		if t.Branch == "" {
			if t.BranchName != "" {
				t.Branch = config.ResolveBranchTemplate(t.BranchName, t.ID, t.Title, t.Slug)
			} else {
				t.Branch = pc.cfg.ResolveBranchName(t.ID, t.Slug)
			}
		}
		worktree, err := gitpkg.CreateWorktree(pc.repoRoot, t.ID, pc.cfg.Git.BaseBranch, t.Branch)
		if err != nil {
			s.sendError(conn, fmt.Sprintf("failed to create worktree: %v", err))
			return
		}
		t.WorktreePath = worktree.Path
		if err := s.database.UpdateTaskWorktreePath(t.ID, worktree.Path); err != nil {
			log.Printf("Warning: failed to update worktree path: %v", err)
		}
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
	session := tmux.NewStepSession(taskID, "continue", t.WorktreePath)

	if session.Exists() {
		session.Kill()
	}

	sortieDir := filepath.Join(t.WorktreePath, ".sortie")
	if err := os.MkdirAll(sortieDir, 0755); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to create sortie dir: %v", err))
		return
	}
	scriptFile := filepath.Join(sortieDir, "run-continue.sh")

	claudeCmd := "claude"
	if pc.cfg.Claude.Yolo {
		claudeCmd = "claude --dangerously-skip-permissions"
	}
	script := fmt.Sprintf("#!/bin/bash\n%s\nexec bash\n", claudeCmd)
	if err := os.WriteFile(scriptFile, []byte(script), 0755); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to write wrapper script: %v", err))
		return
	}

	if err := session.Create("bash", scriptFile); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to create tmux session: %v", err))
		return
	}

	if err := s.database.UpdateTaskStatus(t.ID, task.StatusTmux); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to update task status: %v", err))
		return
	}

	s.broadcastTaskUpdate(t.ID)

	log.Printf("Continue session started for task #%d (tmux: %s)", t.ID, session.Name)
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

	// Kill tmux sessions
	agentID := fmt.Sprintf("%d", t.ID)
	if err := tmux.KillSessionsForTask(agentID); err != nil {
		log.Printf("Warning: failed to kill tmux sessions for task #%d: %v", t.ID, err)
	}

	pc, err := s.getProjectContext(t.ProjectID)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to get project context: %v", err))
		return
	}

	// Set finalizing status while we run summarizer + on_complete
	if err := s.database.UpdateTaskStatus(t.ID, task.StatusFinalizing); err != nil {
		log.Printf("Warning: failed to set finalizing status for task #%d: %v", t.ID, err)
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
		log.Printf("Warning: finalize failed for task #%d: %v", t.ID, err)
		// Don't fail the whole operation — still mark as completed.
		// Best-effort cleanup of worktree and branch so they don't linger.
		if t.WorktreePath != "" && repoRoot != "" {
			if rmErr := gitpkg.RemoveWorktree(repoRoot, t.WorktreePath); rmErr != nil {
				log.Printf("Warning: failed to remove worktree for task #%d: %v", t.ID, rmErr)
			}
			gitpkg.CleanupWorktrees(repoRoot)
			if err := s.database.ClearWorktreePath(t.ID); err != nil {
				log.Printf("Warning: failed to clear worktree path for task #%d: %v", t.ID, err)
			}
		}
		if t.Branch != "" && repoRoot != "" {
			if rmErr := gitpkg.ForceDeleteBranch(repoRoot, t.Branch); rmErr != nil {
				log.Printf("Warning: failed to delete branch for task #%d: %v", t.ID, rmErr)
			}
		}
	}

	// Mark task as completed
	if err := s.database.UpdateTaskStatus(t.ID, task.StatusCompleted); err != nil {
		log.Printf("Error: failed to mark task #%d as completed: %v", t.ID, err)
		return
	}

	s.broadcastTaskUpdate(t.ID)
	log.Printf("Task #%d finalized from tmux continue session", t.ID)
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
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
	if description == "" {
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

	title := task.SanitizeTitle(description)

	slug := task.Slugify(title)

	priority := task.PriorityMedium
	if req.Priority != "" && task.IsValidPriority(req.Priority) {
		priority = task.Priority(req.Priority)
	} else if proj.DefaultPriority != "" {
		priority = proj.DefaultPriority
	}

	t, err := s.database.CreateTaskWithPriority(proj.ID, title, description, slug, req.Workflow, req.BranchName, "", task.StatusInit, priority, req.Images)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to create task: %v", err))
		return
	}

	s.broadcastToSubscribers(MsgTaskUpdate, TaskUpdateResponse{Task: s.taskToInfo(t)})

	s.sendMessage(conn, MsgCreateTask, CreateTaskResponse{Task: s.taskToInfo(t)})

	go s.refineTaskTitle(t.ID, t.ProjectID, t.BranchName, description)
}

func (s *Server) refineTaskTitle(taskID, projectID int64, branchName, description string) {
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	projCfg := s.cfg
	if pc, err := s.getProjectContext(projectID); err == nil {
		projCfg = pc.cfg
	}

	title, err := s.generateTitle(ctx, description, &projCfg.Claude)
	if err != nil {
		log.Printf("Failed to generate AI title for task #%d: %v", taskID, err)
		if err := s.database.UpdateTaskStatus(taskID, task.StatusPending); err != nil {
			log.Printf("Failed to transition task #%d to pending: %v", taskID, err)
		}
		s.broadcastTaskUpdate(taskID)
		return
	}

	slug := task.Slugify(title)

	// Use per-task branch template if provided, otherwise fall back to config default
	var branch string
	if branchName != "" {
		branch = config.ResolveBranchTemplate(branchName, taskID, title, slug)
	} else {
		branch = projCfg.ResolveBranchName(taskID, slug)
	}

	if err := s.database.FinalizeTaskIdentity(taskID, title, slug, branch); err != nil {
		log.Printf("Failed to update title for task #%d: %v", taskID, err)
		if err := s.database.UpdateTaskStatus(taskID, task.StatusPending); err != nil {
			log.Printf("Failed to transition task #%d to pending: %v", taskID, err)
		}
		s.broadcastTaskUpdate(taskID)
		return
	}

	if err := s.database.UpdateTaskStatus(taskID, task.StatusPending); err != nil {
		log.Printf("Failed to transition task #%d to pending: %v", taskID, err)
		return
	}

	s.broadcastTaskUpdate(taskID)
	log.Printf("AI title for task #%d: %s (branch: %s)", taskID, title, branch)
}

func (s *Server) generateTitle(ctx context.Context, description string, claude *config.ClaudeConfig) (string, error) {
	prompt := fmt.Sprintf(
		"Generate a concise task title (one short sentence, max 80 characters, no quotes, no prefix like 'Title:') for the following task description:\n\n%s",
		description,
	)

	args := []string{"-p", prompt, "--output-format", "text"}
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

func (s *Server) handleDeleteTask(conn net.Conn, req DeleteTaskRequest) {
	t, err := s.database.GetTask(req.TaskID)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to get task: %v", err))
		return
	}

	agentID := fmt.Sprintf("%d", t.ID)
	_ = s.manager.StopAgent(agentID)

	if err := tmux.KillSessionsForTask(agentID); err != nil {
		log.Printf("Warning: failed to kill tmux sessions for task #%d: %v", t.ID, err)
	}

	repoRoot := s.getProjectRepoRoot(t)

	if t.WorktreePath != "" && repoRoot != "" {
		if err := gitpkg.RemoveWorktree(repoRoot, t.WorktreePath); err != nil {
			log.Printf("Warning: failed to remove worktree for task #%d: %v", t.ID, err)
		}
	}

	if t.Branch != "" && repoRoot != "" {
		if err := gitpkg.ForceDeleteBranch(repoRoot, t.Branch); err != nil {
			log.Printf("Warning: failed to delete branch for task #%d: %v", t.ID, err)
		}
	}

	dataDir := s.getProjectDataDir(t)
	logDir := workflow.ProjectLogsDir(dataDir, t.ID)
	if err := os.RemoveAll(logDir); err != nil {
		log.Printf("Warning: failed to remove log dir for task #%d: %v", t.ID, err)
	}

	if err := s.database.DeleteTask(t.ID); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to delete task: %v", err))
		return
	}

	s.sendMessage(conn, MsgOK, OKResponse{Message: fmt.Sprintf("task #%d deleted", t.ID)})
}
