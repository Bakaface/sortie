package daemon

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aface/sortie/internal/claude"
	gitpkg "github.com/aface/sortie/internal/git"
	"github.com/aface/sortie/internal/task"
	"github.com/aface/sortie/internal/tmux"
	"github.com/aface/sortie/internal/workflow"
)

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

	if err := writeClaudeScript(scriptFile, pc.cfg.Claude.Yolo, ""); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to write wrapper script: %v", err))
		return
	}

	setupCmd := pc.cfg.TmuxSetupCommand
	if tmux.SetupCommandControlsAgent(setupCmd) {
		if err := session.Create(""); err != nil {
			s.sendError(conn, fmt.Sprintf("failed to create tmux session: %v", err))
			return
		}
	} else {
		if err := session.Create("bash", scriptFile); err != nil {
			s.sendError(conn, fmt.Sprintf("failed to create tmux session: %v", err))
			return
		}
	}

	// Run tmux setup command if configured
	if setupCmd != "" {
		claudeCmd := "claude"
		if pc.cfg.Claude.Yolo {
			claudeCmd += " --dangerously-skip-permissions"
		}
		vars := &tmux.SetupVars{
			ClaudeCommand: claudeCmd,
			RunAgent:      scriptFile,
		}
		if err := session.RunSetupCommand(setupCmd, vars); err != nil {
			log.Printf("%sWarning: tmux setup command failed for task #%d: %v", s.projectLogPrefix(t.ProjectID), t.ID, err)
		}
	}

	// Async: discover Claude session ID and record it
	go func() {
		sid, _ := claude.FindSessionByWorkdir(t.WorktreePath, 15*time.Second)
		if sid != "" {
			if err := s.database.UpsertChat(t.ID, "continue", sid, session.Name); err != nil {
				log.Printf("%sWarning: failed to upsert chat for continue task #%d: %v", s.projectLogPrefix(t.ProjectID), t.ID, err)
			}
		}
	}()

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

	// Determine whether the workflow has more steps to run.
	// The engine increments t.StepIndex to i+1 before pausing at the tmux gate,
	// so t.StepIndex already points at the next step. If t.StepIndex < len(steps)
	// there is more work to do; advance to the next step instead of finalizing.
	wf := pc.cfg.GetWorkflow(t.Workflow)
	hasMoreSteps := wf != nil && t.StepIndex < len(wf.Steps)

	if hasMoreSteps {
		// Advance to the next step: resume the engine. ResumeAfterApproval will
		// run summarise_chat for the just-completed tmux step (Sub-feature D) and
		// then continue with the remaining workflow steps.
		origStatus := t.Status
		if err := s.database.UpdateTaskStatus(t.ID, task.StatusRunning); err != nil {
			s.sendError(conn, fmt.Sprintf("failed to update task status: %v", err))
			return
		}

		engine := pc.engine
		manager := s.manager
		runner := func(ctx context.Context) error {
			var outputFn func([]string)
			if a, ok := manager.GetAgentByTaskID(t.ID); ok {
				outputFn = a.AppendOutput
			}
			return engine.ResumeAfterApproval(ctx, t, outputFn)
		}
		if _, err := s.manager.StartAgent(t, t.WorktreePath, runner); err != nil {
			_ = s.database.UpdateTaskStatus(t.ID, origStatus)
			s.sendError(conn, fmt.Sprintf("failed to start agent: %v", err))
			return
		}

		s.broadcastTaskUpdate(t.ID)
		log.Printf("%sTask #%d: tmux step done, advancing to step %d of %d", s.projectLogPrefix(t.ProjectID), t.ID, t.StepIndex, len(wf.Steps))
		s.sendMessage(conn, MsgOK, OKResponse{Message: fmt.Sprintf("task #%d advancing to next step", t.ID)})
		return
	}

	// Last step: run full finalization (on_complete merge + summarizer + cleanup).

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
	if setupCmds := pc.cfg.GetWorktreeSetupCommands(nil); len(setupCmds) > 0 {
		if err := workflow.RunWorktreeSetupCommands(context.Background(), pc.repoRoot, t.WorktreePath, setupCmds); err != nil {
			log.Printf("%sWarning: worktree setup commands failed for task #%d: %v", s.projectLogPrefix(t.ProjectID), t.ID, err)
		}
	}

	return nil
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

// setupTmuxDirect creates a tmux session for a task that should go directly into tmux state,
// skipping the normal workflow. Used for branch tasks created via the "b" keybind.
func (s *Server) setupTmuxDirect(taskID, projectID int64, title string) {
	slug := task.Slugify(title)

	t, err := s.database.GetTask(taskID)
	if err != nil {
		log.Printf("%sFailed to get task #%d for tmux-direct: %v", s.projectLogPrefix(projectID), taskID, err)
		return
	}

	pc, err := s.getProjectContext(projectID)
	if err != nil {
		log.Printf("%sFailed to get project context for tmux-direct task #%d: %v", s.projectLogPrefix(projectID), taskID, err)
		return
	}

	// Resolve branch
	branch := t.CheckoutBranch

	if err := s.database.FinalizeTaskIdentity(taskID, title, slug, branch); err != nil {
		log.Printf("%sFailed to finalize identity for tmux-direct task #%d: %v", s.projectLogPrefix(projectID), taskID, err)
		return
	}

	// Ensure worktree exists
	if err := s.ensureWorktree(t, pc); err != nil {
		log.Printf("%sFailed to ensure worktree for tmux-direct task #%d: %v", s.projectLogPrefix(projectID), taskID, err)
		return
	}

	if !tmux.IsAvailable() {
		log.Printf("%sTmux not available for tmux-direct task #%d", s.projectLogPrefix(projectID), taskID)
		return
	}

	// Create tmux session
	taskIDStr := fmt.Sprintf("%d", taskID)
	session := tmux.NewSession(pc.cfg.Project.Name, taskIDStr, t.WorktreePath)

	if session.Exists() {
		session.Kill()
	}

	sortieDir := filepath.Join(t.WorktreePath, ".sortie")
	if err := os.MkdirAll(sortieDir, 0755); err != nil {
		log.Printf("%sFailed to create sortie dir for tmux-direct task #%d: %v", s.projectLogPrefix(projectID), taskID, err)
		return
	}
	scriptFile := filepath.Join(sortieDir, "run-continue.sh")

	if err := writeClaudeScript(scriptFile, pc.cfg.Claude.Yolo, ""); err != nil {
		log.Printf("%sFailed to write claude script for tmux-direct task #%d: %v", s.projectLogPrefix(projectID), taskID, err)
		return
	}

	setupCmd := pc.cfg.TmuxSetupCommand
	if tmux.SetupCommandControlsAgent(setupCmd) {
		if err := session.Create(""); err != nil {
			log.Printf("%sFailed to create tmux session for tmux-direct task #%d: %v", s.projectLogPrefix(projectID), taskID, err)
			return
		}
	} else {
		if err := session.Create("bash", scriptFile); err != nil {
			log.Printf("%sFailed to create tmux session for tmux-direct task #%d: %v", s.projectLogPrefix(projectID), taskID, err)
			return
		}
	}

	// Run tmux setup command if configured
	if setupCmd != "" {
		claudeCmd := "claude"
		if pc.cfg.Claude.Yolo {
			claudeCmd += " --dangerously-skip-permissions"
		}
		vars := &tmux.SetupVars{
			ClaudeCommand: claudeCmd,
			RunAgent:      scriptFile,
		}
		if err := session.RunSetupCommand(setupCmd, vars); err != nil {
			log.Printf("%sWarning: tmux setup command failed for tmux-direct task #%d: %v", s.projectLogPrefix(projectID), taskID, err)
		}
	}

	// Async: discover Claude session ID and record it
	worktreePath := t.WorktreePath
	sessionName := session.Name
	go func() {
		sid, _ := claude.FindSessionByWorkdir(worktreePath, 15*time.Second)
		if sid != "" {
			if err := s.database.UpsertChat(taskID, "tmux-direct", sid, sessionName); err != nil {
				log.Printf("%sWarning: failed to upsert chat for tmux-direct task #%d: %v", s.projectLogPrefix(projectID), taskID, err)
			}
		}
	}()

	if err := s.database.UpdateTaskStatus(taskID, task.StatusTmux); err != nil {
		log.Printf("%sFailed to update status for tmux-direct task #%d: %v", s.projectLogPrefix(projectID), taskID, err)
		return
	}

	s.broadcastTaskUpdate(taskID)
	log.Printf("%sTmux-direct session started for task #%d (tmux: %s)", s.projectLogPrefix(projectID), taskID, session.Name)
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

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// writeClaudeScript writes a bash wrapper script that runs claude and drops to a shell.
// If resumeSessionID is non-empty, the script invokes `claude --resume <id>` to
// restore a previous chat session.
func writeClaudeScript(scriptPath string, yolo bool, resumeSessionID string) error {
	script := fmt.Sprintf("#!/bin/bash\n%s\nexec bash\n", buildClaudeCommand(yolo, resumeSessionID))
	return os.WriteFile(scriptPath, []byte(script), 0755)
}

// buildClaudeCommand assembles the `claude` CLI invocation with the appropriate
// flags. If resumeSessionID is non-empty, `--resume <id>` is appended so the
// chat is automatically restored.
func buildClaudeCommand(yolo bool, resumeSessionID string) string {
	cmd := "claude"
	if yolo {
		cmd += " --dangerously-skip-permissions"
	}
	if resumeSessionID != "" {
		cmd += " --resume " + resumeSessionID
	}
	return cmd
}
