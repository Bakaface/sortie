package daemon

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"

	"path/filepath"

	"github.com/aface/sortie/internal/agent"
	"github.com/aface/sortie/internal/config"
	"github.com/aface/sortie/internal/db"
	gitpkg "github.com/aface/sortie/internal/git"
	"github.com/aface/sortie/internal/merge"
	"github.com/aface/sortie/internal/notify"
	"github.com/aface/sortie/internal/task"
	"github.com/aface/sortie/internal/tmux"
	"github.com/aface/sortie/internal/workflow"
)

const (
	// agentShutdownGracePeriod is the time allowed for agents to finish before force-stopping on shutdown.
	agentShutdownGracePeriod = 30 * time.Second

	// scannerBufferSize is the max message size accepted on the Unix socket (10MB).
	scannerBufferSize = 10 * 1024 * 1024
)

type projectContext struct {
	cfg           *config.Config
	engine        *workflow.Engine
	repoRoot      string
	configModTime time.Time // zero = no .sortie.yml at load time
}

type Server struct {
	cfg      *config.Config
	listener net.Listener
	manager  *agent.Manager
	database *db.DB
	notifier *notify.Notifier

	projectsMu sync.RWMutex
	projects   map[int64]*projectContext

	// mergeLocks hands out per-repo merge serializers. Owned here so the lock
	// survives engine reconstruction (the merge invariant is per-repo, not
	// per-engine).
	mergeLocks *merge.Locks

	mu           sync.RWMutex
	clients      map[net.Conn]bool
	subscribers  map[net.Conn]bool
	tmuxActivity map[int64]string

	// tmuxAutoState tracks per-task auto-advance bookkeeping for tasks
	// in StatusTmux. Lives behind mu. Entries are populated as the tmux
	// monitor observes activity / sentinels and cleared when the task
	// leaves StatusTmux (the daemon kills the session, or the user
	// finalizes the task manually).
	tmuxAutoState map[int64]*tmuxAutoEntry

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	shutdownOnce sync.Once
}

// tmuxAutoEntry holds the daemon's per-task state for tmux auto-advance.
//
// firstIdleAt is the timestamp at which the tmux pane was first observed
// in the ActivityIdle state since the most recent WIP transition. It is
// cleared on every transition back to WIP. The fallback path advances
// the workflow when (now - firstIdleAt) exceeds tmuxIdleFallbackDuration
// and no Stop-hook sentinel has shown up.
//
// advancing is set true the moment the daemon decides to advance the
// task, so re-entrant ticks of the monitor loop don't fire StartAgent
// twice while ResumeAfterApproval is still spinning up. Cleared when
// the task leaves StatusTmux.
type tmuxAutoEntry struct {
	firstIdleAt time.Time
	advancing   bool
}

func NewServer(cfg *config.Config, database *db.DB) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	notifier := notify.New(&cfg.Notifications)
	return &Server{
		cfg:           cfg,
		database:      database,
		manager:       agent.NewManager(cfg.Agents.MaxConcurrent, cfg.Agents.OutputBufferLines),
		notifier:      notifier,
		projects:      make(map[int64]*projectContext),
		mergeLocks:    merge.NewLocks(),
		clients:       make(map[net.Conn]bool),
		subscribers:   make(map[net.Conn]bool),
		tmuxActivity:  make(map[int64]string),
		tmuxAutoState: make(map[int64]*tmuxAutoEntry),
		ctx:           ctx,
		cancel:        cancel,
	}
}

func (s *Server) getProjectContext(projectID int64) (*projectContext, error) {
	s.projectsMu.RLock()
	if pc, ok := s.projects[projectID]; ok {
		s.projectsMu.RUnlock()

		// Check if .sortie.yml has changed since we cached
		configPath := filepath.Join(pc.repoRoot, ".sortie.yml")
		info, statErr := os.Stat(configPath)
		fresh := false
		switch {
		case statErr == nil && !pc.configModTime.IsZero():
			fresh = info.ModTime().Equal(pc.configModTime)
		case statErr != nil && pc.configModTime.IsZero():
			fresh = true // was absent, still absent
		}
		if fresh {
			return pc, nil
		}
		// Config changed — evict and fall through to reload
		s.projectsMu.Lock()
		delete(s.projects, projectID)
		s.projectsMu.Unlock()
		log.Printf("Config changed for project %d (%s), reloading", projectID, pc.repoRoot)
	} else {
		s.projectsMu.RUnlock()
	}

	proj, err := s.database.GetProject(projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	projCfg, err := config.LoadForProject(proj.Path)
	if err != nil {
		log.Printf("Warning: failed to load config for project %s, using defaults: %v", proj.Path, err)
		projCfg = s.cfg
	}

	// Stat config file for future invalidation checks
	var modTime time.Time
	configPath := filepath.Join(proj.Path, ".sortie.yml")
	if info, err := os.Stat(configPath); err == nil {
		modTime = info.ModTime()
	}

	engine := workflow.NewEngine(projCfg, s.database, s.notifier, proj.Path, s.mergeLocks.For(proj.Path))

	pc := &projectContext{
		cfg:           projCfg,
		engine:        engine,
		repoRoot:      proj.Path,
		configModTime: modTime,
	}

	s.projectsMu.Lock()
	s.projects[projectID] = pc
	s.projectsMu.Unlock()

	return pc, nil
}

// projectLogPrefix returns a "[projectname] " prefix for log messages.
// If the project name cannot be resolved, it returns an empty string.
func (s *Server) projectLogPrefix(projectID int64) string {
	if pc, err := s.getProjectContext(projectID); err == nil && pc.cfg.Project.Name != "" {
		return "[" + pc.cfg.Project.Name + "] "
	}
	if proj, err := s.database.GetProject(projectID); err == nil && proj.Name != "" {
		return "[" + proj.Name + "] "
	}
	return ""
}

func (s *Server) getProjectDataDir(t *task.Task) string {
	pc, err := s.getProjectContext(t.ProjectID)
	if err != nil {
		return config.GetGlobalDataDir()
	}
	return filepath.Join(pc.repoRoot, ".sortie")
}

func (s *Server) getProjectRepoRoot(t *task.Task) string {
	pc, err := s.getProjectContext(t.ProjectID)
	if err != nil {
		return ""
	}
	return pc.repoRoot
}

func (s *Server) Start() error {
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

	if err := s.recoverOrphanedTasks(); err != nil {
		log.Printf("Warning: failed to recover orphaned tasks: %v", err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("Received signal %q (pid=%d, ppid=%d)", sig, os.Getpid(), os.Getppid())
		s.Shutdown()
	}()

	s.wg.Add(1)
	go s.acceptLoop()

	s.wg.Add(1)
	go s.taskPollerLoop()

	s.wg.Add(1)
	go s.tmuxMonitorLoop()

	log.Printf("Daemon started, listening on %s (pid=%d, ppid=%d)", s.cfg.Daemon.SocketPath, os.Getpid(), os.Getppid())

	s.wg.Wait()

	return nil
}

func (s *Server) acceptLoop() {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
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
	scanner.Buffer(make([]byte, 0, 1024*1024), scannerBufferSize)
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
		if err := msg.DecodePayload(&req); err != nil {
			s.sendError(conn, "invalid payload")
			return
		}
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

	case MsgGetTask:
		var req GetTaskRequest
		if err := msg.DecodePayload(&req); err != nil {
			s.sendError(conn, "invalid payload")
			return
		}
		s.handleGetTask(conn, req)

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

	case MsgFinalizeTask:
		var req FinalizeTaskRequest
		if err := msg.DecodePayload(&req); err != nil {
			s.sendError(conn, "invalid payload")
			return
		}
		s.handleFinalizeTask(conn, req)

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

	case MsgUpdateField:
		var req UpdateFieldRequest
		if err := msg.DecodePayload(&req); err != nil {
			s.sendError(conn, "invalid payload")
			return
		}
		s.handleUpdateField(conn, req)

	case MsgRevertTask:
		var req RevertTaskRequest
		if err := msg.DecodePayload(&req); err != nil {
			s.sendError(conn, "invalid payload")
			return
		}
		s.handleRevertTask(conn, req)

	case MsgUpdateDependency:
		var req UpdateDependencyRequest
		if err := msg.DecodePayload(&req); err != nil {
			s.sendError(conn, "invalid payload")
			return
		}
		s.handleUpdateDependency(conn, req)

	case MsgDetachBranch:
		var req DetachBranchRequest
		if err := msg.DecodePayload(&req); err != nil {
			s.sendError(conn, "invalid payload")
			return
		}
		s.handleDetachBranch(conn, req)

	case MsgAttachBranch:
		var req AttachBranchRequest
		if err := msg.DecodePayload(&req); err != nil {
			s.sendError(conn, "invalid payload")
			return
		}
		s.handleAttachBranch(conn, req)

	case MsgGetStepContexts:
		var req GetStepContextsRequest
		if err := msg.DecodePayload(&req); err != nil {
			s.sendError(conn, "invalid payload")
			return
		}
		s.handleGetStepContexts(conn, req)

	case MsgGetTaskSteps:
		var req GetTaskStepsRequest
		if err := msg.DecodePayload(&req); err != nil {
			s.sendError(conn, "invalid payload")
			return
		}
		s.handleGetTaskSteps(conn, req)

	case MsgUpdateStepContext:
		var req UpdateStepContextRequest
		if err := msg.DecodePayload(&req); err != nil {
			s.sendError(conn, "invalid payload")
			return
		}
		s.handleUpdateStepContext(conn, req)

	case MsgListWorkflows:
		var req ListWorkflowsRequest
		if err := msg.DecodePayload(&req); err != nil {
			s.sendError(conn, "invalid payload")
			return
		}
		s.handleListWorkflows(conn, req)

	case MsgShutdown:
		s.mu.RLock()
		clientCount := len(s.clients)
		s.mu.RUnlock()
		log.Printf("Received MsgShutdown from socket client (conn=%p, total_clients=%d)", conn, clientCount)
		s.Shutdown()

	default:
		s.sendError(conn, "unknown message type")
	}
}

func (s *Server) Shutdown() {
	s.shutdownOnce.Do(s.shutdown)
}

func (s *Server) shutdown() {
	buf := make([]byte, 8192)
	n := runtime.Stack(buf, false)
	log.Printf("Shutting down daemon (pid=%d, ppid=%d)\nshutdown caller goroutine:\n%s", os.Getpid(), os.Getppid(), buf[:n])

	if s.listener != nil {
		s.listener.Close()
	}

	if s.database != nil {
		runningTasks, err := s.database.GetRunningTasks()
		if err == nil {
			// Collect repo roots that may have in-progress merges
			dirtyRepoRoots := make(map[string]bool)
			for _, t := range runningTasks {
				log.Printf("%sMarking task #%d as failed (daemon shutdown)", s.projectLogPrefix(t.ProjectID), t.ID)
				if err := s.database.UpdateTaskError(t.ID, "daemon shutdown"); err != nil {
					log.Printf("%sFailed to mark task #%d as failed: %v", s.projectLogPrefix(t.ProjectID), t.ID, err)
				}
				if repoRoot := s.getProjectRepoRoot(t); repoRoot != "" {
					dirtyRepoRoots[repoRoot] = true
				}
			}
			// Clean up any staged changes left by interrupted merges
			for repoRoot := range dirtyRepoRoots {
				if err := gitpkg.CleanRepoState(repoRoot); err != nil {
					log.Printf("Warning: failed to clean repo state for %s on shutdown: %v", repoRoot, err)
				} else {
					log.Printf("Cleaned repo state for %s on shutdown", repoRoot)
				}
			}
		}
	}

	s.manager.Shutdown(agentShutdownGracePeriod)

	s.projectsMu.RLock()
	for _, pc := range s.projects {
		if sessions, err := tmux.ListSessions(tmux.SessionPrefix(pc.cfg.Project.Name)); err == nil {
			for _, sess := range sessions {
				sess.Kill()
			}
		}
	}
	s.projectsMu.RUnlock()

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

func Start(cfg *config.Config) error {
	dbPath := cfg.GetDatabasePath("")
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	server := NewServer(cfg, database)
	return server.Start()
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
