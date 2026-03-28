package workflow

import (
	"fmt"
	"regexp"
	"strings"
)

type TaskVars struct {
	ID          int64
	Title       string
	Description string
	Slug        string
	Branch      string
	Images      []string // worktree-relative paths to attached images
}

type GitVars struct {
	BaseBranch   string
	TargetBranch string
	RepoRoot     string
}

type LoopVars struct {
	Iteration     int
	MaxIterations int
}

type TemplateContext struct {
	Task  TaskVars
	Steps map[string]string // step name -> result text from DB
	Git   GitVars
	Loop  LoopVars
}

var templatePattern = regexp.MustCompile(`\{\{([a-zA-Z0-9_.]+)\}\}`)

// ResolveTemplate replaces {{dotted.path}} placeholders in the template string.
func ResolveTemplate(tmpl string, ctx *TemplateContext) string {
	return templatePattern.ReplaceAllStringFunc(tmpl, func(match string) string {
		key := match[2 : len(match)-2] // strip {{ and }}

		switch {
		case key == "task.id":
			return fmt.Sprintf("%d", ctx.Task.ID)
		case key == "task.title":
			return ctx.Task.Title
		case key == "task.description":
			return ctx.Task.Description
		case key == "task.slug":
			return ctx.Task.Slug
		case key == "task.branch":
			return ctx.Task.Branch
		case key == "task.images":
			return strings.Join(ctx.Task.Images, "\n")
		case key == "git.base_branch":
			return ctx.Git.BaseBranch
		case key == "git.target_branch":
			return ctx.Git.TargetBranch
		case key == "git.repo_root":
			return ctx.Git.RepoRoot
		case key == "loop.iteration":
			return fmt.Sprintf("%d", ctx.Loop.Iteration)
		case key == "loop.max_iterations":
			return fmt.Sprintf("%d", ctx.Loop.MaxIterations)
		case strings.HasPrefix(key, "steps."):
			// Format: steps.<name>.context
			rest := key[len("steps."):]
			stepName := strings.TrimSuffix(rest, ".context")
			if val, ok := ctx.Steps[stepName]; ok {
				return val
			}
			return ""
		case strings.HasPrefix(key, "artifacts."):
			// Backward compatibility: artifacts.<name> still works
			artifactName := key[len("artifacts."):]
			if val, ok := ctx.Steps[artifactName]; ok {
				return val
			}
			return ""
		default:
			return match // leave unknown placeholders as-is
		}
	})
}
