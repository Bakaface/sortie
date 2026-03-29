# internal/workflow — Task Execution Engine

Step execution, template resolution, system prompt injection, merge/conflict handling. Load `/workflow` skill before making substantial changes.

## Critical Invariants

- **Step context captured from Claude's NDJSON result event, stored in DB after each step** — synchronous capture; step index persisted
- **Merge mutex (`mergeMu`) must serialize all merge operations** — even concurrent tasks in different worktrees of the same repo
- **Non-worktree mode skips branch creation, uses project root** — `on_complete` falls back to `Commit()` instead of merge
