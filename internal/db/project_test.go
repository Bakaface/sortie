package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetOrCreateProject(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// Create a project
	proj, err := database.GetOrCreateProject("/home/user/myproject")
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}
	if proj.ID == 0 {
		t.Error("expected non-zero project ID")
	}
	if proj.Path != "/home/user/myproject" {
		t.Errorf("expected path '/home/user/myproject', got '%s'", proj.Path)
	}
	if proj.Name != "myproject" {
		t.Errorf("expected name 'myproject', got '%s'", proj.Name)
	}

	// Get the same project again (should return existing)
	proj2, err := database.GetOrCreateProject("/home/user/myproject")
	if err != nil {
		t.Fatalf("failed to get existing project: %v", err)
	}
	if proj2.ID != proj.ID {
		t.Errorf("expected same ID %d, got %d", proj.ID, proj2.ID)
	}
}

func TestGetOrCreateProject_NormalizesPath(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// Use current directory as a relative path that can be resolved
	cwd, _ := os.Getwd()
	proj, err := database.GetOrCreateProject(cwd)
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}
	if proj.Path != cwd {
		t.Errorf("expected absolute path '%s', got '%s'", cwd, proj.Path)
	}
}

func TestGetProjectByPath_NotFound(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	_, err = database.GetProjectByPath("/nonexistent/path")
	if err == nil {
		t.Error("expected error for non-existent project")
	}
}

func TestListProjects(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// Create two projects
	_, err = database.GetOrCreateProject("/home/user/project-a")
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.GetOrCreateProject("/home/user/project-b")
	if err != nil {
		t.Fatal(err)
	}

	projects, err := database.ListProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 {
		t.Errorf("expected 2 projects, got %d", len(projects))
	}
}

func TestGetTasksByProject(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// Create two projects
	projA, err := database.GetOrCreateProject("/home/user/project-a")
	if err != nil {
		t.Fatal(err)
	}
	projB, err := database.GetOrCreateProject("/home/user/project-b")
	if err != nil {
		t.Fatal(err)
	}

	// Create tasks for each project
	_, err = database.CreateTask(projA.ID, "Task A1", "Description A1", "task-a1", "", "", "pending", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.CreateTask(projA.ID, "Task A2", "Description A2", "task-a2", "", "", "pending", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.CreateTask(projB.ID, "Task B1", "Description B1", "task-b1", "", "", "pending", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Get tasks for project A
	tasksA, err := database.GetTasksByProject(projA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasksA) != 2 {
		t.Errorf("expected 2 tasks for project A, got %d", len(tasksA))
	}
	for _, task := range tasksA {
		if task.ProjectID != projA.ID {
			t.Errorf("expected project_id %d, got %d", projA.ID, task.ProjectID)
		}
	}

	// Get tasks for project B
	tasksB, err := database.GetTasksByProject(projB.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasksB) != 1 {
		t.Errorf("expected 1 task for project B, got %d", len(tasksB))
	}

	// Get all tasks
	allTasks, err := database.GetAllTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(allTasks) != 3 {
		t.Errorf("expected 3 total tasks, got %d", len(allTasks))
	}
}

func TestCreateTask_WithProjectID(t *testing.T) {
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

	task, err := database.CreateTask(proj.ID, "Test", "Test desc", "test", "", "", "pending", nil)
	if err != nil {
		t.Fatal(err)
	}
	if task.ProjectID != proj.ID {
		t.Errorf("expected project_id %d, got %d", proj.ID, task.ProjectID)
	}
}

func TestMigration_FreshDBHasProjectsTable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// Fresh DB should be able to create projects (table exists from schema)
	proj, err := database.GetOrCreateProject("/home/user/test-project")
	if err != nil {
		t.Fatalf("expected to create project in fresh DB: %v", err)
	}
	if proj.ID == 0 {
		t.Error("expected non-zero project ID")
	}
}
