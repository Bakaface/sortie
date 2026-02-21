package db

import (
	"path/filepath"
	"testing"

	"github.com/aface/sortie/internal/task"
)

func TestGetAllTasks_SortedDescending(t *testing.T) {
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

	// Create tasks in ascending order
	_, err = database.CreateTask(proj.ID, "First", "First task", "first", "", "", "pending", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.CreateTask(proj.ID, "Second", "Second task", "second", "", "", "pending", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.CreateTask(proj.ID, "Third", "Third task", "third", "", "", "pending", nil)
	if err != nil {
		t.Fatal(err)
	}

	tasks, err := database.GetAllTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}

	// Tasks should be sorted by ID descending (newest first)
	for i := 1; i < len(tasks); i++ {
		if tasks[i-1].ID <= tasks[i].ID {
			t.Errorf("expected descending order: task[%d].ID=%d should be > task[%d].ID=%d",
				i-1, tasks[i-1].ID, i, tasks[i].ID)
		}
	}
}

func TestGetTasksByProject_SortedDescending(t *testing.T) {
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

	// Create tasks in ascending order
	_, err = database.CreateTask(proj.ID, "First", "First task", "first", "", "", "pending", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.CreateTask(proj.ID, "Second", "Second task", "second", "", "", "pending", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.CreateTask(proj.ID, "Third", "Third task", "third", "", "", "pending", nil)
	if err != nil {
		t.Fatal(err)
	}

	tasks, err := database.GetTasksByProject(proj.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}

	// Tasks should be sorted by ID descending (newest first)
	for i := 1; i < len(tasks); i++ {
		if tasks[i-1].ID <= tasks[i].ID {
			t.Errorf("expected descending order: task[%d].ID=%d should be > task[%d].ID=%d",
				i-1, tasks[i-1].ID, i, tasks[i].ID)
		}
	}
}

func TestCreateTaskWithPriority(t *testing.T) {
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

	// Create task with explicit priority
	created, err := database.CreateTaskWithPriority(proj.ID, "Urgent task", "Do it now", "urgent-task", "", "", "pending", task.PriorityUrgent, nil)
	if err != nil {
		t.Fatal(err)
	}

	if created.Priority != task.PriorityUrgent {
		t.Errorf("expected priority 'urgent', got %q", created.Priority)
	}

	// Create task with default priority (via CreateTask)
	defaultTask, err := database.CreateTask(proj.ID, "Default task", "Do it", "default-task", "", "", "pending", nil)
	if err != nil {
		t.Fatal(err)
	}

	if defaultTask.Priority != task.PriorityMedium {
		t.Errorf("expected default priority 'medium', got %q", defaultTask.Priority)
	}
}

func TestUpdateTaskPriority(t *testing.T) {
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

	created, err := database.CreateTask(proj.ID, "Task", "Description", "task-slug", "", "", "pending", nil)
	if err != nil {
		t.Fatal(err)
	}

	if created.Priority != task.PriorityMedium {
		t.Errorf("expected initial priority 'medium', got %q", created.Priority)
	}

	// Update priority
	if err := database.UpdateTaskPriority(created.ID, task.PriorityHigh); err != nil {
		t.Fatal(err)
	}

	updated, err := database.GetTask(created.ID)
	if err != nil {
		t.Fatal(err)
	}

	if updated.Priority != task.PriorityHigh {
		t.Errorf("expected priority 'high', got %q", updated.Priority)
	}
}

func TestGetClaimableTasks_SortedByPriority(t *testing.T) {
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

	// Create tasks with different priorities
	_, err = database.CreateTaskWithPriority(proj.ID, "Low task", "Low", "low-task", "", "", "pending", task.PriorityLow, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.CreateTaskWithPriority(proj.ID, "High task", "High", "high-task", "", "", "pending", task.PriorityHigh, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.CreateTaskWithPriority(proj.ID, "Urgent task", "Urgent", "urgent-task", "", "", "pending", task.PriorityUrgent, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.CreateTaskWithPriority(proj.ID, "Medium task", "Medium", "medium-task", "", "", "pending", task.PriorityMedium, nil)
	if err != nil {
		t.Fatal(err)
	}

	tasks, err := database.GetClaimableTasks()
	if err != nil {
		t.Fatal(err)
	}

	if len(tasks) != 4 {
		t.Fatalf("expected 4 claimable tasks, got %d", len(tasks))
	}

	// Should be sorted by priority descending: urgent, high, medium, low
	expectedOrder := []task.Priority{task.PriorityUrgent, task.PriorityHigh, task.PriorityMedium, task.PriorityLow}
	for i, expected := range expectedOrder {
		if tasks[i].Priority != expected {
			t.Errorf("task[%d]: expected priority %q, got %q (title: %s)", i, expected, tasks[i].Priority, tasks[i].Title)
		}
	}
}

func TestResetTaskForRetryFromStep(t *testing.T) {
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

	// Create a failed task with step_index = 2
	created, err := database.CreateTask(proj.ID, "Failed task", "Description", "failed-task", "", "", task.StatusFailed, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate task progression
	if err := database.UpdateTaskStep(created.ID, 2, "step2"); err != nil {
		t.Fatal(err)
	}
	if err := database.UpdateTaskExitCode(created.ID, 1, "error occurred"); err != nil {
		t.Fatal(err)
	}

	// Reset for retry from step (should preserve step_index)
	if err := database.ResetTaskForRetryFromStep(created.ID); err != nil {
		t.Fatal(err)
	}

	updated, err := database.GetTask(created.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Status should be pending
	if updated.Status != task.StatusPending {
		t.Errorf("expected status 'pending', got %q", updated.Status)
	}

	// step_index should be preserved (still 2)
	if updated.StepIndex != 2 {
		t.Errorf("expected step_index 2, got %d", updated.StepIndex)
	}

	// current_step should be cleared
	if updated.CurrentStep != "" {
		t.Errorf("expected current_step to be empty, got %q", updated.CurrentStep)
	}

	// exit_code should be cleared
	if updated.ExitCode != nil {
		t.Errorf("expected exit_code to be nil, got %v", *updated.ExitCode)
	}

	// error_message should be cleared
	if updated.ErrorMessage != "" {
		t.Errorf("expected error_message to be empty, got %q", updated.ErrorMessage)
	}

	// started_at and completed_at should be cleared
	if updated.StartedAt != nil {
		t.Errorf("expected started_at to be nil, got %v", *updated.StartedAt)
	}
	if updated.CompletedAt != nil {
		t.Errorf("expected completed_at to be nil, got %v", *updated.CompletedAt)
	}
}
