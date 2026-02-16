package daemon

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/aface/ralph-tamer-kit/internal/agent"
	"github.com/aface/ralph-tamer-kit/internal/config"
	"github.com/aface/ralph-tamer-kit/internal/db"
	gitpkg "github.com/aface/ralph-tamer-kit/internal/git"
	"github.com/aface/ralph-tamer-kit/internal/notify"
	"github.com/aface/ralph-tamer-kit/internal/task"
	"github.com/aface/ralph-tamer-kit/internal/workflow"
)

type Server struct {
	cfg      *config.Config
	listener net.Listener
	manager  *agent.Manager
	engine   *workflow.Engine
	database *db.DB
	notifier *notify.Notifier
	repoRoot string

	mu          sync.RWMutex
	clients     map[net.Conn]bool
	subscribers map[net.Conn]bool

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewServer(cfg *config.Config, database *db.DB, repoRoot string) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	notifier := notify.New(&cfg.Notifications)
	return &Server{
		cfg:         cfg,
		database:    database,
		repoRoot:    repoRoot,
		manager:     agent.NewManager(cfg.Agents.MaxConcurrent, cfg.Agents.OutputBufferLines),
		engine:      workflow.NewEngine(cfg, database, notifier, repoRoot),
		notifier:    notifier,
		clients:     make(map[net.Conn]bool),
		subscribers: make(map[net.Conn]bool),
		ctx:         ctx,
		cancel:      cancel,
	}
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
		s.handleListTasks(conn)

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

	case MsgDeleteTask:
		var req DeleteTaskRequest
		if err := msg.DecodePayload(&req); err != nil {
			s.sendError(conn, "invalid payload")
			return
		}
		s.handleDeleteTask(conn, req)

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

func (s *Server) handleListTasks(conn net.Conn) {
	tasks, err := s.database.GetAllTasks()
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to get tasks: %v", err))
		return
	}

	infos := make([]TaskInfo, len(tasks))
	for i, t := range tasks {
		infos[i] = taskToInfo(t)
	}

	s.sendMessage(conn, MsgTaskList, TaskListResponse{Tasks: infos})
}

func (s *Server) handleStartAgent(conn net.Conn, req StartAgentRequest) {
	t, err := s.database.GetTask(req.TaskID)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to get task: %v", err))
		return
	}

	if t.Status != task.StatusPending && t.Status != task.StatusAwaitingApproval {
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
		s.sendError(conn, fmt.Sprintf("failed to get output: %v", err))
		return
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

	info := taskToInfo(t)
	s.sendMessage(conn, MsgGetTask, GetTaskResponse{Task: info})
}

func (s *Server) handleApproveTask(conn net.Conn, req ApproveTaskRequest) {
	t, err := s.database.GetTask(req.TaskID)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to get task: %v", err))
		return
	}

	if t.Status != task.StatusAwaitingApproval {
		s.sendError(conn, fmt.Sprintf("task is not awaiting approval (status: %s)", t.Status))
		return
	}

	// Set status to running
	if err := s.database.UpdateTaskStatus(t.ID, task.StatusRunning); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to update task status: %v", err))
		return
	}

	// Resume the task (engine will continue from t.StepIndex)
	if err := s.startTaskAgent(t); err != nil {
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

	if t.Status != task.StatusAwaitingApproval {
		s.sendError(conn, fmt.Sprintf("task is not awaiting approval (status: %s)", t.Status))
		return
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

func (s *Server) handleGetLogs(conn net.Conn, req GetLogsRequest) {
	t, err := s.database.GetTask(req.TaskID)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to get task: %v", err))
		return
	}

	if t.WorktreePath == "" {
		s.sendError(conn, "task has no worktree")
		return
	}

	// Default to current step if not specified
	step := req.Step
	if step == "" {
		step = t.CurrentStep
	}

	// Get log file path
	logPath := workflow.LogPath(t.WorktreePath, step)

	// Read log file
	file, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			s.sendMessage(conn, MsgGetLogs, GetLogsResponse{
				TaskID: req.TaskID,
				Step:   step,
				Lines:  []string{},
			})
			return
		}
		s.sendError(conn, fmt.Sprintf("failed to open log file: %v", err))
		return
	}
	defer file.Close()

	// Read all lines
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to read log file: %v", err))
		return
	}

	// Apply tail if requested
	if req.Tail > 0 && len(lines) > req.Tail {
		lines = lines[len(lines)-req.Tail:]
	}

	s.sendMessage(conn, MsgGetLogs, GetLogsResponse{
		TaskID: req.TaskID,
		Step:   step,
		Lines:  lines,
	})
}

func (s *Server) handleCreateTask(conn net.Conn, req CreateTaskRequest) {
	description := strings.TrimSpace(req.Description)
	if description == "" {
		s.sendError(conn, "description cannot be empty")
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

	t, err := s.database.CreateTask(title, description, slug, req.Workflow, "")
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to create task: %v", err))
		return
	}

	// Broadcast to subscribers so TUI updates immediately
	s.broadcastToSubscribers(MsgTaskUpdate, TaskUpdateResponse{Task: taskToInfo(t)})

	s.sendMessage(conn, MsgCreateTask, CreateTaskResponse{Task: taskToInfo(t)})
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

	// Remove worktree if it exists
	if t.WorktreePath != "" {
		if err := gitpkg.RemoveWorktree(s.repoRoot, t.WorktreePath); err != nil {
			log.Printf("Warning: failed to remove worktree for task #%d: %v", t.ID, err)
		}
	}

	// Force-delete branch if it exists
	if t.Branch != "" {
		if err := gitpkg.ForceDeleteBranch(s.repoRoot, t.Branch); err != nil {
			log.Printf("Warning: failed to delete branch for task #%d: %v", t.ID, err)
		}
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
		// Check if the task is awaiting_approval (engine set this in the DB).
		// If so, don't mark it completed — it's paused for approval.
		refreshedTask, err := s.database.GetTask(a.Task.ID)
		if err == nil && refreshedTask.Status == task.StatusAwaitingApproval {
			log.Printf("Agent %s paused task #%d for approval", a.ID, a.Task.ID)
			s.notifier.AgentWaitingForInput(a.ID, taskTitle)
			return
		}

		log.Printf("Agent %s completed task #%d", a.ID, a.Task.ID)
		if err := s.database.UpdateTaskStatus(a.Task.ID, task.StatusCompleted); err != nil {
			log.Printf("Failed to update task status: %v", err)
		}
		s.notifier.AgentCompleted(a.ID, taskTitle)

		// Check if all tasks are now terminal
		s.checkAllTasksDone()

	case agent.StateFailed:
		log.Printf("Agent %s failed task #%d: %s", a.ID, a.Task.ID, a.Error)
		if err := s.database.UpdateTaskError(a.Task.ID, a.Error); err != nil {
			log.Printf("Failed to update task error: %v", err)
		}
		s.notifier.AgentFailed(a.ID, taskTitle, a.Error)

		// Check if all tasks are now terminal
		s.checkAllTasksDone()

	case agent.StateWaitingForInput:
		s.notifier.AgentWaitingForInput(a.ID, taskTitle)
	}
}

func (s *Server) checkAllTasksDone() {
	tasks, err := s.database.GetAllTasks()
	if err != nil || len(tasks) == 0 {
		return
	}
	for _, t := range tasks {
		switch t.Status {
		case task.StatusPending, task.StatusRunning, task.StatusAwaitingApproval:
			return // Still active work
		}
	}
	// All tasks are in a terminal state (completed, failed, or stopped)
	log.Println("All tasks completed")
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

	// The workflow engine handles worktree creation, so we just need a workdir placeholder.
	// Use the expected worktree path so the agent tracks it.
	workDir := t.WorktreePath
	if workDir == "" {
		workDir = s.repoRoot // Temporary; engine will create and set the real worktree path
	}

	engine := s.engine
	runner := func(ctx context.Context) error {
		return engine.RunTask(ctx, t)
	}

	if _, err := s.manager.StartAgent(t, workDir, runner); err != nil {
		return err
	}

	return nil
}


// recoverOrphanedTasks resets any tasks stuck in "running" state to "pending".
// Tasks in "awaiting_approval" are left alone — they need explicit user action.
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

	// Log awaiting_approval tasks so users know they exist
	allTasks, err := s.database.GetAllTasks()
	if err != nil {
		return nil // Non-critical, don't fail
	}
	for _, t := range allTasks {
		if t.Status == task.StatusAwaitingApproval {
			log.Printf("Task #%d is awaiting approval (use 'approve' or 'reject' command)", t.ID)
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

func taskToInfo(t *task.Task) TaskInfo {
	return TaskInfo{
		ID:           t.ID,
		Title:        t.Title,
		Description:  t.Description,
		Slug:         t.Slug,
		Workflow:     t.Workflow,
		Status:       string(t.Status),
		StepIndex:    t.StepIndex,
		CurrentStep:  t.CurrentStep,
		Branch:       t.Branch,
		WorktreePath: t.WorktreePath,
		ErrorMessage: t.ErrorMessage,
		Context:      t.Context,
		BlockedBy:    t.BlockedBy,
		CreatedAt:    t.CreatedAt,
		StartedAt:    t.StartedAt,
		CompletedAt:  t.CompletedAt,
	}
}

func Start(cfg *config.Config, foreground bool) error {
	repoRoot, err := gitpkg.GetRepoRoot(".")
	if err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}

	cfg.ApplyDetectedProject(repoRoot)

	dbPath := cfg.GetDatabasePath(repoRoot)
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	server := NewServer(cfg, database, repoRoot)
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
