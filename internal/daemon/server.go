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
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"path/filepath"

	"github.com/aface/sortie/internal/agent"
	"github.com/aface/sortie/internal/config"
	"github.com/aface/sortie/internal/db"
	gitpkg "github.com/aface/sortie/internal/git"
	"github.com/aface/sortie/internal/notify"
	"github.com/aface/sortie/internal/task"
	"github.com/aface/sortie/internal/tmux"
	"github.com/aface/sortie/internal/workflow"
)

// projectContext holds per-project state: config, engine, and repoRoot.
type projectContext struct {
	cfg      *config.Config
	engine   *workflow.Engine
	repoRoot string
}

type Server struct {
	cfg      *config.Config
	listener net.Listener
	manager  *agent.Manager
	database *db.DB
	notifier *notify.Notifier

	// Per-project engines, keyed by project ID
	projectsMu sync.RWMutex
	projects   map[int64]*projectContext

	mu          sync.RWMutex
	clients     map[net.Conn]bool
	subscribers map[net.Conn]bool

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewServer(cfg *config.Config, database *db.DB) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	notifier := notify.New(&cfg.Notifications)
	return &Server{
		cfg:         cfg,
		database:    database,
		manager:     agent.NewManager(cfg.Agents.MaxConcurrent, cfg.Agents.OutputBufferLines),
		notifier:    notifier,
		projects:    make(map[int64]*projectContext),
		clients:     make(map[net.Conn]bool),
		subscribers: make(map[net.Conn]bool),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// getProjectContext returns or creates the project context for a given project ID.
func (s *Server) getProjectContext(projectID int64) (*projectContext, error) {
	s.projectsMu.RLock()
	if pc, ok := s.projects[projectID]; ok {
		s.projectsMu.RUnlock()
		return pc, nil
	}
	s.projectsMu.RUnlock()

	// Load project from DB
	proj, err := s.database.GetProject(projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	// Load project-specific config
	projCfg, err := config.LoadForProject(proj.Path)
	if err != nil {
		log.Printf("Warning: failed to load config for project %s, using defaults: %v", proj.Path, err)
		projCfg = s.cfg
	}

	engine := workflow.NewEngine(projCfg, s.database, s.notifier, proj.Path)

	pc := &projectContext{
		cfg:      projCfg,
		engine:   engine,
		repoRoot: proj.Path,
	}

	s.projectsMu.Lock()
	s.projects[projectID] = pc
	s.projectsMu.Unlock()

	return pc, nil
}

// getProjectDataDir returns the .sortie data directory for a task's project.
func (s *Server) getProjectDataDir(t *task.Task) string {
	pc, err := s.getProjectContext(t.ProjectID)
	if err != nil {
		// Fallback: use global data dir
		return config.GetGlobalDataDir()
	}
	return filepath.Join(pc.repoRoot, ".sortie")
}

// getProjectRepoRoot returns the repo root for a task's project.
func (s *Server) getProjectRepoRoot(t *task.Task) string {
	pc, err := s.getProjectContext(t.ProjectID)
	if err != nil {
		return ""
	}
	return pc.repoRoot
}

func (s *Server) Start(foreground bool) error {
	if err := s.cfg.EnsureDirs(); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	if err := s.writePidFile(); err != nil {
		return fmt.Errorf("failed to write pid file: %w", err)
	}

	if err := os.RemoveAll(s.cfg.Daemon.SocketPath); err != nil {
		return fmt.Errorf("failed to remove old socket: %w", err)
	}

	listener, err := net.Listen("unix", s.cfg.Daemon.SocketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on socket: %w", err)
	}
	s.listener = listener

	s.manager.SetStateChangeCallback(s.onAgentStateChange)

	if err := s.manager.RecoverSessions(); err != nil {
		log.Printf("Warning: failed to recover sessions: %v", err)
	}

	if err := s.recoverOrphanedTasks(); err != nil {
		log.Printf("Warning: failed to recover orphaned tasks: %v", err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Received shutdown signal")
		s.Shutdown()
	}()

	s.wg.Add(1)
	go s.acceptLoop()

	s.wg.Add(1)
	go s.taskPollerLoop()

	log.Printf("Daemon started, listening on %s", s.cfg.Daemon.SocketPath)

	s.wg.Wait()

	return nil
}

func (s *Server) acceptLoop() {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return
			default:
				log.Printf("Accept error: %v", err)
				continue
			}
		}

		s.mu.Lock()
		s.clients[conn] = true
		s.mu.Unlock()

		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer func() {
		s.mu.Lock()
		delete(s.clients, conn)
		delete(s.subscribers, conn)
		s.mu.Unlock()
		conn.Close()
	}()

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB buffer for large IPC messages
	for scanner.Scan() {
		msg, err := DecodeMessage(scanner.Bytes())
		if err != nil {
			s.sendError(conn, "invalid message format")
			continue
		}

		s.handleMessage(conn, msg)
	}
}

func (s *Server) handleMessage(conn net.Conn, msg *Message) {
	switch msg.Type {
	case MsgPing:
		s.sendMessage(conn, MsgPong, nil)

	case MsgListAgents:
		s.handleListAgents(conn)

	case MsgListTasks:
		var req ListTasksRequest
		msg.DecodePayload(&req) // gracefully handles nil payload (treats as zero value = all projects)
		s.handleListTasks(conn, req)

	case MsgStartAgent:
		var req StartAgentRequest
		if err := msg.DecodePayload(&req); err != nil {
			s.sendError(conn, "invalid payload")
			return
		}
		s.handleStartAgent(conn, req)

	case MsgStopAgent:
		var req StopAgentRequest
		if err := msg.DecodePayload(&req); err != nil {
			s.sendError(conn, "invalid payload")
			return
		}
		s.handleStopAgent(conn, req)

	case MsgSubscribe:
		s.handleSubscribe(conn)

	case MsgUnsubscribe:
		s.handleUnsubscribe(conn)

	case MsgGetOutput:
		var req GetOutputRequest
		if err := msg.DecodePayload(&req); err != nil {
			s.sendError(conn, "invalid payload")
			return
		}
		s.handleGetOutput(conn, req)

	case MsgSendInput:
		var req SendInputRequest
		if err := msg.DecodePayload(&req); err != nil {
			s.sendError(conn, "invalid payload")
			return
		}
		s.handleSendInput(conn, req)

	case MsgGetTask:
		var req GetTaskRequest
		if err := msg.DecodePayload(&req); err != nil {
			s.sendError(conn, "invalid payload")
			return
		}
		s.handleGetTask(conn, req)

	case MsgApproveTask:
		var req ApproveTaskRequest
		if err := msg.DecodePayload(&req); err != nil {
			s.sendError(conn, "invalid payload")
			return
		}
		s.handleApproveTask(conn, req)

	case MsgRejectTask:
		var req RejectTaskRequest
		if err := msg.DecodePayload(&req); err != nil {
			s.sendError(conn, "invalid payload")
			return
		}
		s.handleRejectTask(conn, req)

	case MsgRetryTask:
		var req RetryTaskRequest
		if err := msg.DecodePayload(&req); err != nil {
			s.sendError(conn, "invalid payload")
			return
		}
		s.handleRetryTask(conn, req)

	case MsgGetLogs:
		var req GetLogsRequest
		if err := msg.DecodePayload(&req); err != nil {
			s.sendError(conn, "invalid payload")
			return
		}
		s.handleGetLogs(conn, req)

	case MsgCreateTask:
		var req CreateTaskRequest
		if err := msg.DecodePayload(&req); err != nil {
			s.sendError(conn, "invalid payload")
			return
		}
		s.handleCreateTask(conn, req)

	case MsgContinueTask:
		var req ContinueTaskRequest
		if err := msg.DecodePayload(&req); err != nil {
			s.sendError(conn, "invalid payload")
			return
		}
		s.handleContinueTask(conn, req)

	case MsgDeleteTask:
		var req DeleteTaskRequest
		if err := msg.DecodePayload(&req); err != nil {
			s.sendError(conn, "invalid payload")
			return
		}
		s.handleDeleteTask(conn, req)

	case MsgUpdatePriority:
		var req UpdatePriorityRequest
		if err := msg.DecodePayload(&req); err != nil {
			s.sendError(conn, "invalid payload")
			return
		}
		s.handleUpdatePriority(conn, req)

	case MsgShutdown:
		s.Shutdown()

	default:
		s.sendError(conn, "unknown message type")
	}
}

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
		// Agent not in memory — try reading logs from the task's project data dir
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

func (s *Server) handleApproveTask(conn net.Conn, req ApproveTaskRequest) {
	t, err := s.database.GetTask(req.TaskID)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to get task: %v", err))
		return
	}

	if t.Status != task.StatusAwaitingApproval && t.Status != task.StatusTmux {
		s.sendError(conn, fmt.Sprintf("task is not awaiting approval (status: %s)", t.Status))
		return
	}

	// Kill any tmux sessions from the approved step before resuming
	agentID := fmt.Sprintf("%d", t.ID)
	if err := tmux.KillSessionsForTask(agentID); err != nil {
		log.Printf("Warning: failed to kill tmux sessions for task #%d: %v", t.ID, err)
	}

	// Save original status for rollback
	origStatus := t.Status

	// Set status to running
	if err := s.database.UpdateTaskStatus(t.ID, task.StatusRunning); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to update task status: %v", err))
		return
	}

	// Resume the task (engine will continue from t.StepIndex)
	if err := s.startTaskAgent(t); err != nil {
		// Roll back status so the task isn't stuck as "running" with no agent
		_ = s.database.UpdateTaskStatus(t.ID, origStatus)
		s.sendError(conn, fmt.Sprintf("failed to start agent: %v", err))
		return
	}

	s.sendMessage(conn, MsgOK, OKResponse{Message: "task approved and resumed"})
}

func (s *Server) handleRejectTask(conn net.Conn, req RejectTaskRequest) {
	t, err := s.database.GetTask(req.TaskID)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to get task: %v", err))
		return
	}

	if t.Status != task.StatusAwaitingApproval && t.Status != task.StatusTmux {
		s.sendError(conn, fmt.Sprintf("task is not awaiting approval (status: %s)", t.Status))
		return
	}

	// Kill any tmux sessions from the rejected step
	agentID := fmt.Sprintf("%d", t.ID)
	if err := tmux.KillSessionsForTask(agentID); err != nil {
		log.Printf("Warning: failed to kill tmux sessions for task #%d: %v", t.ID, err)
	}

	repoRoot := s.getProjectRepoRoot(t)

	// Remove worktree if it exists
	if t.WorktreePath != "" && repoRoot != "" {
		if err := gitpkg.RemoveWorktree(repoRoot, t.WorktreePath); err != nil {
			log.Printf("Warning: failed to remove worktree for task #%d: %v", t.ID, err)
		}
	}

	// Delete branch if it exists
	if t.Branch != "" && repoRoot != "" {
		if err := gitpkg.ForceDeleteBranch(repoRoot, t.Branch); err != nil {
			log.Printf("Warning: failed to delete branch for task #%d: %v", t.ID, err)
		}
	}

	// Set status to failed
	if err := s.database.UpdateTaskStatus(t.ID, task.StatusFailed); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to update task status: %v", err))
		return
	}

	s.sendMessage(conn, MsgOK, OKResponse{Message: "task rejected"})
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

	if !t.Status.IsTerminal() {
		s.sendError(conn, fmt.Sprintf("task is not in a terminal state (status: %s)", t.Status))
		return
	}

	// Failed tasks: retry from the last failed step
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

	// Completed tasks: open tmux session with context loaded (not auto-sent)
	if !tmux.IsAvailable() {
		s.sendError(conn, "tmux is not installed or not in PATH")
		return
	}

	pc, err := s.getProjectContext(t.ProjectID)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to get project context: %v", err))
		return
	}

	// Ensure worktree exists (may have been cleaned up)
	if t.WorktreePath == "" || !dirExists(t.WorktreePath) {
		if t.Branch == "" {
			t.Branch = pc.cfg.ResolveBranchName(t.ID, t.Slug)
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

	// Write context into CLAUDE.md so claude loads it without auto-sending
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

	// Create tmux session with just `claude` (no prompt arg — waits for user input)
	taskID := fmt.Sprintf("%d", t.ID)
	session := tmux.NewStepSession(taskID, "continue", t.WorktreePath)

	// Kill stale session if exists
	if session.Exists() {
		session.Kill()
	}

	// Write wrapper script that runs claude without a prompt argument
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

	// Update task status to tmux
	if err := s.database.UpdateTaskStatus(t.ID, task.StatusTmux); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to update task status: %v", err))
		return
	}

	s.broadcastTaskUpdate(t.ID)

	log.Printf("Continue session started for task #%d (tmux: %s)", t.ID, session.Name)
	s.sendMessage(conn, MsgOK, OKResponse{Message: fmt.Sprintf("continue session started for task #%d", t.ID)})
}

// dirExists returns true if the path exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func (s *Server) handleGetLogs(conn net.Conn, req GetLogsRequest) {
	// Resolve the data dir from the task's project
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
		// Read a specific step's log
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

	// No step specified — read all .log files in the task's log dir
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

// readLogFile reads all lines from a log file, returning nil if the file doesn't exist.
func readLogFile(path string) []string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024) // 1MB buffer for large NDJSON lines
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

	// Resolve project from path
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

	// Generate title from first line, truncated to 80 chars
	title := description
	if idx := strings.IndexByte(title, '\n'); idx != -1 {
		title = title[:idx]
	}
	title = strings.TrimSpace(title)
	if len(title) > 80 {
		title = title[:80]
	}

	slug := task.Slugify(title)

	// Resolve priority: request > project default > medium
	priority := task.PriorityMedium
	if req.Priority != "" && task.IsValidPriority(req.Priority) {
		priority = task.Priority(req.Priority)
	} else if proj.DefaultPriority != "" {
		priority = proj.DefaultPriority
	}

	t, err := s.database.CreateTaskWithPriority(proj.ID, title, description, slug, req.Workflow, "", task.StatusInit, priority, req.Images)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to create task: %v", err))
		return
	}

	// Broadcast to subscribers so TUI updates immediately
	s.broadcastToSubscribers(MsgTaskUpdate, TaskUpdateResponse{Task: s.taskToInfo(t)})

	s.sendMessage(conn, MsgCreateTask, CreateTaskResponse{Task: s.taskToInfo(t)})

	// Fire-and-forget goroutine for AI title generation
	go s.refineTaskTitle(t.ID, t.ProjectID, description)
}

// refineTaskTitle generates an AI title for a task in the background and updates the DB.
func (s *Server) refineTaskTitle(taskID, projectID int64, description string) {
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	title, err := s.generateTitle(ctx, description)
	if err != nil {
		log.Printf("Failed to generate AI title for task #%d: %v", taskID, err)
		// Transition to pending even on failure so the task can still be claimed
		if err := s.database.UpdateTaskStatus(taskID, task.StatusPending); err != nil {
			log.Printf("Failed to transition task #%d to pending: %v", taskID, err)
		}
		s.broadcastTaskUpdate(taskID)
		return
	}

	// Use per-project config for branch name resolution
	branchCfg := s.cfg
	if pc, err := s.getProjectContext(projectID); err == nil {
		branchCfg = pc.cfg
	}

	slug := task.Slugify(title)
	branch := branchCfg.ResolveBranchName(taskID, slug)

	if err := s.database.FinalizeTaskIdentity(taskID, title, slug, branch); err != nil {
		log.Printf("Failed to update title for task #%d: %v", taskID, err)
		// Still transition to pending
		if err := s.database.UpdateTaskStatus(taskID, task.StatusPending); err != nil {
			log.Printf("Failed to transition task #%d to pending: %v", taskID, err)
		}
		s.broadcastTaskUpdate(taskID)
		return
	}

	// Transition to pending now that title is finalized
	if err := s.database.UpdateTaskStatus(taskID, task.StatusPending); err != nil {
		log.Printf("Failed to transition task #%d to pending: %v", taskID, err)
		return
	}

	s.broadcastTaskUpdate(taskID)
	log.Printf("AI title for task #%d: %s", taskID, title)
}

func (s *Server) broadcastTaskUpdate(taskID int64) {
	t, err := s.database.GetTask(taskID)
	if err != nil {
		log.Printf("Failed to re-fetch task #%d for broadcast: %v", taskID, err)
		return
	}
	s.broadcastToSubscribers(MsgTaskUpdate, TaskUpdateResponse{Task: s.taskToInfo(t)})
}

// generateTitle invokes Claude CLI to produce a concise title from a task description.
func (s *Server) generateTitle(ctx context.Context, description string) (string, error) {
	prompt := fmt.Sprintf(
		"Generate a concise task title (one short sentence, max 80 characters, no quotes, no prefix like 'Title:') for the following task description:\n\n%s",
		description,
	)

	args := []string{"-p", prompt, "--output-format", "text"}
	args = append(args, s.cfg.Claude.DefaultArgs...)

	cmd := exec.CommandContext(ctx, s.cfg.Claude.Command, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("claude command failed: %w (stderr: %s)", err, stderr.String())
	}

	title := strings.TrimSpace(stdout.String())
	if title == "" {
		return "", fmt.Errorf("claude returned empty title")
	}

	if len(title) > 80 {
		title = title[:80]
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

func (s *Server) handleDeleteTask(conn net.Conn, req DeleteTaskRequest) {
	t, err := s.database.GetTask(req.TaskID)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to get task: %v", err))
		return
	}

	// Stop agent if running
	agentID := fmt.Sprintf("%d", t.ID)
	_ = s.manager.StopAgent(agentID)

	// Kill any tmux sessions for this task
	if err := tmux.KillSessionsForTask(agentID); err != nil {
		log.Printf("Warning: failed to kill tmux sessions for task #%d: %v", t.ID, err)
	}

	repoRoot := s.getProjectRepoRoot(t)

	// Remove worktree if it exists
	if t.WorktreePath != "" && repoRoot != "" {
		if err := gitpkg.RemoveWorktree(repoRoot, t.WorktreePath); err != nil {
			log.Printf("Warning: failed to remove worktree for task #%d: %v", t.ID, err)
		}
	}

	// Force-delete branch if it exists
	if t.Branch != "" && repoRoot != "" {
		if err := gitpkg.ForceDeleteBranch(repoRoot, t.Branch); err != nil {
			log.Printf("Warning: failed to delete branch for task #%d: %v", t.ID, err)
		}
	}

	// Remove log directory
	dataDir := s.getProjectDataDir(t)
	logDir := workflow.ProjectLogsDir(dataDir, t.ID)
	if err := os.RemoveAll(logDir); err != nil {
		log.Printf("Warning: failed to remove log dir for task #%d: %v", t.ID, err)
	}

	// Delete from database
	if err := s.database.DeleteTask(t.ID); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to delete task: %v", err))
		return
	}

	s.sendMessage(conn, MsgOK, OKResponse{Message: fmt.Sprintf("task #%d deleted", t.ID)})
}

func (s *Server) onAgentStateChange(a *agent.Agent, oldState, newState agent.State) {
	info := agentToInfo(a)
	s.broadcastToSubscribers(MsgAgentUpdate, AgentUpdateResponse{Agent: info})

	taskTitle := a.Task.Title
	if taskTitle == "" {
		taskTitle = a.Task.Description
	}

	// Update database task status based on agent state.
	// Worktrees are NOT auto-cleaned; they persist until explicit cleanup.
	switch newState {
	case agent.StateCompleted:
		// Check if the task is awaiting approval or in tmux status (engine set this in the DB).
		// If so, don't mark it completed — it's paused for approval.
		refreshedTask, err := s.database.GetTask(a.Task.ID)
		if err == nil && (refreshedTask.Status == task.StatusAwaitingApproval || refreshedTask.Status == task.StatusTmux) {
			log.Printf("Agent %s paused task #%d for approval", a.ID, a.Task.ID)
			s.notifier.AgentWaitingForInput(a.ID, taskTitle)
			return
		}

		log.Printf("Agent %s completed task #%d", a.ID, a.Task.ID)
		if err := s.database.UpdateTaskStatus(a.Task.ID, task.StatusCompleted); err != nil {
			log.Printf("Failed to update task status: %v", err)
		}
		// Kill tmux sessions for completed task
		if err := tmux.KillSessionsForTask(a.ID); err != nil {
			log.Printf("Warning: failed to kill tmux sessions for task %s: %v", a.ID, err)
		}
		s.notifier.AgentCompleted(a.ID, taskTitle)

		// Check if all tasks in the same project are done
		s.checkProjectTasksDone(a.Task.ProjectID)

	case agent.StateFailed:
		log.Printf("Agent %s failed task #%d: %s", a.ID, a.Task.ID, a.Error)
		if err := s.database.UpdateTaskError(a.Task.ID, a.Error); err != nil {
			log.Printf("Failed to update task error: %v", err)
		}
		// Kill tmux sessions for failed task
		if err := tmux.KillSessionsForTask(a.ID); err != nil {
			log.Printf("Warning: failed to kill tmux sessions for task %s: %v", a.ID, err)
		}
		s.notifier.AgentFailed(a.ID, taskTitle, a.Error)

		// Check if all tasks in the same project are done
		s.checkProjectTasksDone(a.Task.ProjectID)

	case agent.StateWaitingForInput:
		s.notifier.AgentWaitingForInput(a.ID, taskTitle)
	}
}

// checkProjectTasksDone checks if all tasks in a project are in a terminal state.
func (s *Server) checkProjectTasksDone(projectID int64) {
	tasks, err := s.database.GetTasksByProject(projectID)
	if err != nil || len(tasks) == 0 {
		return
	}
	for _, t := range tasks {
		switch t.Status {
		case task.StatusPending, task.StatusRunning, task.StatusAwaitingApproval, task.StatusTmux, task.StatusSummarizing, task.StatusInit:
			return // Still active work
		}
	}
	// All tasks in this project are in a terminal state
	log.Printf("All tasks completed for project %d", projectID)
	s.notifier.AllTasksCompleted()
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
	// Atomically claim the task (pending -> running) to prevent double-starts
	if t.Status == task.StatusPending {
		claimed, err := s.database.ClaimTask(t.ID)
		if err != nil {
			return fmt.Errorf("failed to claim task: %w", err)
		}
		if !claimed {
			return agent.ErrTaskAlreadyTracked
		}
	}

	// Get per-project engine
	pc, err := s.getProjectContext(t.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to get project context: %w", err)
	}

	// The workflow engine handles worktree creation, so we just need a workdir placeholder.
	workDir := t.WorktreePath
	if workDir == "" {
		workDir = pc.repoRoot // Temporary; engine will create and set the real worktree path
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

// recoverOrphanedTasks resets any tasks stuck in "running" or "init" state to "pending".
// Tasks in "awaiting-approval" are left alone — they need explicit user action.
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

	// Also recover tasks stuck in generating-title (title generation goroutine lost on restart)
	allTasks, err := s.database.GetAllTasks()
	if err != nil {
		return nil // Non-critical, don't fail
	}
	for _, t := range allTasks {
		if t.Status == task.StatusInit {
			log.Printf("Recovering task #%d stuck in init, resetting to pending", t.ID)
			if err := s.database.UpdateTaskStatus(t.ID, task.StatusPending); err != nil {
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
			log.Printf("Task #%d is awaiting approval (use 'approve' or 'reject' command)", t.ID)
		}
		if t.Status == task.StatusTmux {
			log.Printf("Task #%d has tmux session running (use 'approve' or 'reject' command)", t.ID)
		}
	}

	return nil
}

func (s *Server) Shutdown() {
	log.Println("Shutting down daemon...")

	// Stop accepting new connections first
	if s.listener != nil {
		s.listener.Close()
	}

	// Mark running tasks as failed before stopping agents
	if s.database != nil {
		runningTasks, err := s.database.GetRunningTasks()
		if err == nil {
			for _, t := range runningTasks {
				log.Printf("Marking task #%d as failed (daemon shutdown)", t.ID)
				if err := s.database.UpdateTaskError(t.ID, "daemon shutdown"); err != nil {
					log.Printf("Failed to mark task #%d as failed: %v", t.ID, err)
				}
			}
		}
	}

	// Shutdown agents with grace period
	s.manager.Shutdown(30 * time.Second)

	// Kill all Sortie tmux sessions
	if sessions, err := tmux.ListSessions(tmux.SessionPrefix); err == nil {
		for _, sess := range sessions {
			sess.Kill()
		}
	}

	// Now cancel context to stop poller and accept loop
	s.cancel()

	s.mu.Lock()
	for conn := range s.clients {
		conn.Close()
	}
	s.mu.Unlock()

	if s.database != nil {
		s.database.Close()
	}

	os.Remove(s.cfg.Daemon.PidFile)
	os.Remove(s.cfg.Daemon.SocketPath)

	log.Println("Daemon stopped")
}

func (s *Server) writePidFile() error {
	return os.WriteFile(s.cfg.Daemon.PidFile, []byte(strconv.Itoa(os.Getpid())), 0644)
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

// taskToInfo converts a task.Task to a TaskInfo for IPC, populating project fields.
func (s *Server) taskToInfo(t *task.Task) TaskInfo {
	info := TaskInfo{
		ID:           t.ID,
		ProjectID:    t.ProjectID,
		Title:        t.Title,
		Description:  t.Description,
		Slug:         t.Slug,
		Workflow:     t.Workflow,
		Status:       string(t.Status),
		Priority:     string(t.Priority),
		StepIndex:    t.StepIndex,
		CurrentStep:  t.CurrentStep,
		Branch:       t.Branch,
		WorktreePath: t.WorktreePath,
		ErrorMessage: t.ErrorMessage,
		Context:      t.Context,
		Images:       t.Images,
		BlockedBy:    t.BlockedBy,
		CreatedAt:    t.CreatedAt,
		StartedAt:    t.StartedAt,
		CompletedAt:  t.CompletedAt,
	}

	// Populate project info
	if proj, err := s.database.GetProject(t.ProjectID); err == nil {
		info.ProjectName = proj.Name
		info.ProjectPath = proj.Path
	}

	return info
}

func Start(cfg *config.Config, foreground bool) error {
	dbPath := cfg.GetDatabasePath("")
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	server := NewServer(cfg, database)
	return server.Start(foreground)
}

func Stop(cfg *config.Config) error {
	conn, err := net.Dial("unix", cfg.Daemon.SocketPath)
	if err != nil {
		return fmt.Errorf("daemon not running or cannot connect: %w", err)
	}
	defer conn.Close()

	msg, _ := NewMessage(MsgShutdown, nil)
	data, _ := EncodeMessage(msg)
	conn.Write(data)

	return nil
}

func Status(cfg *config.Config) (bool, int, error) {
	data, err := os.ReadFile(cfg.Daemon.PidFile)
	if err != nil {
		if os.IsNotExist(err) {
			return false, 0, nil
		}
		return false, 0, err
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return false, 0, nil
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return false, 0, nil
	}

	err = process.Signal(syscall.Signal(0))
	if err != nil {
		os.Remove(cfg.Daemon.PidFile)
		return false, 0, nil
	}

	return true, pid, nil
}
