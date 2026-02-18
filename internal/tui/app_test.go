package tui

import (
	"strings"
	"testing"

	"github.com/aface/ralph-tamer-kit/internal/daemon"
	tea "github.com/charmbracelet/bubbletea"
)

func TestHandleDetailKey_QReturnsToList(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(),
		detail: newDetailView(),
		view:   viewDetail,
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	result, cmd := m.handleDetailKey(msg)
	updated := result.(Model)

	if updated.view != viewList {
		t.Errorf("expected view to be viewList (%d), got %d", viewList, updated.view)
	}
	if updated.quitting {
		t.Error("expected quitting to be false")
	}
	if cmd != nil {
		t.Error("expected no command, got non-nil")
	}
}

func TestHandleDetailKey_EscReturnsToList(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(),
		detail: newDetailView(),
		view:   viewDetail,
	}
	// Default follow mode is true; first Esc exits follow mode
	m.detail.SetFollowMode(false)

	msg := tea.KeyMsg{Type: tea.KeyEscape}
	result, cmd := m.handleDetailKey(msg)
	updated := result.(Model)

	if updated.view != viewList {
		t.Errorf("expected view to be viewList (%d), got %d", viewList, updated.view)
	}
	if cmd != nil {
		t.Error("expected no command, got non-nil")
	}
}

func TestHandleDetailKey_EscInFollowModeExitsFollow(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(),
		detail: newDetailView(),
		view:   viewDetail,
	}
	m.detail.SetFollowMode(true)

	msg := tea.KeyMsg{Type: tea.KeyEscape}
	result, _ := m.handleDetailKey(msg)
	updated := result.(Model)

	if updated.view != viewDetail {
		t.Errorf("expected view to remain viewDetail (%d), got %d", viewDetail, updated.view)
	}
	if updated.detail.IsFollowMode() {
		t.Error("expected follow mode to be false after esc")
	}
}

func TestHandleDetailKey_QInFollowModeReturnsToList(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(),
		detail: newDetailView(),
		view:   viewDetail,
	}
	m.detail.SetFollowMode(true)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	result, _ := m.handleDetailKey(msg)
	updated := result.(Model)

	if updated.view != viewList {
		t.Errorf("expected view to be viewList (%d), got %d", viewList, updated.view)
	}
}

func TestDetailView_ShowsOnlyLogs(t *testing.T) {
	d := newDetailView()
	d.SetTask(&daemon.TaskInfo{
		ID:           14,
		Title:        "Test task",
		Description:  "Some description",
		Status:       "running",
		Branch:       "rtk/14-test",
		CurrentStep:  "implement",
		StepIndex:    1,
		Context:      "some context info",
		ErrorMessage: "",
	})
	d.SetSize(80, 24)
	d.SetOutput([]string{"log line 1", "log line 2"})

	output := d.View()

	// Should contain the compact title with task ID
	if !strings.Contains(output, "#14") {
		t.Error("expected output to contain task ID '#14'")
	}

	// Should contain log content
	if !strings.Contains(output, "log line 1") {
		t.Error("expected output to contain log lines")
	}

	// Should NOT contain verbose metadata sections
	if strings.Contains(output, "Status:") {
		t.Error("expected output to not contain 'Status:' metadata line")
	}
	if strings.Contains(output, "Branch:") {
		t.Error("expected output to not contain 'Branch:' metadata line")
	}
	if strings.Contains(output, "Step 1:") {
		t.Error("expected output to not contain workflow step line")
	}
	if strings.Contains(output, "Context:") {
		t.Error("expected output to not contain 'Context:' section")
	}
	if strings.Contains(output, "Some description") {
		t.Error("expected output to not contain description")
	}
}

func TestTmuxSessionsMsg_UpdatesListView(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(),
		detail: newDetailView(),
		view:   viewList,
	}

	sessions := tmuxSessionsMsg(map[int64]bool{1: true, 5: true})
	result, cmd := m.Update(sessions)
	updated := result.(Model)

	if !updated.list.tmuxSessions[1] {
		t.Error("expected task 1 to have tmux session")
	}
	if !updated.list.tmuxSessions[5] {
		t.Error("expected task 5 to have tmux session")
	}
	if updated.list.tmuxSessions[3] {
		t.Error("expected task 3 to NOT have tmux session")
	}
	if cmd != nil {
		t.Error("expected no command, got non-nil")
	}
}

func TestTmuxDetachedMsg_TriggersRefresh(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(),
		detail: newDetailView(),
		view:   viewList,
	}

	msg := tmuxDetachedMsg{taskID: 42}
	_, cmd := m.Update(msg)

	if cmd == nil {
		t.Error("expected refresh command after tmux detach, got nil")
	}
}

func TestHandleListKey_TReturnsCommandWhenTaskSelected(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(),
		detail: newDetailView(),
		view:   viewList,
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 42, Title: "Test", Status: "awaiting-approval"},
	})

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}}
	_, cmd := m.handleListKey(msg)

	if cmd == nil {
		t.Error("expected command from 't' key with selected task, got nil")
	}
}

func TestHandleListKey_TNoOpWithNoTasks(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(),
		detail: newDetailView(),
		view:   viewList,
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}}
	_, cmd := m.handleListKey(msg)

	if cmd != nil {
		t.Error("expected no command from 't' key with no tasks, got non-nil")
	}
}

func TestHandleDetailKey_TReturnsCommandWhenTaskSet(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(),
		detail: newDetailView(),
		view:   viewDetail,
	}
	task := daemon.TaskInfo{ID: 42, Title: "Test", Status: "awaiting-approval"}
	m.detail.SetTask(&task)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}}
	_, cmd := m.handleDetailKey(msg)

	if cmd == nil {
		t.Error("expected command from 't' key in detail view with task, got nil")
	}
}

func TestHandleDetailKey_TNoOpWithNoTask(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(),
		detail: newDetailView(),
		view:   viewDetail,
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}}
	_, cmd := m.handleDetailKey(msg)

	if cmd != nil {
		t.Error("expected no command from 't' key in detail view without task, got non-nil")
	}
}

func TestListView_RendersTmuxIndicator(t *testing.T) {
	l := newListView()
	l.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Task with tmux", Status: "awaiting-approval", CurrentStep: "implement"},
		{ID: 2, Title: "Task without tmux", Status: "running", CurrentStep: "review"},
	})
	l.tmuxSessions = map[int64]bool{1: true}
	l.SetSize(100, 24)

	output := l.View()

	if !strings.Contains(output, "[T]") {
		t.Error("expected task list to contain [T] indicator for task with tmux session")
	}
}

func TestListView_NoTmuxIndicatorWithoutSessions(t *testing.T) {
	l := newListView()
	l.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Task without tmux", Status: "running", CurrentStep: "implement"},
	})
	l.SetSize(100, 24)

	output := l.View()

	if strings.Contains(output, "[T]") {
		t.Error("expected task list to NOT contain [T] indicator when no tmux sessions")
	}
}

func TestHandleListKey_QQuitsApp(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(),
		detail: newDetailView(),
		view:   viewList,
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	result, cmd := m.handleListKey(msg)
	updated := result.(Model)

	if !updated.quitting {
		t.Error("expected quitting to be true")
	}
	if cmd == nil {
		t.Error("expected quit command, got nil")
	}
}
