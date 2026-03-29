package db

import (
	"path/filepath"
	"testing"
)

func TestUpdateTaskStepContext(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	proj, err := database.GetOrCreateProject("/home/user/myproject")
	if err != nil {
		t.Fatal(err)
	}

	task, err := database.CreateTask(proj.ID, "Test", "Test task", "test", "", "", "running", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create and complete a step with initial context
	if err := database.CreateTaskStep(task.ID, "implement"); err != nil {
		t.Fatal(err)
	}
	initial := "initial last_message context"
	if err := database.CompleteTaskStep(task.ID, "implement", &initial, 0); err != nil {
		t.Fatal(err)
	}

	// Verify initial context
	ctx, err := database.GetTaskStepContext(task.ID, "implement")
	if err != nil {
		t.Fatal(err)
	}
	if ctx != initial {
		t.Fatalf("expected initial context %q, got %q", initial, ctx)
	}

	// Update context (simulates background summarize_chat)
	updated := "summarized chat log context"
	if err := database.UpdateTaskStepContext(task.ID, "implement", updated); err != nil {
		t.Fatal(err)
	}

	// Verify updated context
	ctx, err = database.GetTaskStepContext(task.ID, "implement")
	if err != nil {
		t.Fatal(err)
	}
	if ctx != updated {
		t.Fatalf("expected updated context %q, got %q", updated, ctx)
	}
}
