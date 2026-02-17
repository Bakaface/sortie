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
