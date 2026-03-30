package tui

import (
	"strings"
	"testing"

	"github.com/aface/sortie/internal/daemon"
	"github.com/charmbracelet/lipgloss"
)

func TestBuildBranchTree_LinearChain(t *testing.T) {
	tasks := []daemon.TaskInfo{
		{ID: 3, Branch: "feat-c", TargetBranch: "feat-b"},
		{ID: 2, Branch: "feat-b", TargetBranch: "feat-a"},
		{ID: 1, Branch: "feat-a", TargetBranch: "main"},
	}
	ordered, entries := buildBranchTree(tasks)

	if len(ordered) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(ordered))
	}
	// Root is feat-a (targets main, which no task owns), then feat-b, then feat-c
	wantIDs := []int64{1, 2, 3}
	wantDepths := []int{0, 1, 2}
	for i, want := range wantIDs {
		if ordered[i].ID != want {
			t.Errorf("ordered[%d].ID = %d, want %d", i, ordered[i].ID, want)
		}
		if entries[i].Depth != wantDepths[i] {
			t.Errorf("entries[%d].Depth = %d, want %d", i, entries[i].Depth, wantDepths[i])
		}
	}
}

func TestBuildBranchTree_MultipleRoots(t *testing.T) {
	tasks := []daemon.TaskInfo{
		{ID: 2, Branch: "feat-b", TargetBranch: "main"},
		{ID: 1, Branch: "feat-a", TargetBranch: "main"},
	}
	ordered, entries := buildBranchTree(tasks)

	if len(ordered) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(ordered))
	}
	// Both roots, sorted by ID desc
	if ordered[0].ID != 2 || ordered[1].ID != 1 {
		t.Errorf("expected IDs [2,1], got [%d,%d]", ordered[0].ID, ordered[1].ID)
	}
	if entries[0].Depth != 0 || entries[1].Depth != 0 {
		t.Errorf("expected depths [0,0], got [%d,%d]", entries[0].Depth, entries[1].Depth)
	}
}

func TestBuildBranchTree_FanOut(t *testing.T) {
	tasks := []daemon.TaskInfo{
		{ID: 3, Branch: "feat-c", TargetBranch: "feat-a"},
		{ID: 2, Branch: "feat-b", TargetBranch: "feat-a"},
		{ID: 1, Branch: "feat-a", TargetBranch: "main"},
	}
	ordered, entries := buildBranchTree(tasks)

	if len(ordered) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(ordered))
	}
	// Root is feat-a, then children feat-c and feat-b (sorted by ID desc)
	if ordered[0].ID != 1 {
		t.Errorf("root should be ID 1, got %d", ordered[0].ID)
	}
	if entries[0].Depth != 0 {
		t.Errorf("root depth should be 0, got %d", entries[0].Depth)
	}
	// Children should be at depth 1
	if entries[1].Depth != 1 || entries[2].Depth != 1 {
		t.Errorf("children depths should be [1,1], got [%d,%d]", entries[1].Depth, entries[2].Depth)
	}
	// First child (ID 3) is not last, second child (ID 2) is last
	if entries[1].IsLast {
		t.Error("first child should not be IsLast")
	}
	if !entries[2].IsLast {
		t.Error("second child should be IsLast")
	}
}

func TestBuildBranchTree_OrphanedChildren(t *testing.T) {
	// Child targets a branch that no task owns → child becomes root
	tasks := []daemon.TaskInfo{
		{ID: 2, Branch: "feat-b", TargetBranch: "feat-a"},
		{ID: 1, Branch: "feat-c", TargetBranch: "main"},
	}
	ordered, entries := buildBranchTree(tasks)

	if len(ordered) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(ordered))
	}
	// Both are roots since feat-a doesn't exist
	if entries[0].Depth != 0 || entries[1].Depth != 0 {
		t.Errorf("expected depths [0,0], got [%d,%d]", entries[0].Depth, entries[1].Depth)
	}
}

func TestBuildBranchTree_Cycle(t *testing.T) {
	tasks := []daemon.TaskInfo{
		{ID: 2, Branch: "feat-b", TargetBranch: "feat-a"},
		{ID: 1, Branch: "feat-a", TargetBranch: "feat-b"},
	}
	ordered, entries := buildBranchTree(tasks)

	// Should not infinite loop; both tasks should appear
	if len(ordered) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(ordered))
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestBuildBranchTree_Empty(t *testing.T) {
	ordered, entries := buildBranchTree(nil)
	if len(ordered) != 0 || entries != nil {
		t.Error("expected empty results for nil input")
	}
}

func TestBuildBranchTree_SingleTask(t *testing.T) {
	tasks := []daemon.TaskInfo{
		{ID: 1, Branch: "feat-a", TargetBranch: "main"},
	}
	ordered, entries := buildBranchTree(tasks)

	if len(ordered) != 1 {
		t.Fatalf("expected 1 task, got %d", len(ordered))
	}
	if entries[0].Depth != 0 {
		t.Errorf("single task depth should be 0, got %d", entries[0].Depth)
	}
}

func TestRenderBranchColumn_Depth0(t *testing.T) {
	e := treeEntry{Depth: 0}
	result := e.renderBranchColumn("my-branch", 20)
	if len([]rune(result)) != 20 {
		t.Errorf("expected width 20, got %d: %q", len([]rune(result)), result)
	}
	if !strings.HasPrefix(result, "my-branch") {
		t.Errorf("expected branch name at start, got %q", result)
	}
}

func TestRenderBranchColumn_Depth1Last(t *testing.T) {
	e := treeEntry{Depth: 1, IsLast: true, Ancestors: []bool{false}}
	result := e.renderBranchColumn("child-branch", 20)
	if !strings.HasPrefix(result, "  └─") {
		t.Errorf("expected '  └─' prefix, got %q", result)
	}
	if len([]rune(result)) != 20 {
		t.Errorf("expected width 20, got %d: %q", len([]rune(result)), result)
	}
}

func TestRenderBranchColumn_Depth1NotLast(t *testing.T) {
	e := treeEntry{Depth: 1, IsLast: false, Ancestors: []bool{true}}
	result := e.renderBranchColumn("sibling", 20)
	if !strings.HasPrefix(result, "│ ├─") {
		t.Errorf("expected '│ ├─' prefix, got %q", result)
	}
	if len([]rune(result)) != 20 {
		t.Errorf("expected width 20, got %d: %q", len([]rune(result)), result)
	}
}

func TestRenderBranchColumn_Depth2Mixed(t *testing.T) {
	e := treeEntry{Depth: 2, IsLast: true, Ancestors: []bool{true, false}}
	result := e.renderBranchColumn("deep", 20)
	// Ancestors: [true, false] → "│ " + "  ", then "└─"
	if !strings.HasPrefix(result, "│   └─") {
		t.Errorf("expected '│   └─' prefix, got %q", result)
	}
	if len([]rune(result)) != 20 {
		t.Errorf("expected width 20, got %d: %q", len([]rune(result)), result)
	}
}

func TestBranchViewIntegration(t *testing.T) {
	width := 120
	list := newListView(false, "test-project")
	list.branchView = true
	list.showBranch = true
	list.SetSize(width, 24)

	tasks := []daemon.TaskInfo{
		{ID: 3, Branch: "feat-c", TargetBranch: "feat-a", Status: "running", Title: "Task C"},
		{ID: 2, Branch: "feat-b", TargetBranch: "feat-a", Status: "pending", Title: "Task B"},
		{ID: 1, Branch: "feat-a", TargetBranch: "main", Status: "completed", Title: "Task A"},
	}
	list.SetTasks(tasks)

	output := list.View()
	lines := strings.Split(output, "\n")

	// Collect widths of task rows (lines containing task data) and verify uniformity
	var taskLineWidths []int
	for _, line := range lines {
		if strings.Contains(line, "feat-") {
			taskLineWidths = append(taskLineWidths, lipgloss.Width(line))
		}
	}
	if len(taskLineWidths) > 1 {
		for i := 1; i < len(taskLineWidths); i++ {
			if taskLineWidths[i] != taskLineWidths[0] {
				t.Errorf("task lines have inconsistent widths: line 0 = %d, line %d = %d",
					taskLineWidths[0], i, taskLineWidths[i])
			}
		}
	}

	// Check that TREE header appears instead of BRANCH
	foundTree := false
	for _, line := range lines {
		if strings.Contains(line, "TREE") {
			foundTree = true
			break
		}
	}
	if !foundTree {
		t.Error("expected TREE header in branch view mode")
	}

	// Verify tree structure: root at depth 0, children at depth 1
	if len(list.treeEntries) != 3 {
		t.Fatalf("expected 3 tree entries, got %d", len(list.treeEntries))
	}
	if list.treeEntries[0].Depth != 0 {
		t.Errorf("root should be depth 0, got %d", list.treeEntries[0].Depth)
	}
	if list.treeEntries[1].Depth != 1 || list.treeEntries[2].Depth != 1 {
		t.Errorf("children should be depth 1, got %d and %d", list.treeEntries[1].Depth, list.treeEntries[2].Depth)
	}
}
