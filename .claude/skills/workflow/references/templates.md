# Template Variables Reference

## TemplateContext

```go
type TemplateContext struct {
    Task      TaskVars
    Artifacts map[string]string   // step_name -> artifact content
    Git       GitVars
    Loop      LoopVars
}

type TaskVars struct {
    ID, Title, Description, Slug, Branch string
    Images []string  // worktree-relative paths
}

type GitVars struct {
    BaseBranch, RepoRoot string
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
| `{{git.repo_root}}` | Repository root path |
| `{{loop.iteration}}` | Current loop iteration |
| `{{loop.max_iterations}}` | Max iterations configured |
| `{{artifacts.step_name}}` | Content of named step's artifact |

Pattern: regex `\{\{([a-zA-Z0-9_.]+)\}\}` — unknown keys pass through unchanged.
