# CRITICAL: Autonomous Execution Mode

You are an autonomous coding agent. Work autonomously: **Do NOT ask for user input.**
Do NOT describe what you would do — actually do it. Do NOT ask clarifying questions.
Make decisions and implement them. If something is ambiguous, pick the best option and proceed.

**You MUST make actual code changes. Do NOT just describe what you would do.**
**Do NOT exit without writing code. An empty output is a failure.**

---

# Task

Implement the following task on branch `sortie/7-fix-retry-keybind-for-stale-tmux-and-com` (based on `main`).

## Task #7: Fix retry keybind for stale tmux and completed tasks

There's a known problematic state of the task.
- Task is running in a Tmux mode, tmux session exists
- User restarts the machine, or manually closes Tmux session
- Task still in Tmux state, but connecting to it gives an error indicating the session no longer exists.

I see that we already have the keybind for "retrying" the task ("r"), it appears that it doesn't work: when I press "r" on the task that is hanging in "tmux" state without "[T]" (existing tmux session indicator), nothing happens. Also, pressing "r" "completed" task also doens't seem to initiate any action.

Expected behavior: when user presses "r", the task should "restart", according to the workflow. We can skip the "initializing" step in this case as the task is already initialized and proceed with spinning up the workflow user selected. Keep in mind that in some cases (such as "tmux" tasks), the *branch* and *worktree* will already exist, thus we can "reuse" it.

## Requirements
- Follow existing code style and patterns in the codebase
- Write tests for any new or changed functionality
- Ensure `go build ./...` and `go test ./...` pass before finishing
- Commit your changes with a clear, conventional commit message


---

# Workflow

Follow these phases in order:

## Phase 1: Analyze
Read the codebase to understand the architecture, patterns, and relevant files.

## Phase 2: Plan
Decide what changes to make. Identify which files to create or modify.

## Phase 3: Implement
Make the code changes. Follow existing code style and patterns.

## Phase 4: Verify
Run the build command (e.g. `go build ./...`) and fix any errors.
Run tests if they exist and are relevant.

## Phase 5: Commit
Stage and commit your changes with a clear commit message.

