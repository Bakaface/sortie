package daemon

import (
	"fmt"
	"log"
	"time"

	"github.com/aface/sortie/internal/task"
	"github.com/aface/sortie/internal/tmux"
)

func (s *Server) tmuxMonitorLoop() {
	defer s.wg.Done()

	cfg := tmux.DefaultMonitorConfig()
	monitor := tmux.NewMonitor(cfg)

	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.checkTmuxActivity(monitor)
		}
	}
}

func (s *Server) checkTmuxActivity(monitor *tmux.Monitor) {
	tasks, err := s.database.GetAllTasks()
	if err != nil {
		return
	}

	// Collect active tmux task IDs for cleanup
	activeSessions := make(map[string]bool)

	for _, t := range tasks {
		if t.Status != task.StatusTmux {
			continue
		}

		pc, err := s.getProjectContext(t.ProjectID)
		if err != nil {
			continue
		}

		taskID := fmt.Sprintf("%d", t.ID)
		session := tmux.NewSession(pc.cfg.Project.Name, taskID, t.WorktreePath)
		activeSessions[session.Name] = true

		if !session.Exists() {
			continue
		}

		activity, changed := monitor.Check(session)
		if !changed {
			continue
		}

		activityStr := string(activity)
		log.Printf("%sTask #%d tmux activity changed to: %s", s.projectLogPrefix(t.ProjectID), t.ID, activityStr)

		s.mu.Lock()
		s.tmuxActivity[t.ID] = activityStr
		s.mu.Unlock()

		// Broadcast activity change
		s.broadcastToSubscribers(MsgTmuxActivity, TmuxActivityResponse{
			TaskID:   t.ID,
			Activity: activityStr,
		})

		// Also broadcast a task update so TUI gets the TmuxActivity field
		s.broadcastTaskUpdate(t.ID)
	}

	// Cleanup: remove sessions from the monitor that are no longer tracked
	s.mu.Lock()
	for taskID := range s.tmuxActivity {
		found := false
		for _, t := range tasks {
			if t.ID == taskID && t.Status == task.StatusTmux {
				found = true
				break
			}
		}
		if !found {
			delete(s.tmuxActivity, taskID)
		}
	}
	s.mu.Unlock()

	// Remove stale sessions from monitor
	for name := range monitor.Sessions() {
		if !activeSessions[name] {
			monitor.Remove(name)
		}
	}
}
