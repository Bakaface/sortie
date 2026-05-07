package tui

import (
	"testing"

	"github.com/aface/sortie/internal/client"
	"github.com/aface/sortie/internal/daemon"
	tea "github.com/charmbracelet/bubbletea"
)

func TestHandleConfirmKey_YConfirmsRetry(t *testing.T) {
	m := Model{
		keys:          newKeyMap(),
		client:        &client.Client{},
		list:          newListView(false, ""),
		detail:        newDetailView(),
		confirmAction: "retry",
		confirmTaskID: 42,
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}
	result, cmd := m.handleConfirmKey(msg)
	updated := result.(Model)

	if updated.confirmAction != "" {
		t.Errorf("expected confirmAction to be cleared, got %q", updated.confirmAction)
	}
	if updated.confirmTaskID != 0 {
		t.Errorf("expected confirmTaskID to be cleared, got %d", updated.confirmTaskID)
	}
	if cmd == nil {
		t.Error("expected command from confirmed retry, got nil")
	}
}

func TestHandleConfirmKey_NCancelsRetry(t *testing.T) {
	m := Model{
		keys:          newKeyMap(),
		client:        &client.Client{},
		list:          newListView(false, ""),
		detail:        newDetailView(),
		confirmAction: "retry",
		confirmTaskID: 42,
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}
	result, cmd := m.handleConfirmKey(msg)
	updated := result.(Model)

	if updated.confirmAction != "" {
		t.Errorf("expected confirmAction to be cleared, got %q", updated.confirmAction)
	}
	if updated.confirmTaskID != 0 {
		t.Errorf("expected confirmTaskID to be cleared, got %d", updated.confirmTaskID)
	}
	if cmd != nil {
		t.Error("expected no command from cancelled retry, got non-nil")
	}
}

func TestHandleConfirmKey_EscCancelsRetry(t *testing.T) {
	m := Model{
		keys:          newKeyMap(),
		client:        &client.Client{},
		list:          newListView(false, ""),
		detail:        newDetailView(),
		confirmAction: "retry",
		confirmTaskID: 42,
	}

	msg := tea.KeyMsg{Type: tea.KeyEsc}
	result, cmd := m.handleConfirmKey(msg)
	updated := result.(Model)

	if updated.confirmAction != "" {
		t.Errorf("expected confirmAction to be cleared, got %q", updated.confirmAction)
	}
	if cmd != nil {
		t.Error("expected no command from esc cancel, got non-nil")
	}
}

func TestHandleListKey_RTriggersRetryConfirmationForFailedTask(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		client: &client.Client{},
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 42, Title: "Failed task", Status: "failed"},
	})

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}
	result, cmd := m.handleListKey(msg)
	updated := result.(Model)

	if updated.confirmAction != "retry" {
		t.Errorf("expected confirmAction to be 'retry', got %q", updated.confirmAction)
	}
	if updated.confirmTaskID != 42 {
		t.Errorf("expected confirmTaskID to be 42, got %d", updated.confirmTaskID)
	}
	if cmd != nil {
		t.Error("expected no command (confirmation pending), got non-nil")
	}
}

func TestHandleListKey_RTriggersRetryConfirmationForCompletedTask(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		client: &client.Client{},
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 43, Title: "Completed task", Status: "completed"},
	})

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}
	result, cmd := m.handleListKey(msg)
	updated := result.(Model)

	if updated.confirmAction != "retry" {
		t.Errorf("expected confirmAction to be 'retry', got %q", updated.confirmAction)
	}
	if updated.confirmTaskID != 43 {
		t.Errorf("expected confirmTaskID to be 43, got %d", updated.confirmTaskID)
	}
	if cmd != nil {
		t.Error("expected no command (confirmation pending), got non-nil")
	}
}

func TestHandleListKey_RTriggersRetryConfirmationForTmuxTask(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		client: &client.Client{},
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 44, Title: "Tmux task", Status: "tmux"},
	})

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}
	result, cmd := m.handleListKey(msg)
	updated := result.(Model)

	if updated.confirmAction != "retry" {
		t.Errorf("expected confirmAction to be 'retry', got %q", updated.confirmAction)
	}
	if updated.confirmTaskID != 44 {
		t.Errorf("expected confirmTaskID to be 44, got %d", updated.confirmTaskID)
	}
	if cmd != nil {
		t.Error("expected no command (confirmation pending), got non-nil")
	}
}

func TestHandleListKey_RNoOpForRunningTask(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		client: &client.Client{},
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 45, Title: "Running task", Status: "running"},
	})

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}
	result, cmd := m.handleListKey(msg)
	updated := result.(Model)

	if updated.confirmAction != "" {
		t.Errorf("expected no confirmAction for running task, got %q", updated.confirmAction)
	}
	if cmd != nil {
		t.Error("expected no command for running task, got non-nil")
	}
}

func TestHandleListKey_RNoOpWithNoClient(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		client: nil,
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 46, Title: "Failed task", Status: "failed"},
	})

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	if updated.confirmAction != "" {
		t.Errorf("expected no confirmAction without client, got %q", updated.confirmAction)
	}
}
