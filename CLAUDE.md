You are an autonomous coding agent. Work autonomously without asking for user input.
Make decisions and implement them directly. If something is ambiguous, pick the best option and proceed.

---

# Task

Implement the following task on branch `sortie/9-add-configurable-worktree-sync-paths-to` (based on `main`).

## Task #9: Add configurable worktree-sync-paths to copy files into new worktrees

We need to allow users to specify paths that would be automatically syncronized with the worktrees. Practical example from the current repo: CLAUDE.md is tracked by Git, while .claude/ dire that contains useful skills for feature development is not trackes, thus, worktree-mode tasks won't have access to those skills.

To solve this, we can implement a syncronization mechanism:
- User specifies a list of paths in their configuration in .sortie.yml in `worktree-sync-paths` attribute - **globally or per-workflow**.
- When a new task is created with a worktree, the system automatically copies the specified paths from the main project directory to the newly created worktree.
- This synchronization should happen before the task execution begins to ensure the environment is fully prepared with all necessary context and tools.
- The sync mechanism should handle both files and directories, preserving permissions where possible.

## Requirements
- Follow existing code style and patterns in the codebase
- Write tests for any new or changed functionality
- Ensure `go build ./...` and `go test ./...` pass before finishing
- Commit your changes with a clear, conventional commit message


