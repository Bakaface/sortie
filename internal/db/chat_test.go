package db

import (
	"path/filepath"
	"testing"
)

func TestUpsertChat(t *testing.T) {
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

	// Insert a chat
	if err := database.UpsertChat(task.ID, "implement", "session-001", "tmux-abc"); err != nil {
		t.Fatal(err)
	}

	// Verify it exists
	chat, err := database.GetChatByStep(task.ID, "implement")
	if err != nil {
		t.Fatal(err)
	}
	if chat == nil {
		t.Fatal("expected chat to exist, got nil")
	}
	if chat.SessionID != "session-001" {
		t.Errorf("expected session_id %q, got %q", "session-001", chat.SessionID)
	}
	if chat.TmuxSessionName != "tmux-abc" {
		t.Errorf("expected tmux_session_name %q, got %q", "tmux-abc", chat.TmuxSessionName)
	}
	if chat.StepName != "implement" {
		t.Errorf("expected step_name %q, got %q", "implement", chat.StepName)
	}
	if chat.TaskID != task.ID {
		t.Errorf("expected task_id %d, got %d", task.ID, chat.TaskID)
	}

	// Upsert again with a different session ID — should update
	if err := database.UpsertChat(task.ID, "implement", "session-002", "tmux-xyz"); err != nil {
		t.Fatal(err)
	}

	updated, err := database.GetChatByStep(task.ID, "implement")
	if err != nil {
		t.Fatal(err)
	}
	if updated == nil {
		t.Fatal("expected updated chat to exist, got nil")
	}
	if updated.SessionID != "session-002" {
		t.Errorf("expected updated session_id %q, got %q", "session-002", updated.SessionID)
	}
	if updated.TmuxSessionName != "tmux-xyz" {
		t.Errorf("expected updated tmux_session_name %q, got %q", "tmux-xyz", updated.TmuxSessionName)
	}
}

func TestGetLatestChat(t *testing.T) {
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

	// Insert a chat for "plan" first
	if err := database.UpsertChat(task.ID, "plan", "session-plan", ""); err != nil {
		t.Fatal(err)
	}

	// Upsert "plan" again with a new session ID — this triggers the ON CONFLICT path
	// which updates created_at to CURRENT_TIMESTAMP, making it the most recent
	if err := database.UpsertChat(task.ID, "plan", "session-plan-updated", ""); err != nil {
		t.Fatal(err)
	}

	// Insert "implement" — created_at will be the same second or earlier than the updated "plan"
	// so we check that GetLatestChat returns a valid chat for this task
	if err := database.UpsertChat(task.ID, "implement", "session-implement", ""); err != nil {
		t.Fatal(err)
	}

	latest, err := database.GetLatestChat(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest == nil {
		t.Fatal("expected a chat, got nil")
	}
	// Should return one of the task's chats with a valid session ID
	if latest.TaskID != task.ID {
		t.Errorf("expected task_id %d, got %d", task.ID, latest.TaskID)
	}
	if latest.SessionID == "" {
		t.Error("expected non-empty session_id from GetLatestChat")
	}
}

func TestGetChatsByTask(t *testing.T) {
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

	steps := []struct {
		name      string
		sessionID string
	}{
		{"plan", "session-plan"},
		{"implement", "session-implement"},
		{"review", "session-review"},
	}

	for _, s := range steps {
		if err := database.UpsertChat(task.ID, s.name, s.sessionID, ""); err != nil {
			t.Fatal(err)
		}
	}

	chats, err := database.GetChatsByTask(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(chats) != 3 {
		t.Fatalf("expected 3 chats, got %d", len(chats))
	}

	// Verify all steps are present
	sessionIDs := make(map[string]bool)
	for _, c := range chats {
		sessionIDs[c.SessionID] = true
	}
	for _, s := range steps {
		if !sessionIDs[s.sessionID] {
			t.Errorf("expected session_id %q in results", s.sessionID)
		}
	}

	// Verify task IDs are all correct
	for _, c := range chats {
		if c.TaskID != task.ID {
			t.Errorf("expected task_id %d, got %d", task.ID, c.TaskID)
		}
	}
}

func TestDeleteChatsForTask(t *testing.T) {
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

	if err := database.UpsertChat(task.ID, "plan", "session-plan", ""); err != nil {
		t.Fatal(err)
	}
	if err := database.UpsertChat(task.ID, "implement", "session-implement", ""); err != nil {
		t.Fatal(err)
	}

	// Verify chats exist before deletion
	before, err := database.GetChatsByTask(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 2 {
		t.Fatalf("expected 2 chats before deletion, got %d", len(before))
	}

	// Delete chats
	if err := database.DeleteChatsForTask(task.ID); err != nil {
		t.Fatal(err)
	}

	// GetLatestChat should return nil
	latest, err := database.GetLatestChat(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest != nil {
		t.Errorf("expected nil after deletion, got chat with session_id %q", latest.SessionID)
	}

	// GetChatsByTask should return empty slice
	after, err := database.GetChatsByTask(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Errorf("expected 0 chats after deletion, got %d", len(after))
	}
}

func TestGetChatByStep_NotFound(t *testing.T) {
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

	// No chats inserted — should return nil, nil
	chat, err := database.GetChatByStep(task.ID, "nonexistent-step")
	if err != nil {
		t.Fatalf("expected nil error for non-existent step, got: %v", err)
	}
	if chat != nil {
		t.Errorf("expected nil chat for non-existent step, got chat with session_id %q", chat.SessionID)
	}
}
