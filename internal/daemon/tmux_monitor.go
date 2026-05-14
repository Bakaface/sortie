package daemon

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aface/sortie/internal/task"
	"github.com/aface/sortie/internal/tmux"
	"github.com/aface/sortie/internal/workflow"
)

// tmuxIdleFallbackDuration is how long a tmux pane must remain in the
// ActivityIdle state before the daemon assumes the Claude turn has finished
// and triggers auto-advance. The hash-stability detector already requires
// several stable polls before flagging idle; this additional dwell time
// provides margin against very-slow streaming output that briefly stabilises.
//
// Why 30s: balances tail-latency tolerance against operator wait time when
// the Stop hook has been disabled by managed-settings policy and we have to
// rely on the fallback. Pulled from the brief.
const tmuxIdleFallbackDuration = 30 * time.Second

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

		// Update activity broadcast / cache when it changed.
		if changed {
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

		// Evaluate auto-advance every tick (not only on `changed`) so the
		// fallback timer can fire even when the activity state has held at
		// `idle` across multiple polls.
		s.maybeAutoAdvance(t, activity)
	}

	// Cleanup: remove sessions / state for tasks that are no longer in tmux mode.
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
			delete(s.tmuxAutoState, taskID)
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

// maybeAutoAdvance inspects a tmux-state task and triggers auto-advance when
// the just-finished step is configured for it AND either:
//
//   - the Claude Code Stop hook has dropped a sentinel file in
//     <worktree>/.sortie/step-done/ (primary signal), or
//   - the tmux pane has been ActivityIdle for tmuxIdleFallbackDuration
//     (fallback signal, in case hooks were disabled).
//
// Tasks whose just-finished step is marked `human: true` are never
// auto-advanced — they pause at the approval gate until the user acts. The
// sentinel is still consumed (and discarded) so it doesn't leak across the
// next attach/continue.
func (s *Server) maybeAutoAdvance(t *task.Task, activity tmux.Activity) {
	// Don't double-advance if a prior tick already kicked off the engine.
	s.mu.RLock()
	entry, hasEntry := s.tmuxAutoState[t.ID]
	if hasEntry && entry.advancing {
		s.mu.RUnlock()
		return
	}
	s.mu.RUnlock()

	pc, err := s.getProjectContext(t.ProjectID)
	if err != nil {
		return
	}

	// Figure out which step just finished. The engine bumps StepIndex to i+1
	// before pausing at the tmux gate, so the just-finished step is at
	// StepIndex-1.
	wf := pc.cfg.GetWorkflow(t.Workflow)
	if wf == nil || t.StepIndex <= 0 || t.StepIndex > len(wf.Steps) {
		return
	}
	justFinished := wf.Steps[t.StepIndex-1]

	// Steps that the user explicitly wants to approve are out of scope for
	// auto-advance. Consume any stray sentinel so it doesn't trigger advance
	// the next time the user attaches an interactive session.
	if justFinished.Human {
		consumeSentinels(t.WorktreePath)
		return
	}

	// Primary signal: presence of any sentinel file written by the Stop hook.
	hasSentinel := stopHookSentinelExists(t.WorktreePath)

	// Fallback signal: pane has been idle for tmuxIdleFallbackDuration.
	fallbackReady := false
	now := time.Now()
	s.mu.Lock()
	if entry == nil {
		entry = &tmuxAutoEntry{}
		s.tmuxAutoState[t.ID] = entry
	}
	if activity == tmux.ActivityIdle {
		if entry.firstIdleAt.IsZero() {
			entry.firstIdleAt = now
		}
		if now.Sub(entry.firstIdleAt) >= tmuxIdleFallbackDuration {
			fallbackReady = true
		}
	} else {
		// Any non-idle state resets the idle timer so we don't accumulate
		// dwell time across WIP transitions.
		entry.firstIdleAt = time.Time{}
	}
	s.mu.Unlock()

	if !hasSentinel && !fallbackReady {
		return
	}

	// Mark advancing before any side-effects so the next tick is a no-op.
	s.mu.Lock()
	entry.advancing = true
	s.mu.Unlock()

	// Sentinel files have served their purpose — clear them so they don't
	// resurrect advance attempts after a manual finalize/retry.
	consumeSentinels(t.WorktreePath)

	signal := "stop-hook sentinel"
	if !hasSentinel {
		signal = fmt.Sprintf("idle for %s (fallback)", tmuxIdleFallbackDuration)
	}
	log.Printf("%sTask #%d: auto-advancing via %s (step %q done)",
		s.projectLogPrefix(t.ProjectID), t.ID, signal, justFinished.Name)

	outcome, err := s.advanceTmuxTask(t)
	if err != nil {
		log.Printf("%sTask #%d: auto-advance failed: %v", s.projectLogPrefix(t.ProjectID), t.ID, err)
		// Reset advancing so a future tick can retry. The step state in
		// the DB is unchanged because advanceTmuxTask rolls back on error.
		s.mu.Lock()
		if e, ok := s.tmuxAutoState[t.ID]; ok {
			e.advancing = false
		}
		s.mu.Unlock()
		return
	}
	log.Printf("%sTask #%d: auto-advance result: %s", s.projectLogPrefix(t.ProjectID), t.ID, outcome.message)
}

// stopHookSentinelExists returns true when the Stop-hook step-done directory
// contains at least one sentinel JSON file. Reading errors (missing dir,
// permission denied) are treated as "no sentinel" — the fallback timer
// remains the safety net.
func stopHookSentinelExists(worktreePath string) bool {
	if worktreePath == "" {
		return false
	}
	entries, err := os.ReadDir(workflow.StepDoneDir(worktreePath))
	if err != nil {
		return false
	}
	for _, e := range entries {
		// Skip dotfiles (the hook command writes its temp file as `.<pid>...`
		// and renames it on success; transient dotfiles are not sentinels).
		if len(e.Name()) > 0 && e.Name()[0] == '.' {
			continue
		}
		if !e.IsDir() {
			return true
		}
	}
	return false
}

// consumeSentinels removes every sentinel JSON file from the step-done
// directory. Errors are intentionally swallowed: a leftover sentinel only
// triggers a single redundant auto-advance attempt at worst, and the
// monitor's `advancing` flag guards against double-firing within one task.
func consumeSentinels(worktreePath string) {
	if worktreePath == "" {
		return
	}
	dir := workflow.StepDoneDir(worktreePath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		_ = os.Remove(dir + "/" + e.Name())
	}
}
