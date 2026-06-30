//go:build e2e

package e2e

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// pinnedWorkflowYAML returns a fully-pinned workflow: it pins the description,
// the worktree toggle, a new-branch template, and the target branch. Such a
// workflow can be launched without any New Task input (the TUI skips the screen;
// the CLI accepts `create -w pinned` with no description argument).
func pinnedWorkflowYAML(stubPath string) string {
	return fmt.Sprintf(`claude:
  command: %s
poll_interval: 100ms
git:
  base_branch: main
on_complete: merge
workflows:
  - name: pinned
    description: "Pinned task description"
    worktree: true
    branch: "sortie/pinned-{{task_id}}"
    target: main
    print: true
    steps:
      - name: implementing
        prompt: "Implement the task"
`, stubPath)
}

// TestWorkflowPinsSkipPath runs a fully-pinned workflow created WITHOUT a
// description argument and asserts:
//   - the task is accepted (the pinned description satisfies the empty-description gate)
//   - the created task carries the workflow's pinned description and branch
//   - the task reaches "completed" and merges to main
func TestWorkflowPinsSkipPath(t *testing.T) {
	e := setupE2E(t, "workflow_pins")
	e.WriteSortieYAML(pinnedWorkflowYAML(e.StubPath))

	// No description argument — the workflow pins it. (--title is supplied only
	// to skip the title-refinement round-trip, matching the other e2e scenarios;
	// the description still comes entirely from the workflow pin.)
	e.MustSortie("create", "--title", "pinned run", "--workflow", "pinned")

	// The pinned description and branch template are applied at creation time.
	if got := e.TaskField(1, "description"); got != "Pinned task description" {
		t.Errorf("pinned description: got %q, want %q", got, "Pinned task description")
	}
	if got := e.TaskField(1, "branch_name"); got != "sortie/pinned-{{task_id}}" {
		t.Errorf("pinned branch template: got %q, want %q", got, "sortie/pinned-{{task_id}}")
	}

	e.WaitStatus(1, "completed", 10*time.Second)
	e.AssertMergedFor(1)

	calls := e.StubCalls("step")
	if len(calls) != 1 {
		t.Errorf("stub step calls: got %d, want 1", len(calls))
	}
}

// noWorktreePinnedYAML returns a workflow that pins worktree:false (plus the
// description, so the empty-description gate is satisfied). branch/checkout/target
// are intentionally absent — they are invalid when worktree is false.
func noWorktreePinnedYAML(stubPath string) string {
	return fmt.Sprintf(`claude:
  command: %s
poll_interval: 100ms
git:
  base_branch: main
on_complete: merge
workflows:
  - name: inplace
    description: "In-place task description"
    worktree: false
    print: true
    steps:
      - name: implementing
        prompt: "Implement the task"
`, stubPath)
}

// partialPinWorkflowYAML returns a partially-pinned workflow: only the
// description and target branch are pinned. worktree and branch are left to
// the project defaults / auto-generation. The CLI must still accept invocation
// without a description argument (the empty-description gate falls back to the
// workflow's pinned description), and the daemon-side precedence chain must
// supply the project default worktree (true) when the workflow leaves it open.
func partialPinWorkflowYAML(stubPath string) string {
	return fmt.Sprintf(`claude:
  command: %s
poll_interval: 100ms
git:
  base_branch: main
on_complete: merge
workflows:
  - name: partial
    description: "Partially pinned description"
    target: main
    print: true
    steps:
      - name: implementing
        prompt: "Implement the task"
`, stubPath)
}

// TestWorkflowPinsPartial verifies that a workflow pinning only some New Task
// fields (description + target here) still accepts launch without an explicit
// description and applies the pins to the resulting task — while leaving
// unpinned fields (worktree, branch) to the project defaults / auto-resolution.
func TestWorkflowPinsPartial(t *testing.T) {
	e := setupE2E(t, "workflow_pins")
	e.WriteSortieYAML(partialPinWorkflowYAML(e.StubPath))

	// No `-d` — only the pinned description should reach the task.
	e.MustSortie("create", "--title", "partial run", "--workflow", "partial")

	if got := e.TaskField(1, "description"); got != "Partially pinned description" {
		t.Errorf("pinned description: got %q, want %q", got, "Partially pinned description")
	}
	if got := e.TaskField(1, "target_branch"); got != "main" {
		t.Errorf("pinned target: got %q, want %q", got, "main")
	}
	// Worktree was unpinned → falls back to the project default (true).
	if got := e.TaskField(1, "worktree"); got != "true" {
		t.Errorf("unpinned worktree should default to true: got %q", got)
	}

	e.WaitStatus(1, "completed", 10*time.Second)
	e.AssertMergedFor(1)
}

// TestWorkflowPinsWorktreeFalse exercises the worktree:false pin end-to-end: the
// task must run in the project root (no git worktree, no branch), and on_complete
// must be a no-op (a non-worktree task shares the user's working tree, so sortie
// leaves it as-is — nothing is committed or merged).
func TestWorkflowPinsWorktreeFalse(t *testing.T) {
	e := setupE2E(t, "workflow_pins") // reuses the "implementing" step stub fixtures
	e.WriteSortieYAML(noWorktreePinnedYAML(e.StubPath))

	e.MustSortie("create", "--title", "in-place run", "--workflow", "inplace")

	// Pinned worktree:false → task carries worktree=false and never gets a branch.
	if got := e.TaskField(1, "worktree"); got != "false" {
		t.Errorf("pinned worktree: got %q, want %q", got, "false")
	}
	if got := e.TaskField(1, "branch"); got != "" {
		t.Errorf("non-worktree task should have no branch: got %q, want empty", got)
	}

	e.WaitStatus(1, "completed", 10*time.Second)

	// The step agent ran in the project root, not a worktree.
	calls := e.StubCalls("step")
	if len(calls) != 1 {
		t.Fatalf("stub step calls: got %d, want 1", len(calls))
	}
	if strings.Contains(calls[0].CWD, "/worktrees/") {
		t.Errorf("non-worktree step ran under a worktree dir: CWD=%q", calls[0].CWD)
	}

	// on_complete must NOT commit or merge for a non-worktree task — main stays at
	// the two setup commits ("initial" + "add .sortie.yml").
	logOut := gitInDir(e.ProjectDir, "log", "main", "--oneline")
	if n := len(strings.Split(strings.TrimSpace(logOut), "\n")); n != 2 {
		t.Errorf("main commit count: got %d, want 2 (non-worktree on_complete must be a no-op):\n%s", n, logOut)
	}
}
