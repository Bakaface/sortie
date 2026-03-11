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
	"sync"
	"syscall"
	"time"

	"path/filepath"

	"github.com/aface/sortie/internal/agent"
	"github.com/aface/sortie/internal/config"
	"github.com/aface/sortie/internal/db"
	"github.com/aface/sortie/internal/notify"
	"github.com/aface/sortie/internal/task"
	"github.com/aface/sortie/internal/tmux"
	"github.com/aface/sortie/internal/workflow"
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

	mu           sync.RWMutex
	clients      map[net.Conn]bool
	subscribers  map[net.Conn]bool
	tmuxActivity map[int64]string

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
		clients:      make(map[net.Conn]bool),
		subscribers:  make(map[net.Conn]bool),
		tmuxActivity: make(map[int64]string),
		ctx:         ctx,
		cancel:      cancel,
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

	engine := workflow.NewEngine(projCfg, s.database, s.notifier, proj.Path)

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

	s.wg.Add(1)
	go s.tmuxMonitorLoop()

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
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
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
		msg.DecodePayload(&req)
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

	case MsgShutdown:
		s.Shutdown()

	default:
		s.sendError(conn, "unknown message type")
	}
}

func (s *Server) Shutdown() {
	log.Println("Shutting down daemon...")

	if s.listener != nil {
		s.listener.Close()
	}

	if s.database != nil {
		runningTasks, err := s.database.GetRunningTasks()
		if err == nil {
			for _, t := range runningTasks {
				log.Printf("%sMarking task #%d as failed (daemon shutdown)", s.projectLogPrefix(t.ProjectID), t.ID)
				if err := s.database.UpdateTaskError(t.ID, "daemon shutdown"); err != nil {
					log.Printf("%sFailed to mark task #%d as failed: %v", s.projectLogPrefix(t.ProjectID), t.ID, err)
				}
			}
		}
	}

	s.manager.Shutdown(30 * time.Second)

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
