package workflow

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/aface/sortie/internal/task"
)

type TaskVars struct {
	ID          int64
	Title       string
	Description string
	Context     string
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
	// TaskLookup resolves a task by ID for {{tasks.<id>.<field>}} references.
	// When nil, such references resolve to "".
	TaskLookup func(int64) (*task.Task, error)
}

var templatePattern = regexp.MustCompile(`\{\{([a-zA-Z0-9_.]+)\}\}`)

// supportedTaskRefFields lists the fields valid in {{tasks.<id>.<field>}} refs.
var supportedTaskRefFields = []string{"title", "branch", "description", "context"}

// TaskRef captures a parsed {{tasks.<id>.<field>}} reference.
type TaskRef struct {
	ID    int64
	Field string
	// Raw is the original placeholder text including braces, e.g. "{{tasks.42.title}}".
	Raw string
}

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
		case key == "task.context":
			return ctx.Task.Context
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
		case strings.HasPrefix(key, "tasks."):
			return resolveTaskRef(key, match, ctx)
		default:
			return match // leave unknown placeholders as-is
		}
	})
}

// resolveTaskRef handles the tasks.<id>.<field> placeholder form. Malformed
// references are left verbatim. Missing tasks or lookup errors resolve to "" and
// emit a warning log line. Unsupported field names resolve to "" (validation at
// create/edit time is the user-facing gate).
func resolveTaskRef(key, match string, ctx *TemplateContext) string {
	rest := key[len("tasks."):]
	parts := strings.SplitN(rest, ".", 2)
	if len(parts) != 2 {
		return match
	}
	idStr, field := parts[0], parts[1]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return match
	}
	if !isSupportedTaskRefField(field) {
		log.Printf("template: unsupported task ref field %q in %s", field, match)
		return ""
	}
	if ctx.TaskLookup == nil {
		return ""
	}
	t, err := ctx.TaskLookup(id)
	if err != nil || t == nil {
		log.Printf("template: failed to resolve %s: %v", match, err)
		return ""
	}
	switch field {
	case "title":
		return t.Title
	case "branch":
		return t.Branch
	case "description":
		return t.Description
	case "context":
		return t.Context
	default:
		// Unreachable: gated by isSupportedTaskRefField above.
		return ""
	}
}

func isSupportedTaskRefField(field string) bool {
	for _, f := range supportedTaskRefFields {
		if f == field {
			return true
		}
	}
	return false
}

// SupportedTaskRefFields returns the list of valid fields for
// {{tasks.<id>.<field>}} references (for use in error messages).
func SupportedTaskRefFields() []string {
	out := make([]string, len(supportedTaskRefFields))
	copy(out, supportedTaskRefFields)
	return out
}

// ExtractTaskRefs scans s for all {{tasks.<id>.<field>}} placeholders and
// returns them. Malformed forms (non-numeric id, missing field, extra dots) are
// skipped. The same (id, field) pair may appear multiple times in the result if
// it occurs multiple times in s.
func ExtractTaskRefs(s string) []TaskRef {
	matches := templatePattern.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return nil
	}
	var refs []TaskRef
	for _, m := range matches {
		key := m[1]
		if !strings.HasPrefix(key, "tasks.") {
			continue
		}
		rest := key[len("tasks."):]
		parts := strings.SplitN(rest, ".", 2)
		if len(parts) != 2 {
			continue
		}
		idStr, field := parts[0], parts[1]
		// Reject if the field contains another dot — keep parsing strict.
		if strings.Contains(field, ".") {
			continue
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 {
			continue
		}
		refs = append(refs, TaskRef{ID: id, Field: field, Raw: m[0]})
	}
	return refs
}

// ValidateTaskRefs returns a descriptive error if any ref uses an unsupported
// field name. Other validation (existence, project match, status) is the
// daemon's responsibility because it requires DB access.
func ValidateTaskRefs(refs []TaskRef) error {
	for _, r := range refs {
		if !isSupportedTaskRefField(r.Field) {
			return fmt.Errorf("unsupported task ref field %q in %s (supported: %s)",
				r.Field, r.Raw, strings.Join(supportedTaskRefFields, ", "))
		}
	}
	return nil
}

// ResolveTaskRefs replaces every {{tasks.<id>.<field>}} reference in s with
// the field value of the looked-up task. Used to pre-expand a task's own
// description / context before they are inlined into a step prompt — keeps the
// substitution single-pass (no recursive expansion into other tasks' refs).
// Behaviour on lookup miss matches resolveTaskRef: empty string + warning.
func ResolveTaskRefs(s string, lookup func(int64) (*task.Task, error)) string {
	if s == "" || lookup == nil {
		return s
	}
	if !strings.Contains(s, "{{tasks.") {
		return s
	}
	ctx := &TemplateContext{TaskLookup: lookup}
	return templatePattern.ReplaceAllStringFunc(s, func(match string) string {
		key := match[2 : len(match)-2]
		if !strings.HasPrefix(key, "tasks.") {
			return match
		}
		return resolveTaskRef(key, match, ctx)
	})
}
