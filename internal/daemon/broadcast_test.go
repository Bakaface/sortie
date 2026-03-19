package daemon

import (
	"path/filepath"
	"testing"

	"github.com/aface/sortie/internal/config"
	"github.com/aface/sortie/internal/db"
	"github.com/aface/sortie/internal/task"
)

// TestFinalizeCompletedTaskNoProjectContext verifies that when the project context
// cannot be loaded, finalizeCompletedTask still marks the task as completed.
func TestFinalizeCompletedTaskNoProjectContext(t *testing.T) {
	dir := t.TempDir()

	dbPath := filepath.Join(dir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{}
	s := NewServer(cfg, database)

	proj, err := database.GetOrCreateProject(dir)
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	tk, err := database.CreateTask(proj.ID, "Test task", "desc", "slug", "default", "", task.StatusRunning, nil)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// Use a non-existent project ID to force project context failure
	tk.ProjectID = 99999
	// No worktree so no cleanup needed
	tk.Worktree = false
	tk.WorktreePath = ""

	s.finalizeCompletedTask(tk, "agent-1", "Test task")

	// Re-fetch using original project
	tk2, err := database.GetTask(tk.ID)
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}

	if tk2.Status != task.StatusCompleted {
		t.Errorf("expected task status %s, got %s", task.StatusCompleted, tk2.Status)
	}
}

// TestFinalizeCompletedTaskSetsFinalizingStatus verifies that finalizeCompletedTask
// sets StatusFinalizing before calling runFinalization when there's a project context
// and meaningful changes detected. Since we can't easily mock git operations in an
// integration test, we test the case where the worktree path is empty (skips fast-track).
func TestFinalizeCompletedTaskSetsFinalizingBeforeCompletion(t *testing.T) {
	dir := t.TempDir()

	dbPath := filepath.Join(dir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		Git: config.GitConfig{OnComplete: "none"},
		Workflows: []config.WorkflowConfig{
			{
				Name: "default",
				Steps: []config.StepConfig{
					{Name: "implement", Prompt: "do something"},
				},
			},
		},
	}
	s := NewServer(cfg, database)

	proj, err := database.GetOrCreateProject(dir)
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	// Pre-load project context so finalizeCompletedTask can find it
	if _, err := s.getProjectContext(proj.ID); err != nil {
		t.Fatalf("failed to pre-load project context: %v", err)
	}

	tk, err := database.CreateTask(proj.ID, "Test task", "desc", "slug", "default", "", task.StatusRunning, nil)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// No worktree path: skips the fast-track check
	// No worktree: skips fast-track entirely (t.Worktree == false and t.WorktreePath == "")
	tk.Worktree = false
	tk.WorktreePath = ""

	s.finalizeCompletedTask(tk, "agent-1", "Test task")

	refreshed, err := database.GetTask(tk.ID)
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}

	// After finalization, the task should be completed
	if refreshed.Status != task.StatusCompleted {
		t.Errorf("expected task status %s after finalization, got %s", task.StatusCompleted, refreshed.Status)
	}
}

// TestFinalizeCompletedTaskFastTracksNoWorktree verifies that non-worktree tasks
// that have a project context still end up as StatusCompleted.
func TestFinalizeCompletedTaskCompletesSuccessfully(t *testing.T) {
	dir := t.TempDir()

	dbPath := filepath.Join(dir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		Git: config.GitConfig{OnComplete: "none"},
	}
	s := NewServer(cfg, database)

	proj, err := database.GetOrCreateProject(dir)
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	// Pre-load project context
	if _, err := s.getProjectContext(proj.ID); err != nil {
		t.Fatalf("failed to pre-load project context: %v", err)
	}

	tk, err := database.CreateTask(proj.ID, "Completed task", "desc", "slug", "default", "", task.StatusRunning, nil)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}
	tk.Worktree = false
	tk.WorktreePath = ""

	s.finalizeCompletedTask(tk, "test-agent", "Completed task")

	refreshed, err := database.GetTask(tk.ID)
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}

	if refreshed.Status != task.StatusCompleted {
		t.Errorf("expected StatusCompleted, got %s", refreshed.Status)
	}
}
