package workflow

import (
	"errors"
	"strings"
	"testing"

	"github.com/Bakaface/sortie/internal/task"
)

func makeLookup(tasks map[int64]*task.Task) func(int64) (*task.Task, error) {
	return func(id int64) (*task.Task, error) {
		t, ok := tasks[id]
		if !ok {
			return nil, errors.New("not found")
		}
		return t, nil
	}
}

func TestResolveTemplate_TaskRefFields(t *testing.T) {
	tasks := map[int64]*task.Task{
		42: {
			ID:          42,
			Title:       "Refactor parser",
			Branch:      "sortie/42-refactor",
			Description: "Pull tokenizer out of parser.go",
			Context:     "Done — landed in commit abc123.",
		},
	}

	cases := []struct {
		name  string
		tmpl  string
		want  string
	}{
		{"title", "title={{tasks.42.title}}", "title=Refactor parser"},
		{"branch", "branch={{tasks.42.branch}}", "branch=sortie/42-refactor"},
		{"description", "desc={{tasks.42.description}}", "desc=Pull tokenizer out of parser.go"},
		{"context", "ctx={{tasks.42.context}}", "ctx=Done — landed in commit abc123."},
	}

	ctx := &TemplateContext{TaskLookup: makeLookup(tasks)}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveTemplate(tc.tmpl, ctx)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveTemplate_TaskContextField(t *testing.T) {
	// Direct {{task.context}} on the current task — fixes prior README drift.
	ctx := &TemplateContext{
		Task: TaskVars{Context: "prior summary text"},
	}
	got := ResolveTemplate("ctx={{task.context}}", ctx)
	if got != "ctx=prior summary text" {
		t.Errorf("got %q", got)
	}
}

func TestResolveTemplate_MissingTaskID(t *testing.T) {
	ctx := &TemplateContext{TaskLookup: makeLookup(nil)}
	got := ResolveTemplate("x={{tasks.99.title}}", ctx)
	if got != "x=" {
		t.Errorf("missing id should resolve empty, got %q", got)
	}
}

func TestResolveTemplate_NilLookup(t *testing.T) {
	ctx := &TemplateContext{}
	got := ResolveTemplate("x={{tasks.5.title}}", ctx)
	if got != "x=" {
		t.Errorf("nil lookup should resolve empty, got %q", got)
	}
}

func TestResolveTemplate_MalformedRef(t *testing.T) {
	ctx := &TemplateContext{TaskLookup: makeLookup(nil)}
	cases := []string{
		"{{tasks.abc.title}}",
		"{{tasks.42}}",
		"{{tasks.}}",
	}
	for _, c := range cases {
		got := ResolveTemplate(c, ctx)
		if got != c {
			t.Errorf("malformed ref %q should stay verbatim, got %q", c, got)
		}
	}
}

func TestResolveTemplate_UnsupportedField(t *testing.T) {
	tasks := map[int64]*task.Task{1: {ID: 1, Title: "t"}}
	ctx := &TemplateContext{TaskLookup: makeLookup(tasks)}
	// Unsupported field resolves to "" (validator is the user-facing gate).
	got := ResolveTemplate("x={{tasks.1.slug}}", ctx)
	if got != "x=" {
		t.Errorf("unsupported field should resolve empty, got %q", got)
	}
}

func TestResolveTemplate_SelfReference(t *testing.T) {
	// A task can reference itself; it resolves to its own current field value.
	self := &task.Task{ID: 7, Title: "Self-task", Description: "self desc"}
	ctx := &TemplateContext{
		Task:       TaskVars{ID: 7, Title: "Self-task", Description: "self desc"},
		TaskLookup: makeLookup(map[int64]*task.Task{7: self}),
	}
	got := ResolveTemplate("{{tasks.7.title}}", ctx)
	if got != "Self-task" {
		t.Errorf("self ref got %q", got)
	}
}

func TestResolveTaskRefs_EndToEnd(t *testing.T) {
	// Simulates the engine's pre-expansion: {{task.description}} contains
	// {{tasks.42.title}}, and after pre-resolution the step prompt
	// "{{task.description}}" expands to the full text.
	tasks := map[int64]*task.Task{
		42: {ID: 42, Title: "Underlying refactor"},
	}
	rawDescription := "Build on top of: {{tasks.42.title}}"
	resolvedDescription := ResolveTaskRefs(rawDescription, makeLookup(tasks))
	if resolvedDescription != "Build on top of: Underlying refactor" {
		t.Fatalf("pre-resolution failed: %q", resolvedDescription)
	}

	ctx := &TemplateContext{
		Task: TaskVars{Description: resolvedDescription},
		// TaskLookup intentionally left nil here — pre-resolution should make
		// the final ResolveTemplate pass independent of the lookup table.
	}
	final := ResolveTemplate("{{task.description}}", ctx)
	if final != "Build on top of: Underlying refactor" {
		t.Errorf("final template got %q", final)
	}
}

func TestResolveTaskRefs_NoOpWhenNoMatches(t *testing.T) {
	got := ResolveTaskRefs("plain text", makeLookup(nil))
	if got != "plain text" {
		t.Errorf("expected pass-through, got %q", got)
	}
}

func TestExtractTaskRefs(t *testing.T) {
	in := "see {{tasks.42.title}} and {{tasks.7.branch}}; also {{task.id}} and {{tasks.abc.title}} and {{tasks.5.foo.bar}}"
	refs := ExtractTaskRefs(in)
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d: %+v", len(refs), refs)
	}
	if refs[0].ID != 42 || refs[0].Field != "title" {
		t.Errorf("ref 0: %+v", refs[0])
	}
	if refs[1].ID != 7 || refs[1].Field != "branch" {
		t.Errorf("ref 1: %+v", refs[1])
	}
}

func TestValidateTaskRefs(t *testing.T) {
	good := []TaskRef{
		{ID: 1, Field: "title", Raw: "{{tasks.1.title}}"},
		{ID: 2, Field: "context", Raw: "{{tasks.2.context}}"},
	}
	if err := ValidateTaskRefs(good); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	bad := []TaskRef{{ID: 1, Field: "slug", Raw: "{{tasks.1.slug}}"}}
	err := ValidateTaskRefs(bad)
	if err == nil {
		t.Fatal("expected error for unsupported field")
	}
	if !strings.Contains(err.Error(), "slug") {
		t.Errorf("error should mention the bad field name: %v", err)
	}
	if !strings.Contains(err.Error(), "title") {
		t.Errorf("error should list supported fields: %v", err)
	}
}
