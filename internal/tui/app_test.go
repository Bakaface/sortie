package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/aface/ralph-tamer-kit/internal/client"
	"github.com/aface/ralph-tamer-kit/internal/config"
	"github.com/aface/ralph-tamer-kit/internal/daemon"
	tea "github.com/charmbracelet/bubbletea"
)

func TestHandleDetailKey_QReturnsToList(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false),
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
		list:   newListView(false),
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
		list:   newListView(false),
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
		list:   newListView(false),
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
		list:   newListView(false),
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
		list:   newListView(false),
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
		list:   newListView(false),
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
		list:   newListView(false),
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
		list:   newListView(false),
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
		list:   newListView(false),
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
	l := newListView(false)
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

func TestListView_RendersTmuxStatus(t *testing.T) {
	l := newListView(false)
	l.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Task with tmux status", Status: "tmux", CurrentStep: "implement"},
	})
	l.SetSize(100, 24)

	output := l.View()

	if !strings.Contains(output, "▣") {
		t.Error("expected task list to contain ▣ icon for tmux status")
	}
}

func TestListView_NoTmuxIndicatorWithoutSessions(t *testing.T) {
	l := newListView(false)
	l.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Task without tmux", Status: "running", CurrentStep: "implement"},
	})
	l.SetSize(100, 24)

	output := l.View()

	if strings.Contains(output, "[T]") {
		t.Error("expected task list to NOT contain [T] indicator when no tmux sessions")
	}
}

func TestHandleKey_ClearsErrorAndProcessesKey(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		client: &client.Client{},
		list:   newListView(false),
		detail: newDetailView(),
		view:   viewList,
		err:    fmt.Errorf("some background error"),
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 26, Title: "Test task", Status: "awaiting-approval"},
	})

	// Press "a" while m.err is set — should clear error AND trigger approve confirmation
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	result, _ := m.handleKey(msg)
	updated := result.(Model)

	if updated.err != nil {
		t.Error("expected error to be cleared")
	}
	if updated.confirmAction != "approve" {
		t.Errorf("expected confirmAction to be 'approve', got %q", updated.confirmAction)
	}
	if updated.confirmTaskID != 26 {
		t.Errorf("expected confirmTaskID to be 26, got %d", updated.confirmTaskID)
	}
}

func TestHandleKey_ClearsErrorOnAnyKey(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false),
		detail: newDetailView(),
		view:   viewList,
		err:    fmt.Errorf("some error"),
	}

	// Press "R" (refresh) while m.err is set — should clear error AND process the key
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}}
	result, cmd := m.handleKey(msg)
	updated := result.(Model)

	if updated.err != nil {
		t.Error("expected error to be cleared")
	}
	// "R" triggers refreshTasks which requires a client, so cmd should be non-nil only with client
	// Without client, it returns nil — that's fine, the important thing is the error was cleared
	// and we didn't just swallow the keypress
	_ = cmd
}

func TestHandleListKey_QQuitsApp(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false),
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

func TestHandleListKey_EnterOpensTaskInfoView(t *testing.T) {
	m := Model{
		keys:     newKeyMap(),
		list:     newListView(false),
		detail:   newDetailView(),
		taskInfo: newTaskInfoView(),
		view:     viewList,
		cfg:      &config.Config{},
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 7, Title: "Test task", Status: "running"},
	})

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	result, cmd := m.handleListKey(msg)
	updated := result.(Model)

	if updated.view != viewTaskInfo {
		t.Errorf("expected view to be viewTaskInfo (%d), got %d", viewTaskInfo, updated.view)
	}
	if updated.taskInfo.task == nil {
		t.Fatal("expected taskInfo.task to be set")
	}
	if updated.taskInfo.task.ID != 7 {
		t.Errorf("expected taskInfo task ID 7, got %d", updated.taskInfo.task.ID)
	}
	if cmd != nil {
		t.Error("expected no command (no log loading for task info), got non-nil")
	}
}

func TestHandleListKey_LOpensLogsView(t *testing.T) {
	m := Model{
		keys:     newKeyMap(),
		list:     newListView(false),
		detail:   newDetailView(),
		taskInfo: newTaskInfoView(),
		view:     viewList,
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 7, Title: "Test task", Status: "running"},
	})

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}}
	result, cmd := m.handleListKey(msg)
	updated := result.(Model)

	if updated.view != viewDetail {
		t.Errorf("expected view to be viewDetail (%d), got %d", viewDetail, updated.view)
	}
	if updated.detail.task == nil {
		t.Fatal("expected detail.task to be set")
	}
	if updated.detail.task.ID != 7 {
		t.Errorf("expected detail task ID 7, got %d", updated.detail.task.ID)
	}
	if !updated.detail.IsFollowMode() {
		t.Error("expected follow mode to be true")
	}
	if cmd == nil {
		t.Error("expected loadOutput command, got nil")
	}
}

func TestHandleTaskInfoKey_QReturnsToList(t *testing.T) {
	m := Model{
		keys:     newKeyMap(),
		list:     newListView(false),
		detail:   newDetailView(),
		taskInfo: newTaskInfoView(),
		view:     viewTaskInfo,
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	result, cmd := m.handleTaskInfoKey(msg)
	updated := result.(Model)

	if updated.view != viewList {
		t.Errorf("expected view to be viewList (%d), got %d", viewList, updated.view)
	}
	if cmd != nil {
		t.Error("expected no command, got non-nil")
	}
}

func TestHandleTaskInfoKey_EscReturnsToList(t *testing.T) {
	m := Model{
		keys:     newKeyMap(),
		list:     newListView(false),
		detail:   newDetailView(),
		taskInfo: newTaskInfoView(),
		view:     viewTaskInfo,
	}

	msg := tea.KeyMsg{Type: tea.KeyEscape}
	result, cmd := m.handleTaskInfoKey(msg)
	updated := result.(Model)

	if updated.view != viewList {
		t.Errorf("expected view to be viewList (%d), got %d", viewList, updated.view)
	}
	if cmd != nil {
		t.Error("expected no command, got non-nil")
	}
}

func TestHandleTaskInfoKey_LOpensLogsView(t *testing.T) {
	m := Model{
		keys:     newKeyMap(),
		list:     newListView(false),
		detail:   newDetailView(),
		taskInfo: newTaskInfoView(),
		view:     viewTaskInfo,
	}
	task := daemon.TaskInfo{ID: 42, Title: "Test", Status: "running"}
	m.taskInfo.SetTask(&task)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}}
	result, cmd := m.handleTaskInfoKey(msg)
	updated := result.(Model)

	if updated.view != viewDetail {
		t.Errorf("expected view to be viewDetail (%d), got %d", viewDetail, updated.view)
	}
	if updated.detail.task == nil {
		t.Fatal("expected detail.task to be set")
	}
	if updated.detail.task.ID != 42 {
		t.Errorf("expected detail task ID 42, got %d", updated.detail.task.ID)
	}
	if !updated.detail.IsFollowMode() {
		t.Error("expected follow mode to be true")
	}
	if cmd == nil {
		t.Error("expected loadOutput command, got nil")
	}
}

func TestTaskInfoView_ShowsMetadata(t *testing.T) {
	v := newTaskInfoView()
	v.SetTask(&daemon.TaskInfo{
		ID:          14,
		Title:       "Test task",
		Description: "Some description",
		Status:      "running",
		Branch:      "rtk/14-test",
		CurrentStep: "implement",
		StepIndex:   0,
		Context:     "some context info",
	})
	v.SetWorkflow(&config.WorkflowConfig{
		Name: "default",
		Steps: []config.StepConfig{
			{Name: "implement", Artifact: true},
			{Name: "review", ApprovalRequired: true},
			{Name: "test"},
		},
	})
	v.SetSize(80, 40)

	output := v.View()

	// Should contain metadata
	if !strings.Contains(output, "#14") {
		t.Error("expected output to contain task ID '#14'")
	}
	if !strings.Contains(output, "Status:") {
		t.Error("expected output to contain 'Status:'")
	}
	if !strings.Contains(output, "running") {
		t.Error("expected output to contain status 'running'")
	}
	if !strings.Contains(output, "Branch:") {
		t.Error("expected output to contain 'Branch:'")
	}
	if !strings.Contains(output, "rtk/14-test") {
		t.Error("expected output to contain branch name")
	}
	if !strings.Contains(output, "Some description") {
		t.Error("expected output to contain description")
	}
	if !strings.Contains(output, "some context info") {
		t.Error("expected output to contain context")
	}

	// Should show workflow progress
	if !strings.Contains(output, "implement") {
		t.Error("expected output to contain step name 'implement'")
	}
	if !strings.Contains(output, "review") {
		t.Error("expected output to contain step name 'review'")
	}
	if !strings.Contains(output, "[approval]") {
		t.Error("expected output to contain '[approval]' indicator")
	}
	if !strings.Contains(output, "[artifact]") {
		t.Error("expected output to contain '[artifact]' indicator")
	}
}

func TestTaskInfoView_NoLogs(t *testing.T) {
	v := newTaskInfoView()
	v.SetTask(&daemon.TaskInfo{
		ID:     14,
		Title:  "Test task",
		Status: "running",
	})
	v.SetSize(80, 24)

	output := v.View()

	// Should NOT contain log-style content or FOLLOW/NORMAL mode
	if strings.Contains(output, "FOLLOW") {
		t.Error("expected task info view to not contain FOLLOW mode indicator")
	}
	if strings.Contains(output, "NORMAL") {
		t.Error("expected task info view to not contain NORMAL mode indicator")
	}
}

func TestListView_GlobalModeTitle(t *testing.T) {
	l := newListView(true)
	l.SetSize(100, 24)
	output := l.View()

	if !strings.Contains(output, "Global") {
		t.Error("expected global mode title to contain 'Global'")
	}
}

func TestListView_GlobalModeShowsProjectColumn(t *testing.T) {
	l := newListView(true)
	l.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Task", Status: "running", ProjectName: "myproject"},
	})
	l.SetSize(120, 24)
	output := l.View()

	if !strings.Contains(output, "PROJECT") {
		t.Error("expected global mode to show PROJECT header column")
	}
	if !strings.Contains(output, "myproject") {
		t.Error("expected global mode to show project name 'myproject'")
	}
}

func TestListView_NonGlobalModeHidesProjectColumn(t *testing.T) {
	l := newListView(false)
	l.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Task", Status: "running", ProjectName: "myproject"},
	})
	l.SetSize(100, 24)
	output := l.View()

	if strings.Contains(output, "PROJECT") {
		t.Error("expected non-global mode to NOT show PROJECT header column")
	}
}
