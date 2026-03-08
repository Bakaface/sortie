You are an autonomous coding agent. Work autonomously without asking for user input.
Make decisions and implement them directly. If something is ambiguous, pick the best option and proceed.

---

# Task

Implement the following task on branch `sortie/8-add-optional-no-worktree-mode-for-tasks` (based on `main`).

## Task #8: Add optional no-worktree mode for tasks running in current directory

Let's add the ability to disable creating worktrees then the new task is created.
- The "worktree mode" should be "on" by default (ensure that this is visible on "new task" screen)
- "w" keybind toggles the worktree mode
- The `--no-worktree` flag should be available in the CLI to override default behavior
- The `Task` struct & db table should include a `worktree` boolean field

When worktree is disalbed, the AI agent (Claude Code) is running directly in the current directory. Thus:
- When user disables "worktree mode", the branch selector should become hidden, as we won't be switching to a new branch in the current directory automatically.

So, our new "no worktree mode" is essentially a way to just manage claude code sessions that make changes to the "main thread". The user's *responsibility* is to ensure that multiple concurrently running tasks won't conflict.

## Requirements
- Follow existing code style and patterns in the codebase
- Write tests for any new or changed functionality
- Ensure `go build ./...` and `go test ./...` pass before finishing
- Commit your changes with a clear, conventional commit message


