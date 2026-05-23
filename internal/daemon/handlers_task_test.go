package daemon

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Bakaface/sortie/internal/config"
	"github.com/Bakaface/sortie/internal/db"
	"github.com/Bakaface/sortie/internal/task"
)

// setupServerWithProject creates an in-memory DB, a project, and returns the
// Server and project ID for ref-validation tests.
func setupServerWithProject(t *testing.T) (*Server, int64) {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	proj, err := database.GetOrCreateProject("/tmp/sortie-test")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	s := NewServer(cfg, database)
	return s, proj.ID
}

func TestValidateTaskRefs_NoRefs(t *testing.T) {
	s, projID := setupServerWithProject(t)
	got, err := s.validateTaskRefs("plain description with no refs", projID, 0, "description")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil auto-blocked-by, got %+v", got)
	}
}

func TestValidateTaskRefs_MissingTaskID(t *testing.T) {
	s, projID := setupServerWithProject(t)
	_, err := s.validateTaskRefs("see {{tasks.99.title}}", projID, 0, "description")
	if err == nil || !strings.Contains(err.Error(), "#99") {
		t.Fatalf("expected error mentioning task #99, got %v", err)
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error should mention missing task: %v", err)
	}
}

func TestValidateTaskRefs_DifferentProject(t *testing.T) {
	s, projID := setupServerWithProject(t)
	// Create a task in a different project.
	other, err := s.database.GetOrCreateProject("/tmp/sortie-other")
	if err != nil {
		t.Fatal(err)
	}
	tk, err := s.database.CreateTask(other.ID, "other proj task", "", "slug", "default", "main", task.StatusPending, nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.validateTaskRefs("ref={{tasks."+itoa(tk.ID)+".title}}", projID, 0, "description")
	if err == nil || !strings.Contains(err.Error(), "another project") {
		t.Fatalf("expected cross-project error, got %v", err)
	}
}

func TestValidateTaskRefs_FailedDep(t *testing.T) {
	s, projID := setupServerWithProject(t)
	tk, err := s.database.CreateTask(projID, "failed dep", "", "slug", "default", "main", task.StatusFailed, nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.validateTaskRefs("dep={{tasks."+itoa(tk.ID)+".title}}", projID, 0, "description")
	if err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("expected failed-dep error, got %v", err)
	}
}

func TestValidateTaskRefs_UnsupportedField(t *testing.T) {
	s, projID := setupServerWithProject(t)
	tk, err := s.database.CreateTask(projID, "ok", "", "slug", "default", "main", task.StatusPending, nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.validateTaskRefs("x={{tasks."+itoa(tk.ID)+".slug}}", projID, 0, "description")
	if err == nil || !strings.Contains(err.Error(), "slug") {
		t.Fatalf("expected unsupported-field error, got %v", err)
	}
	if !strings.Contains(err.Error(), "supported:") {
		t.Errorf("error should list supported fields: %v", err)
	}
}

func TestValidateTaskRefs_ActiveDepAutoBlocks(t *testing.T) {
	s, projID := setupServerWithProject(t)
	tk1, err := s.database.CreateTask(projID, "active a", "", "a", "default", "main", task.StatusPending, nil)
	if err != nil {
		t.Fatal(err)
	}
	tk2, err := s.database.CreateTask(projID, "active b", "", "b", "default", "main", task.StatusRunning, nil)
	if err != nil {
		t.Fatal(err)
	}

	auto, err := s.validateTaskRefs(
		"foo {{tasks."+itoa(tk1.ID)+".title}} bar {{tasks."+itoa(tk2.ID)+".branch}}",
		projID,
		0,
		"description",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(auto) != 2 {
		t.Fatalf("expected 2 auto-blockers, got %d: %v", len(auto), auto)
	}
	// Dedup: repeating the same id should still only produce one entry.
	auto2, err := s.validateTaskRefs(
		"a {{tasks."+itoa(tk1.ID)+".title}} a again {{tasks."+itoa(tk1.ID)+".description}}",
		projID,
		0,
		"description",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(auto2) != 1 || auto2[0] != tk1.ID {
		t.Errorf("expected single deduped id, got %v", auto2)
	}
}

func TestValidateTaskRefs_CompletedDepNoAutoBlock(t *testing.T) {
	s, projID := setupServerWithProject(t)
	tk, err := s.database.CreateTask(projID, "done", "", "slug", "default", "main", task.StatusCompleted, nil)
	if err != nil {
		t.Fatal(err)
	}

	auto, err := s.validateTaskRefs("x={{tasks."+itoa(tk.ID)+".title}}", projID, 0, "description")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(auto) != 0 {
		t.Errorf("completed dep should not auto-block, got %v", auto)
	}
}

func TestMergeBlockedBy(t *testing.T) {
	cases := []struct {
		name     string
		explicit []int64
		auto     []int64
		want     []int64
	}{
		{"both empty", nil, nil, nil},
		{"only explicit", []int64{3, 1, 2}, nil, []int64{3, 1, 2}},
		{"only auto", nil, []int64{5, 4}, []int64{5, 4}},
		{"no overlap", []int64{1, 2}, []int64{3, 4}, []int64{1, 2, 3, 4}},
		{"overlap deduped", []int64{1, 2, 3}, []int64{2, 4, 1}, []int64{1, 2, 3, 4}},
		{"explicit dup removed", []int64{1, 1, 2}, nil, []int64{1, 2}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mergeBlockedBy(tc.explicit, tc.auto)
			if !equalInts(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func equalInts(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// itoa is a small convenience wrapper around strconv.FormatInt to keep
// per-line clutter low in the table-driven tests above.
func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// --- Edit handler tests -----------------------------------------------------

func TestHandleUpdateField_DescriptionAutoAddsDep(t *testing.T) {
	s, projID := setupServerWithProject(t)
	host, err := s.database.CreateTask(projID, "host", "old description", "host", "default", "main", task.StatusPending, nil)
	if err != nil {
		t.Fatal(err)
	}
	dep, err := s.database.CreateTask(projID, "dep", "", "dep", "default", "main", task.StatusRunning, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Run the validation+merge logic the handler performs.
	newValue := "now references {{tasks." + itoa(dep.ID) + ".title}}"
	auto, err := s.validateTaskRefs(newValue, projID, host.ID, "description")
	if err != nil {
		t.Fatalf("validation failed: %v", err)
	}
	if err := s.database.UpdateTaskDescription(host.ID, newValue); err != nil {
		t.Fatal(err)
	}
	for _, d := range auto {
		if err := s.database.AddTaskDependency(host.ID, d); err != nil {
			t.Fatal(err)
		}
	}

	updated, err := s.database.GetTask(host.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.BlockedBy) != 1 || updated.BlockedBy[0] != dep.ID {
		t.Errorf("expected BlockedBy=[%d], got %v", dep.ID, updated.BlockedBy)
	}
}

func TestHandleUpdateField_RejectsBadField(t *testing.T) {
	s, projID := setupServerWithProject(t)
	host, err := s.database.CreateTask(projID, "host", "ok", "host", "default", "main", task.StatusPending, nil)
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.database.CreateTask(projID, "other", "", "other", "default", "main", task.StatusPending, nil)
	if err != nil {
		t.Fatal(err)
	}

	bad := "see {{tasks." + itoa(other.ID) + ".slug}}"
	_, err = s.validateTaskRefs(bad, projID, host.ID, "description")
	if err == nil {
		t.Fatal("expected error for unsupported field")
	}
	// Original description should remain unchanged because validation runs
	// before mutation in the handler.
	got, err := s.database.GetTask(host.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "ok" {
		t.Errorf("description should be unchanged after rejection, got %q", got.Description)
	}
}

// A task's description may legitimately reference its own ID — runtime lookup
// resolves the value — but the validator must never auto-add a self-blocking
// edge. The claim filter would otherwise leave the task blocked forever.
func TestValidateTaskRefs_SelfReferenceNotBlocker(t *testing.T) {
	s, projID := setupServerWithProject(t)
	self, err := s.database.CreateTask(projID, "host", "old", "host", "default", "main", task.StatusPending, nil)
	if err != nil {
		t.Fatal(err)
	}

	auto, err := s.validateTaskRefs(
		"refers to itself {{tasks."+itoa(self.ID)+".title}}",
		projID,
		self.ID,
		"description",
	)
	if err != nil {
		t.Fatalf("self-ref should not error: %v", err)
	}
	if len(auto) != 0 {
		t.Errorf("self-ref must not become an auto-blocker, got %v", auto)
	}
}

// Status init is transient during task creation; it should be treated as
// active and auto-add the referenced task to BlockedBy.
func TestValidateTaskRefs_InitStatusAutoBlocks(t *testing.T) {
	s, projID := setupServerWithProject(t)
	tk, err := s.database.CreateTask(projID, "initializing", "", "init", "default", "main", task.StatusInit, nil)
	if err != nil {
		t.Fatal(err)
	}

	auto, err := s.validateTaskRefs(
		"refs {{tasks."+itoa(tk.ID)+".title}}",
		projID,
		0,
		"description",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(auto) != 1 || auto[0] != tk.ID {
		t.Errorf("init-status dep should be auto-blocker, got %v", auto)
	}
}
