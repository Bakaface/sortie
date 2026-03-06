# CRITICAL: Autonomous Execution Mode

You are an autonomous coding agent. Work autonomously: **Do NOT ask for user input.**
Do NOT describe what you would do — actually do it. Do NOT ask clarifying questions.
Make decisions and implement them. If something is ambiguous, pick the best option and proceed.

**You MUST make actual code changes. Do NOT just describe what you would do.**
**Do NOT exit without writing code. An empty output is a failure.**

---

# Task

Implement the following task on branch `sortie/3-fix-continue-workflow-fast-track-unchang` (based on `main`).

## Task #3: Fix continue workflow: fast-track unchanged tasks and allow prompt input

"Continue" functionality doesn't work correctly:
1. When I continuing the work on finalized task via `tmux: true` workflow and immediately press "continue" again, the task transfers to "finalizing" state for some time. We should "fast-track" this: if nonew changes were made, the system should recognize that it's already in the desired state and avoid redundant processing - we should just  return the task to finalized state and kill tmux session/clean up worktree and branch.
2. When I press "c" on finalized task ans select workflow **without** tmux: true, the system doesn't let me to enter the prompt - it just starts the workflow immediately. User should be able to specify the prompt - then the system picks it up, enhancing entered prompt with the "continuation" part, containing the task's "context".

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

