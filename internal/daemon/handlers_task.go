package daemon

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/aface/sortie/internal/config"
	gitpkg "github.com/aface/sortie/internal/git"
	"github.com/aface/sortie/internal/task"
	"github.com/aface/sortie/internal/tmux"
	"github.com/aface/sortie/internal/workflow"
)

const (
	// titleGenerationTimeout is the maximum time allowed for AI-based task title generation.
	titleGenerationTimeout = 30 * time.Second
)

// noiseFiles are files that don't count as meaningful changes when checking
// whether a task produced real output (e.g. when fast-tracking to completed).
var noiseFiles = []string{".claude-output.log", "CLAUDE.md"}

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

func (s *Server) handleGetTask(conn net.Conn, req GetTaskRequest) {
	t, err := s.database.GetTask(req.TaskID)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to get task: %v", err))
		return
	}

	info := s.taskToInfo(t)
	s.sendMessage(conn, MsgGetTask, GetTaskResponse{Task: info})
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
		title = "⎇ " + req.CheckoutBranch
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

	if req.TmuxDirect {
		go s.setupTmuxDirect(t.ID, t.ProjectID, title)
	} else {
		go s.refineTaskTitle(t.ID, t.ProjectID, t.BranchName, t.Worktree, t.CheckoutBranch, description, title, req.Title)
	}
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

func (s *Server) handleGetStepContexts(conn net.Conn, req GetStepContextsRequest) {
	steps, err := s.database.GetAllTaskStepContexts(req.TaskID)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to get step contexts: %v", err))
		return
	}
	s.sendMessage(conn, MsgGetStepContexts, GetStepContextsResponse{Steps: steps})
}

func (s *Server) refineTaskTitle(taskID, projectID int64, branchName string, worktree bool, checkoutBranch string, description string, initialTitle string, manualTitle string) {
	projCfg := s.cfg
	if pc, err := s.getProjectContext(projectID); err == nil {
		projCfg = pc.cfg
	}

	var title string

	// Use manual title if provided, skipping AI generation
	if manualTitle != "" {
		title = manualTitle
	} else if description == "" {
		// Skip AI title generation when description is empty (existing branch with no prompt)
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
