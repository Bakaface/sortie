package workflow

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aface/sortie/internal/config"
	"github.com/aface/sortie/internal/db"
	gitpkg "github.com/aface/sortie/internal/git"
	"github.com/aface/sortie/internal/notify"
	"github.com/aface/sortie/internal/task"
)

const (
	// maxMergeAttempts is the number of times to retry a squash-merge before failing.
	maxMergeAttempts = 3

	// mergeBlockedPollInterval is how often to re-check whether the target branch is clean.
	mergeBlockedPollInterval = 10 * time.Second

	// processExitPollInterval is how often to check whether a Claude subprocess has exited.
	processExitPollInterval = 500 * time.Millisecond
)

type Engine struct {
	cfg      *config.Config
	database *db.DB
	notifier *notify.Notifier
	repoRoot string
	dataDir  string
	mergeMu  *sync.Mutex // serializes merge operations to prevent concurrent git merge conflicts
}

// NewEngine creates a new workflow engine. The mergeMu parameter is an optional
// externally-owned mutex that serializes merge operations for this repo. When
// non-nil, the mutex survives config reloads (the daemon owns it). When nil, a
// fresh mutex is created (for standalone/test usage).
func NewEngine(cfg *config.Config, database *db.DB, notifier *notify.Notifier, repoRoot string, mergeMu ...*sync.Mutex) *Engine {
	var mu *sync.Mutex
	if len(mergeMu) > 0 && mergeMu[0] != nil {
		mu = mergeMu[0]
	} else {
		mu = &sync.Mutex{}
	}
	return &Engine{
		cfg:      cfg,
		database: database,
		notifier: notifier,
		repoRoot: repoRoot,
		dataDir:  filepath.Join(repoRoot, ".sortie"),
		mergeMu:  mu,
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
		// No-worktree mode: run in project root directory
		t.WorktreePath = e.repoRoot
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

		// Record step start in task_steps table
		if err := e.database.CreateTaskStep(t.ID, step.Name); err != nil {
			log.Printf("Warning: failed to create task step record: %v", err)
		}

		// Collect step contexts from prior completed steps
		var priorStepNames []string
		for j := 0; j < i; j++ {
			priorStepNames = append(priorStepNames, steps[j].Name)
		}
		stepContexts, err := e.database.GetTaskStepContexts(t.ID, priorStepNames)
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
		tmplCtx := &TemplateContext{
			Task: TaskVars{
				ID:          t.ID,
				Title:       t.Title,
				Description: t.Description,
				Slug:        t.Slug,
				Branch:      t.Branch,
				Images:      imageRelPaths,
			},
			Steps: stepContexts,
			Git: GitVars{
				BaseBranch:   e.cfg.Git.BaseBranch,
				TargetBranch: e.effectiveBaseBranch(t),
				RepoRoot:     e.repoRoot,
			},
			Loop: loopVars,
		}

		resolvedPrompt := ResolveTemplate(step.Prompt, tmplCtx)

		sysPrompt := BuildSystemPrompt(resolvedPrompt, e.cfg.SystemPrompt, imageRelPaths)

		// Set environment variables
		env := map[string]string{
			"SORTIE_TASK_ID":  fmt.Sprintf("%d", t.ID),
			"SORTIE_STEP":     step.Name,
			"SORTIE_WORKTREE": t.WorktreePath,
		}

		// Spawn Claude process (tmux or direct)
		useTmux := step.UseTmux(wf.Tmux)
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

		// Validate that the step produced meaningful changes
		// Skip for review steps and tmux steps (agent may still be working)
		if !step.Human && !useTmux {
			noiseFiles := []string{".claude-output.log"}
			hasChanges, err := gitpkg.HasMeaningfulChanges(t.WorktreePath, noiseFiles)
			if err != nil {
				log.Printf("Warning: failed to check for changes in step %q: %v", step.Name, err)
			} else if !hasChanges {
				errMsg := fmt.Sprintf("step %q exited successfully but produced no code changes", step.Name)
				e.database.UpdateTaskExitCode(t.ID, 1, errMsg)
				return errors.New(errMsg)
			}
		}

		// Record step completion with context.
		// For summarize_chat, store last_message immediately and kick off
		// background summarization that will overwrite the context when done.
		stepContextText := resultText
		var ctxPtr *string
		if stepContextText != "" {
			ctxPtr = &stepContextText
		}
		if err := e.database.CompleteTaskStep(t.ID, step.Name, ctxPtr, exitCode); err != nil {
			log.Printf("Warning: failed to complete task step record: %v", err)
		}

		if step.SummarizationStrategy == config.SummarizationStrategySummarizeChat {
			stepName := step.Name
			taskID := t.ID
			taskCopy := *t
			logPath := ProjectLogPath(e.dataDir, taskID, stepName)
			go func() {
				bgCtx := context.Background()
				summary, err := e.summarizeChatLog(bgCtx, &taskCopy, stepName, logPath)
				if err != nil {
					log.Printf("Warning: summarize_chat failed for step %q of task #%d: %v", stepName, taskID, err)
					return
				}
				if summary == "" {
					return
				}
				if err := e.database.UpdateTaskStepContext(taskID, stepName, summary); err != nil {
					log.Printf("Warning: failed to update step context after summarize_chat for step %q of task #%d: %v", stepName, taskID, err)
				} else {
					log.Printf("summarize_chat updated step context for step %q of task #%d (%d chars)", stepName, taskID, len(summary))
				}
			}()
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

// ResumeAfterApproval resumes a task from its current step index.
func (e *Engine) ResumeAfterApproval(ctx context.Context, t *task.Task, outputFn func([]string)) error {
	return e.RunTask(ctx, t, outputFn)
}
