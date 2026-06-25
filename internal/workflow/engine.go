package workflow

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/Bakaface/sortie/internal/config"
	"github.com/Bakaface/sortie/internal/db"
	gitpkg "github.com/Bakaface/sortie/internal/git"
	"github.com/Bakaface/sortie/internal/merge"
	"github.com/Bakaface/sortie/internal/notify"
	"github.com/Bakaface/sortie/internal/task"
)

const (
	// processExitPollInterval is how often to check whether a Claude subprocess has exited.
	processExitPollInterval = 500 * time.Millisecond
)

type Engine struct {
	cfg      *config.Config
	database *db.DB
	notifier *notify.Notifier
	repoRoot string
	dataDir  string
	coord    *merge.Coordinator // owns merge serialization, retry, and cleanup-on-failure
}

// NewEngine creates a new workflow engine. The optional Lock parameter is an
// externally-owned per-repo merge serializer; when non-nil, the lock survives
// config reloads (the daemon owns it via merge.Locks). When nil, a fresh lock
// is created for standalone/test usage.
func NewEngine(cfg *config.Config, database *db.DB, notifier *notify.Notifier, repoRoot string, lock ...*merge.Lock) *Engine {
	var lk *merge.Lock
	if len(lock) > 0 && lock[0] != nil {
		lk = lock[0]
	} else {
		lk = &merge.Lock{}
	}
	e := &Engine{
		cfg:      cfg,
		database: database,
		notifier: notifier,
		repoRoot: repoRoot,
		dataDir:  filepath.Join(repoRoot, ".sortie"),
	}
	e.coord = merge.NewCoordinator(
		repoRoot,
		lk,
		merge.Config{},
		e.bindConflictResolver(),
		database.UpdateTaskStatus,
		database.AppendTaskCommit,
	)
	return e
}

// Coord returns the engine's merge coordinator. The daemon uses it to take the
// per-repo lock for ad-hoc operations (revert, worktree teardown) that must
// serialize against merges.
func (e *Engine) Coord() *merge.Coordinator { return e.coord }

// buildTemplateContext constructs a TemplateContext wired to the engine's
// database for {{tasks.X.field}} lookups. Centralised so engine + summarizer
// call sites don't drift.
func (e *Engine) buildTemplateContext(t *task.Task, taskVars TaskVars, stepContexts map[string]string, loopVars LoopVars) *TemplateContext {
	return &TemplateContext{
		Task:  taskVars,
		Steps: stepContexts,
		Git: GitVars{
			BaseBranch:   e.cfg.Git.BaseBranch,
			TargetBranch: e.effectiveBaseBranch(t),
			RepoRoot:     e.repoRoot,
		},
		Loop:       loopVars,
		TaskLookup: e.database.GetTask,
	}
}

// findStepIndex returns the index of a step by name, or -1 if not found.
func findStepIndex(steps []config.StepConfig, name string) int {
	for i, s := range steps {
		if s.Name == name {
			return i
		}
	}
	return -1
}

// effectiveBaseBranch returns the base branch for a task, checking the task's
// per-task TargetBranch override first, then falling back to the config's Git.BaseBranch,
// and finally to "main".
func (e *Engine) effectiveBaseBranch(t *task.Task) string {
	if t.TargetBranch != "" {
		return t.TargetBranch
	}
	if e.cfg.Git.BaseBranch != "" {
		return e.cfg.Git.BaseBranch
	}
	return "main"
}

// effectiveOnComplete returns the finalization action for a task: the workflow's
// on_complete override when set, otherwise the project-level on_complete.
func (e *Engine) effectiveOnComplete(t *task.Task) string {
	if wf := e.cfg.GetWorkflow(t.Workflow); wf != nil && wf.OnComplete != "" {
		return wf.OnComplete
	}
	return e.cfg.OnComplete
}

// RunTask executes the full workflow pipeline for a task.
// It creates/reuses the worktree, then loops through steps starting from t.StepIndex.
// outputFn is called with parsed log lines for live streaming (may be nil).
func (e *Engine) RunTask(ctx context.Context, t *task.Task, outputFn func([]string)) error {
	wf := e.cfg.GetWorkflow(t.Workflow)
	steps := wf.Steps

	if t.Worktree {
		if t.CheckoutBranch != "" {
			// Checkout existing branch mode
			t.Branch = t.CheckoutBranch
			if dbErr := e.database.UpdateTaskBranch(t.ID, t.Branch); dbErr != nil {
				log.Printf("Warning: failed to persist branch name for task #%d: %v", t.ID, dbErr)
			}
			if t.WorktreePath == "" {
				worktree, err := gitpkg.CheckoutWorktree(e.repoRoot, t.ID, t.CheckoutBranch)
				if err != nil {
					return fmt.Errorf("failed to checkout worktree: %w", err)
				}
				t.WorktreePath = worktree.Path
				if err := e.database.UpdateTaskWorktreePath(t.ID, worktree.Path); err != nil {
					log.Printf("Warning: failed to update worktree path: %v", err)
				}
			}
		} else {
			// Normal new branch mode
			if t.Branch == "" {
				t.Branch = e.cfg.ResolveBranchForTask(t.ID, t.Title, t.Slug, t.BranchName)
				if dbErr := e.database.UpdateTaskBranch(t.ID, t.Branch); dbErr != nil {
					log.Printf("Warning: failed to persist branch name for task #%d: %v", t.ID, dbErr)
				}
			}
			if t.WorktreePath == "" {
				worktree, err := gitpkg.CreateWorktree(e.repoRoot, t.ID, e.effectiveBaseBranch(t), t.Branch)
				if err != nil {
					return fmt.Errorf("failed to create worktree: %w", err)
				}
				t.WorktreePath = worktree.Path
				if err := e.database.UpdateTaskWorktreePath(t.ID, worktree.Path); err != nil {
					log.Printf("Warning: failed to update worktree path: %v", err)
				}
			}
		}
	} else {
		// No-worktree mode: run in project root directory.
		// Persist the path so daemon restart can restore tmux sessions for
		// non-worktree tasks (DB row would otherwise have NULL worktree_path).
		t.WorktreePath = e.repoRoot
		if err := e.database.UpdateTaskWorktreePath(t.ID, e.repoRoot); err != nil {
			log.Printf("Warning: failed to persist worktree path for non-worktree task #%d: %v", t.ID, err)
		}
	}

	// Sync configured paths from project root into the worktree
	if t.Worktree {
		syncPaths := e.cfg.GetWorktreeSyncPaths(wf)
		if !syncPaths.IsEmpty() {
			if err := SyncPathsToWorktree(e.repoRoot, t.WorktreePath, syncPaths); err != nil {
				log.Printf("Warning: failed to sync paths to worktree: %v", err)
			}
		}
	}

	// Run worktree setup command(s) if configured
	if t.Worktree {
		if setupCmd := e.cfg.GetWorktreeSetupCommand(wf); setupCmd != "" {
			if err := RunWorktreeSetupCommand(ctx, e.repoRoot, t.WorktreePath, setupCmd); err != nil {
				return fmt.Errorf("worktree setup failed: %w", err)
			}
		}
		if setupCmds := e.cfg.GetWorktreeSetupCommands(wf); len(setupCmds) > 0 {
			if err := RunWorktreeSetupCommands(ctx, e.repoRoot, t.WorktreePath, setupCmds); err != nil {
				return fmt.Errorf("worktree setup failed: %w", err)
			}
		}
	}

	// Ensure .sortie directories exist in worktree
	if err := EnsureWorkDirs(t.WorktreePath); err != nil {
		return fmt.Errorf("failed to create sortie dirs: %w", err)
	}

	// Copy attached images to worktree
	var imageRelPaths []string
	if len(t.Images) > 0 {
		var err error
		imageRelPaths, err = CopyImagesToWorktree(t.WorktreePath, t.Images)
		if err != nil {
			log.Printf("Warning: failed to copy images to worktree: %v", err)
		}
	}

	// Loop through steps from current index
	for i := t.StepIndex; i < len(steps); i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		step := steps[i]

		// Update step tracking
		if err := e.database.UpdateTaskStep(t.ID, i, step.Name); err != nil {
			log.Printf("Warning: failed to update task step: %v", err)
		}

		// If we are resuming after children finished (StatusAwaitingChildren →
		// pending via the poller), the wait-on edges still exist. Snapshot the
		// children for {{children.*}} template vars, then clear the edges so
		// the post-step check sees only NEW edges added during this run.
		// First-time step runs simply find an empty list and skip the work.
		childrenVars, err := e.loadAndClearChildren(t)
		if err != nil {
			log.Printf("Warning: failed to load waits-on children for task #%d: %v", t.ID, err)
		}

		// Record step start in task_steps table
		if err := e.database.CreateTaskStep(t.ID, step.Name); err != nil {
			log.Printf("Warning: failed to create task step record: %v", err)
		}

		// Collect step contexts from ALL steps that have a recorded context.
		// We include later-indexed steps too because loops can populate them
		// in earlier iterations (e.g. reviewing's issues feeding back to
		// implementing on the next pass). Steps that haven't run yet simply
		// have no DB row and resolve to "".
		var allStepNames []string
		for _, s := range steps {
			allStepNames = append(allStepNames, s.Name)
		}
		stepContexts, err := e.database.GetTaskStepContexts(t.ID, allStepNames)
		if err != nil {
			log.Printf("Warning: failed to get step contexts: %v", err)
			stepContexts = make(map[string]string)
		}

		// Build template context and resolve prompt
		loopVars := LoopVars{}
		if step.Loop != nil {
			loopVars.Iteration = t.LoopIteration
			loopVars.MaxIterations = step.Loop.MaxIterations
		} else {
			// Check if any step in the current loop range references this step
			// to populate loop vars for non-loop steps inside a loop
			for _, s := range steps {
				if s.Loop != nil && s.Loop.Goto != "" {
					targetIdx := findStepIndex(steps, s.Loop.Goto)
					if targetIdx >= 0 && targetIdx <= i && i <= findStepIndex(steps, s.Name) {
						loopVars.Iteration = t.LoopIteration
						loopVars.MaxIterations = s.Loop.MaxIterations
						break
					}
				}
			}
		}
		// Pre-resolve any {{tasks.X.field}} refs inside this task's own
		// description / context so step prompts that inline {{task.description}}
		// or {{task.context}} see the expanded text. Single-pass — nested refs
		// in the looked-up task's fields remain verbatim.
		resolvedDescription := ResolveTaskRefs(t.Description, e.database.GetTask)
		resolvedContext := ResolveTaskRefs(t.Context, e.database.GetTask)

		tmplCtx := e.buildTemplateContext(t, TaskVars{
			ID:          t.ID,
			Title:       t.Title,
			Description: resolvedDescription,
			Context:     resolvedContext,
			Slug:        t.Slug,
			Branch:      t.Branch,
			Images:      imageRelPaths,
		}, stepContexts, loopVars)
		tmplCtx.Children = childrenVars

		resolvedPrompt := ResolveTemplate(step.Prompt, tmplCtx)

		sysPrompt := BuildSystemPrompt(resolvedPrompt, e.cfg.SystemPrompt, imageRelPaths)

		// Set environment variables
		env := map[string]string{
			"SORTIE_TASK_ID":  fmt.Sprintf("%d", t.ID),
			"SORTIE_STEP":     step.Name,
			"SORTIE_WORKTREE": t.WorktreePath,
			"SORTIE_PURPOSE":  "step",
			// SORTIE_PROJECT_PATH is the absolute path to the project repo
			// root (NOT the worktree). MCP servers / scripts spawned inside
			// the step that need to create child tasks under the same
			// project use this so they don't accidentally register their
			// `git rev-parse --show-toplevel` (which in a worktree returns
			// the worktree path) as a new project row.
			"SORTIE_PROJECT_PATH": e.repoRoot,
		}

		// Spawn Claude process (tmux or direct)
		useTmux := step.UseTmux(wf.Print)
		var exitCode int
		var resultText string
		var outputTail string
		var sessionID string
		if useTmux {
			exitCode, outputTail, err = e.runClaudeStepTmux(ctx, t, step, resolvedPrompt, env, outputFn, sysPrompt)
		} else {
			exitCode, resultText, sessionID, outputTail, err = e.runClaudeStep(ctx, t, step, resolvedPrompt, env, outputFn, sysPrompt)
		}
		if err != nil {
			e.database.UpdateTaskExitCode(t.ID, 1, err.Error())
			return fmt.Errorf("step %q failed: %w", step.Name, err)
		}

		if exitCode != 0 {
			errMsg := fmt.Sprintf("step %q exited with code %d", step.Name, exitCode)
			if outputTail != "" {
				errMsg += "\n" + outputTail
			}
			e.database.UpdateTaskExitCode(t.ID, exitCode, errMsg)
			return errors.New(errMsg)
		}

		// Record Claude session for this step
		if sessionID != "" {
			if chatErr := e.database.UpsertChat(t.ID, step.Name, sessionID, ""); chatErr != nil {
				log.Printf("Warning: failed to upsert chat for task #%d step %q: %v", t.ID, step.Name, chatErr)
			}
		}

		// If this step spawned child tasks via the MCP create_tasks_and_wait /
		// wait_for_tasks tools, the daemon recorded task_waits_on edges. Suspend
		// the task at the SAME step (do not advance step_index) so the engine
		// re-runs this step once the children reach terminal status. The poller
		// detects "all children terminal" and flips status back to pending. On
		// the re-run, loadAndClearChildren above will surface their results via
		// {{children.*}} template variables.
		hasWaits, waitsErr := e.database.HasAnyWaitsOn(t.ID)
		if waitsErr != nil {
			log.Printf("Warning: failed to check waits-on for task #%d: %v", t.ID, waitsErr)
		}
		if hasWaits {
			// Skip output validation, step completion record, summarize_chat,
			// loop handling, and the tmux/human approval pause — none of them
			// apply to a step that is going to re-run. The step_index stays at
			// i so the next invocation resumes here.
			if err := e.database.UpdateTaskStatus(t.ID, task.StatusAwaitingChildren); err != nil {
				log.Printf("Warning: failed to set %s status for task #%d: %v", task.StatusAwaitingChildren, t.ID, err)
			}
			log.Printf("Task #%d suspended at step %q: waiting for spawned child tasks", t.ID, step.Name)
			return nil
		}

		// Validate that the step produced meaningful changes
		// Skip for human and tmux steps (handled separately or not yet complete)
		if !step.Human && !useTmux {
			if strings.TrimSpace(resultText) == "" {
				// No output — require a diff as the alternative signal
				noiseFiles := []string{".claude-output.log"}
				hasChanges, err := gitpkg.HasMeaningfulChanges(t.WorktreePath, noiseFiles)
				if err != nil {
					log.Printf("Warning: failed to check for changes in step %q: %v", step.Name, err)
				} else if !hasChanges {
					errMsg := fmt.Sprintf("step %q exited successfully but produced no output or code changes", step.Name)
					e.database.UpdateTaskExitCode(t.ID, 1, errMsg)
					return errors.New(errMsg)
				}
			}
		}

		// Detect a manual context override pushed via the update_step_context
		// MCP tool while the step was running. The step row is still 'running'
		// at this point (CompleteTaskStep below flips it to 'completed'), and
		// CreateTaskStep resets the context on each run, so a non-empty value
		// here can only be a deliberate manual write. When present it takes
		// precedence over both last_message capture and summarize_chat.
		manualContext, mErr := e.database.GetRunningTaskStepContext(t.ID, step.Name)
		if mErr != nil {
			log.Printf("Warning: failed to read running step context for step %q of task #%d: %v", step.Name, t.ID, mErr)
		}
		hasManualContext := strings.TrimSpace(manualContext) != ""

		// Record step completion with context.
		// For summarize_chat, store last_message immediately and kick off
		// background summarization that will overwrite the context when done.
		// For "none", skip context capture entirely so later steps see no context.
		// A manual override is preserved verbatim regardless of strategy.
		strategy := step.EffectiveSummarizationStrategy()
		stepContextText := resultText
		var ctxPtr *string
		if hasManualContext {
			ctxPtr = &manualContext
		} else if strategy != config.SummarizationStrategyNone && stepContextText != "" {
			ctxPtr = &stepContextText
		}
		if err := e.database.CompleteTaskStep(t.ID, step.Name, ctxPtr, exitCode); err != nil {
			log.Printf("Warning: failed to complete task step record: %v", err)
		}

		if hasManualContext {
			log.Printf("Step %q of task #%d has a manual context override (%d chars); skipping summarize_chat", step.Name, t.ID, len(manualContext))
		}

		if !hasManualContext && strategy == config.SummarizationStrategySummarizeChat {
			chat, chatErr := e.loadStepChatContent(t, step.Name, useTmux)
			if chatErr != nil {
				log.Printf("Warning: failed to load chat content for step %q of task #%d: %v", step.Name, t.ID, chatErr)
			} else if chat != "" && shouldSummarizeChat(chat, resultText, useTmux) {
				// Surface the step summarization phase via the task status so
				// the TUI can distinguish it from regular step execution.
				restore := e.markSummarizingStep(t, wf)
				summary, sumErr := e.summarizeChatLog(ctx, t, step.Name, step.SummarizationPrompt, chat, step.EffectiveAllowedSummarizationModels(e.cfg.AllowedSummarizationModels))
				restore()
				if sumErr != nil {
					log.Printf("Warning: summarize_chat failed for step %q of task #%d: %v", step.Name, t.ID, sumErr)
				} else if summary != "" {
					if dbErr := e.database.UpdateTaskStepContext(t.ID, step.Name, summary); dbErr != nil {
						log.Printf("Warning: failed to update step context after summarize_chat for step %q of task #%d: %v", step.Name, t.ID, dbErr)
					} else {
						log.Printf("summarize_chat updated step context for step %q of task #%d (%d chars)", step.Name, t.ID, len(summary))
					}
				}
			}
		}

		// Evaluate loop condition
		if step.Loop != nil {
			shouldLoop := true

			// Check max iterations
			if t.LoopIteration >= step.Loop.MaxIterations {
				shouldLoop = false
			}

			// Check exit condition
			if shouldLoop && step.Loop.ExitCondition != nil {
				if step.Loop.ExitCondition.StepContextEmpty != "" {
					content, _ := e.database.GetTaskStepContext(t.ID, step.Loop.ExitCondition.StepContextEmpty)
					if strings.TrimSpace(content) == "" {
						shouldLoop = false
						log.Printf("Loop exit: step context %q is empty for task #%d", step.Loop.ExitCondition.StepContextEmpty, t.ID)
					}
				}
			}

			if shouldLoop {
				targetIdx := findStepIndex(steps, step.Loop.Goto)
				t.LoopIteration++
				if err := e.database.UpdateTaskLoopIteration(t.ID, t.LoopIteration); err != nil {
					log.Printf("Warning: failed to update loop iteration: %v", err)
				}
				if err := e.database.UpdateTaskStep(t.ID, targetIdx, steps[targetIdx].Name); err != nil {
					log.Printf("Warning: failed to update task step for loop: %v", err)
				}
				log.Printf("Task #%d looping back to step %q (iteration %d/%d)", t.ID, step.Loop.Goto, t.LoopIteration, step.Loop.MaxIterations)
				i = targetIdx - 1 // for-loop will increment
				continue
			}

			// Loop done, reset counter
			log.Printf("Task #%d loop completed after %d iterations", t.ID, t.LoopIteration)
			t.LoopIteration = 0
			if err := e.database.UpdateTaskLoopIteration(t.ID, 0); err != nil {
				log.Printf("Warning: failed to reset loop iteration: %v", err)
			}
		}

		// Check if approval required before continuing
		needsApproval := step.Human || useTmux
		if needsApproval {
			// Set status to pause the task. The daemon will wait for user action.
			if err := e.database.UpdateTaskStep(t.ID, i+1, ""); err != nil {
				log.Printf("Warning: failed to update task step: %v", err)
			}
			pauseStatus := task.StatusAwaitingApproval
			if useTmux {
				pauseStatus = task.StatusTmux
			}
			if err := e.database.UpdateTaskStatus(t.ID, pauseStatus); err != nil {
				log.Printf("Warning: failed to set %s: %v", pauseStatus, err)
			}
			return nil
		}
	}

	// All steps completed — merge, summarization, and cleanup
	// are handled by the daemon on agent completion.
	return nil
}

// ErrStepContextRequired is returned by summarizePreviousTmuxStep when a step
// marked `require_context: true` fails to capture its summarize_chat context.
// Callers wrap it; the daemon checks it with errors.Is to decide whether to
// block the task (fail) instead of advancing/finalizing with an empty context.
var ErrStepContextRequired = errors.New("required step context could not be captured")

// ResumeAfterApproval resumes a task from its current step index.
// If the previously-completed step was a tmux step with summarize_chat strategy,
// summarisation runs synchronously here (it could not run during the step itself
// because tmux steps return early and pause the engine).
//
// When the previous step is marked `require_context: true` and its context
// cannot be captured, the task is NOT advanced: the error is recorded and
// returned so the agent-completion path fails the task instead of running the
// next step with an empty {{steps.<name>.context}}.
func (e *Engine) ResumeAfterApproval(ctx context.Context, t *task.Task, outputFn func([]string)) error {
	if err := e.summarizePreviousTmuxStep(ctx, t, nil); err != nil {
		e.database.UpdateTaskExitCode(t.ID, 1, err.Error())
		return err
	}
	return e.RunTask(ctx, t, outputFn)
}

// summarizePreviousTmuxStep runs summarize_chat for the step at t.StepIndex-1
// when that step is tmux with the summarize_chat strategy. No-op otherwise.
// This is the shared hook for capturing tmux step context after the engine
// has paused (since tmux steps return before their chat is meaningful):
//   - ResumeAfterApproval calls it when the user advances to the next step
//   - FinalizeTask calls it when the last step is a tmux step
//
// Best-effort by default: a failure to capture context is logged and nil is
// returned so the surrounding flow proceeds. When the step sets
// `require_context: true`, the same failure instead returns an error wrapping
// ErrStepContextRequired so the caller can block the task.
func (e *Engine) summarizePreviousTmuxStep(ctx context.Context, t *task.Task, logFn func(string, ...any)) error {
	logMsg := func(format string, args ...any) {
		log.Printf(format, args...)
		if logFn != nil {
			logFn(format, args...)
		}
	}

	wf := e.cfg.GetWorkflow(t.Workflow)
	if wf == nil || t.StepIndex <= 0 || t.StepIndex > len(wf.Steps) {
		return nil
	}
	prevStep := wf.Steps[t.StepIndex-1]
	if !prevStep.UseTmux(wf.Print) || prevStep.EffectiveSummarizationStrategy() != config.SummarizationStrategySummarizeChat {
		return nil
	}

	// If the step's context was already published manually — the agent folded
	// its chat into the step context via the update_step_context MCP tool — that
	// is the canonical artifact and must not be clobbered by the auto
	// summarizer. Folding manually is the whole point of update_step_context for
	// tmux steps, so respecting an existing context is the expected behaviour
	// (and it satisfies require_context: the context IS captured).
	if existing, ctxErr := e.database.GetTaskStepContext(t.ID, prevStep.Name); ctxErr == nil && strings.TrimSpace(existing) != "" {
		logMsg("Step %q of task #%d already has a manually-folded context (%d chars); skipping summarize_chat", prevStep.Name, t.ID, len(existing))
		return nil
	}

	// fail records a context-capture failure. When the step demands context it
	// returns a blocking error (wrapping ErrStepContextRequired); otherwise it
	// logs a warning and returns nil so the flow proceeds best-effort.
	fail := func(format string, args ...any) error {
		if prevStep.RequireContext {
			return fmt.Errorf("step %q: %w: "+format, append([]any{prevStep.Name, ErrStepContextRequired}, args...)...)
		}
		logMsg("Warning: "+format, args...)
		return nil
	}

	chat, err := e.loadStepChatContent(t, prevStep.Name, true)
	if err != nil {
		return fail("failed to load chat for tmux step %q of task #%d: %v", prevStep.Name, t.ID, err)
	}
	if chat == "" {
		// The step completed but yielded no chat content to summarize, so its
		// context stays empty — and downstream steps that template
		// {{steps.<name>.context}} will silently receive nothing. loadStepChatContent
		// logs the specific cause (missing JSONL, no recorded session) below.
		return fail("tmux step %q of task #%d produced no chat content; its step context will be empty", prevStep.Name, t.ID)
	}
	// Surface the step summarization phase via the task status so
	// the TUI can distinguish it from regular step execution.
	restore := e.markSummarizingStep(t, wf)
	summary, err := e.summarizeChatLog(ctx, t, prevStep.Name, prevStep.SummarizationPrompt, chat, prevStep.EffectiveAllowedSummarizationModels(e.cfg.AllowedSummarizationModels))
	restore()
	if err != nil {
		return fail("summarize_chat failed for tmux step %q of task #%d: %v", prevStep.Name, t.ID, err)
	}
	if summary == "" {
		return fail("summarize_chat produced an empty summary for tmux step %q of task #%d", prevStep.Name, t.ID)
	}
	if err := e.database.UpdateTaskStepContext(t.ID, prevStep.Name, summary); err != nil {
		return fail("failed to write step context for tmux step %q of task #%d: %v", prevStep.Name, t.ID, err)
	}
	logMsg("summarize_chat updated step context for tmux step %q of task #%d (%d chars)", prevStep.Name, t.ID, len(summary))
	return nil
}

// loadAndClearChildren loads all task_waits_on children for t, converts them
// into a ChildrenVars map keyed by child ID, then deletes the wait-on edges
// from the DB. Returns an empty (but non-nil) map on first-time step runs (no
// edges). The clear-on-read pattern ensures the post-step check sees only
// NEW edges added during this run.
func (e *Engine) loadAndClearChildren(t *task.Task) (ChildrenVars, error) {
	children, err := e.database.GetWaitsOnChildren(t.ID)
	if err != nil {
		return ChildrenVars{ByID: map[int64]ChildVars{}}, err
	}
	out := ChildrenVars{ByID: make(map[int64]ChildVars, len(children))}
	if len(children) == 0 {
		return out, nil
	}
	for _, c := range children {
		out.ByID[c.ID] = ChildVars{
			ID:      c.ID,
			Title:   c.Title,
			Status:  string(c.Status),
			Context: c.Context,
		}
	}
	if err := e.database.RemoveAllTaskWaitsOn(t.ID); err != nil {
		log.Printf("Warning: failed to clear waits-on for task #%d after resume: %v", t.ID, err)
	}
	return out, nil
}

// markSummarizingStep transitions the task to the appropriate "summarizing"
// status and returns a function that restores the previous status. Used to
// surface the step summarization phase in the TUI without persisting it as
// the resting status of the task.
//
// For single-step workflows the step summary IS the task summary (FinalizeTask
// promotes it directly into task.context, skipping the cross-step summarizer),
// so the canonical StatusSummarizing is used. Multi-step workflows use
// StatusSummarizingStep to distinguish per-step summarization from the final
// cross-step summarizer.
//
// On any DB failure the function is a no-op (logged) — step summarization is
// a best-effort phase and must not block forward progress.
func (e *Engine) markSummarizingStep(t *task.Task, wf *config.WorkflowConfig) func() {
	prev := t.Status
	status := task.StatusSummarizingStep
	if wf != nil && len(wf.Steps) == 1 {
		status = task.StatusSummarizing
	}
	if err := e.database.UpdateTaskStatus(t.ID, status); err != nil {
		log.Printf("Warning: failed to set %s status for task #%d: %v", status, t.ID, err)
		return func() {}
	}
	t.Status = status
	return func() {
		if err := e.database.UpdateTaskStatus(t.ID, prev); err != nil {
			log.Printf("Warning: failed to restore status %q for task #%d after step summarization: %v", prev, t.ID, err)
			return
		}
		t.Status = prev
	}
}
