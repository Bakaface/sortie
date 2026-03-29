# Template Variables Reference

## TemplateContext

```go
type TemplateContext struct {
    Task      TaskVars
    Steps     map[string]string   // step_name -> step context (from DB task_steps table)
    Git       GitVars
    Loop      LoopVars
}

type TaskVars struct {
    ID          int64
    Title, Description, Slug, Branch string
    Images []string  // worktree-relative paths
}

type GitVars struct {
    BaseBranch, TargetBranch, RepoRoot string
}

type LoopVars struct {
    Iteration, MaxIterations int
}
```

## Supported Placeholders

| Placeholder | Source |
|-------------|--------|
| `{{task.id}}` | Task ID |
| `{{task.title}}` | Task title |
| `{{task.description}}` | Task description |
| `{{task.slug}}` | URL-safe slug from title |
| `{{task.branch}}` | Resolved branch name |
| `{{task.images}}` | Newline-joined image paths |
| `{{git.base_branch}}` | Base branch (e.g., main) |
| `{{git.target_branch}}` | Effective target branch (per-task override or base branch) |
| `{{git.repo_root}}` | Repository root path |
| `{{loop.iteration}}` | Current loop iteration |
| `{{loop.max_iterations}}` | Max iterations configured |
| `{{steps.step_name.context}}` | Step context from DB (captured from Claude's `result` event) |
| `{{artifacts.step_name}}` | Backward compat alias for `{{steps.step_name.context}}` |

Pattern: regex `\{\{([a-zA-Z0-9_.]+)\}\}` — unknown keys pass through unchanged.
