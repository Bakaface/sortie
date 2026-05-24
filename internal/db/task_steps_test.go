package db

import (
	"path/filepath"
	"testing"
)

// stepContext reads back the raw context value for a step, regardless of
// status. The package-public GetTaskStepContext gates on 'completed', which
// isn't useful when testing running-step writes.
func stepContext(t *testing.T, database *DB, taskID int64, stepName string) string {
	t.Helper()
	rows, err := database.GetTaskStepRows(taskID)
	if err != nil {
		t.Fatalf("GetTaskStepRows: %v", err)
	}
	row, ok := rows[stepName]
	if !ok {
		t.Fatalf("no task_steps row for %q", stepName)
	}
	return row.Context
}

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

func TestUpdateRunningTaskStepContext_Replace(t *testing.T) {
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
	tk, err := database.CreateTask(proj.ID, "Test", "Test task", "test", "", "", "running", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.CreateTaskStep(tk.ID, "implement"); err != nil {
		t.Fatal(err)
	}

	rows, err := database.UpdateRunningTaskStepContext(tk.ID, "implement", "first value", false)
	if err != nil {
		t.Fatalf("first replace: %v", err)
	}
	if rows != 1 {
		t.Fatalf("expected 1 row affected, got %d", rows)
	}
	if got := stepContext(t, database, tk.ID, "implement"); got != "first value" {
		t.Fatalf("after first replace: got %q", got)
	}

	rows, err = database.UpdateRunningTaskStepContext(tk.ID, "implement", "second value", false)
	if err != nil {
		t.Fatalf("second replace: %v", err)
	}
	if rows != 1 {
		t.Fatalf("expected 1 row affected, got %d", rows)
	}
	if got := stepContext(t, database, tk.ID, "implement"); got != "second value" {
		t.Fatalf("after second replace: got %q", got)
	}
}

func TestUpdateRunningTaskStepContext_AppendBuildsUpWithNewline(t *testing.T) {
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
	tk, err := database.CreateTask(proj.ID, "Test", "Test task", "test", "", "", "running", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.CreateTaskStep(tk.ID, "implement"); err != nil {
		t.Fatal(err)
	}

	// First append against empty context should NOT prepend a newline.
	if _, err := database.UpdateRunningTaskStepContext(tk.ID, "implement", "alpha", true); err != nil {
		t.Fatal(err)
	}
	if got := stepContext(t, database, tk.ID, "implement"); got != "alpha" {
		t.Fatalf("first append: got %q, want %q", got, "alpha")
	}

	// Second append should add newline + value.
	if _, err := database.UpdateRunningTaskStepContext(tk.ID, "implement", "beta", true); err != nil {
		t.Fatal(err)
	}
	if got := stepContext(t, database, tk.ID, "implement"); got != "alpha\nbeta" {
		t.Fatalf("second append: got %q, want %q", got, "alpha\nbeta")
	}

	// Third append.
	if _, err := database.UpdateRunningTaskStepContext(tk.ID, "implement", "gamma", true); err != nil {
		t.Fatal(err)
	}
	if got := stepContext(t, database, tk.ID, "implement"); got != "alpha\nbeta\ngamma" {
		t.Fatalf("third append: got %q, want %q", got, "alpha\nbeta\ngamma")
	}
}

func TestUpdateRunningTaskStepContext_IgnoresCompletedStep(t *testing.T) {
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
	tk, err := database.CreateTask(proj.ID, "Test", "Test task", "test", "", "", "running", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.CreateTaskStep(tk.ID, "implement"); err != nil {
		t.Fatal(err)
	}

	initial := "completed-context"
	if err := database.CompleteTaskStep(tk.ID, "implement", &initial, 0); err != nil {
		t.Fatal(err)
	}

	rows, err := database.UpdateRunningTaskStepContext(tk.ID, "implement", "should not stick", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rows != 0 {
		t.Fatalf("expected 0 rows affected (step is completed), got %d", rows)
	}
	if got := stepContext(t, database, tk.ID, "implement"); got != initial {
		t.Fatalf("completed context should be untouched, got %q", got)
	}
}

func TestUpdateRunningTaskStepContext_UnknownStep(t *testing.T) {
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
	tk, err := database.CreateTask(proj.ID, "Test", "Test task", "test", "", "", "running", nil)
	if err != nil {
		t.Fatal(err)
	}

	rows, err := database.UpdateRunningTaskStepContext(tk.ID, "never-created", "value", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rows != 0 {
		t.Fatalf("expected 0 rows affected for unknown step, got %d", rows)
	}
}
