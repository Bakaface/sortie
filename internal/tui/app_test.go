package tui

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/aface/sortie/internal/client"
	"github.com/aface/sortie/internal/config"
	"github.com/aface/sortie/internal/daemon"
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
		Branch:       "sortie/14-test",
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
		Branch:      "sortie/14-test",
		CurrentStep: "implement",
		StepIndex:   0,
		Context:     "some context info",
	})
	v.SetWorkflow(&config.WorkflowConfig{
		Name: "default",
		Steps: []config.StepConfig{
			{Name: "implement", Artifact: true},
			{Name: "review", Human: true},
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
	if !strings.Contains(output, "sortie/14-test") {
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
	if !strings.Contains(output, "[human]") {
		t.Error("expected output to contain '[human]' indicator")
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

func TestListView_SortsTasksDescendingByID(t *testing.T) {
	l := newListView(false)
	l.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "First task", Status: "completed"},
		{ID: 3, Title: "Third task", Status: "running"},
		{ID: 2, Title: "Second task", Status: "pending"},
	})

	if len(l.tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(l.tasks))
	}
	if l.tasks[0].ID != 3 {
		t.Errorf("expected first task ID to be 3 (newest), got %d", l.tasks[0].ID)
	}
	if l.tasks[1].ID != 2 {
		t.Errorf("expected second task ID to be 2, got %d", l.tasks[1].ID)
	}
	if l.tasks[2].ID != 1 {
		t.Errorf("expected third task ID to be 1 (oldest), got %d", l.tasks[2].ID)
	}
}

func TestListView_SortsTasksDescendingPreservesCursor(t *testing.T) {
	l := newListView(false)
	// Set initial tasks with cursor at position 0
	l.SetTasks([]daemon.TaskInfo{
		{ID: 5, Title: "Task 5", Status: "running"},
		{ID: 3, Title: "Task 3", Status: "pending"},
	})
	l.cursor = 1 // pointing at task 3

	// Update with same tasks — cursor should stay valid
	l.SetTasks([]daemon.TaskInfo{
		{ID: 3, Title: "Task 3", Status: "pending"},
		{ID: 5, Title: "Task 5", Status: "running"},
	})

	if l.cursor > len(l.tasks)-1 {
		t.Errorf("cursor %d exceeds task count %d", l.cursor, len(l.tasks))
	}
	// Tasks should still be sorted descending
	if l.tasks[0].ID != 5 {
		t.Errorf("expected first task ID to be 5, got %d", l.tasks[0].ID)
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

func TestHandleListKey_CTriggersConfirmForCompletedTask(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		client: &client.Client{},
		list:   newListView(false),
		detail: newDetailView(),
		view:   viewList,
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 10, Title: "Completed task", Status: "completed"},
	})

	// First "c" sets pendingC
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)
	if !updated.pendingC {
		t.Error("expected pendingC to be true after first 'c'")
	}

	// Second "c" triggers continue confirm
	result, cmd := updated.handleListKey(msg)
	updated = result.(Model)

	if updated.confirmAction != "continue" {
		t.Errorf("expected confirmAction to be 'continue', got %q", updated.confirmAction)
	}
	if updated.confirmTaskID != 10 {
		t.Errorf("expected confirmTaskID to be 10, got %d", updated.confirmTaskID)
	}
	if cmd != nil {
		t.Error("expected no command (confirmation pending), got non-nil")
	}
}

func TestHandleListKey_CTriggersConfirmForFailedTask(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		client: &client.Client{},
		list:   newListView(false),
		detail: newDetailView(),
		view:   viewList,
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 11, Title: "Failed task", Status: "failed"},
	})

	// First "c" sets pendingC
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	// Second "c" triggers continue confirm
	result, cmd := updated.handleListKey(msg)
	updated = result.(Model)

	if updated.confirmAction != "continue" {
		t.Errorf("expected confirmAction to be 'continue', got %q", updated.confirmAction)
	}
	if updated.confirmTaskID != 11 {
		t.Errorf("expected confirmTaskID to be 11, got %d", updated.confirmTaskID)
	}
	if cmd != nil {
		t.Error("expected no command (confirmation pending), got non-nil")
	}
}

func TestHandleListKey_CNoOpForRunningTask(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		client: &client.Client{},
		list:   newListView(false),
		detail: newDetailView(),
		view:   viewList,
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 12, Title: "Running task", Status: "running"},
	})

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	if updated.confirmAction != "" {
		t.Errorf("expected no confirmAction for running task, got %q", updated.confirmAction)
	}
}

func TestHandleListKey_CNoOpWithNoClient(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false),
		detail: newDetailView(),
		view:   viewList,
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 13, Title: "Completed task", Status: "completed"},
	})

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	if updated.confirmAction != "" {
		t.Errorf("expected no confirmAction without client, got %q", updated.confirmAction)
	}
}

func newTestModelWithTasks(n int) Model {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false),
		detail: newDetailView(),
		view:   viewList,
	}
	tasks := make([]daemon.TaskInfo, n)
	for i := range tasks {
		tasks[i] = daemon.TaskInfo{ID: int64(i + 1), Title: fmt.Sprintf("Task %d", i+1), Status: "pending"}
	}
	m.list.SetTasks(tasks)
	m.list.SetSize(100, 30) // 30 lines tall → visibleRows = 25, half = 12
	return m
}

func TestHandleListKey_GGGoesToTop(t *testing.T) {
	m := newTestModelWithTasks(20)
	m.list.cursor = 15

	// First "g"
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}
	result, _ := m.handleListKey(msg)
	m = result.(Model)

	if m.list.cursor != 15 {
		t.Errorf("expected cursor to stay at 15 after first 'g', got %d", m.list.cursor)
	}
	if !m.list.IsPendingG() {
		t.Error("expected pendingG to be true after first 'g'")
	}

	// Second "g"
	result, _ = m.handleListKey(msg)
	m = result.(Model)

	if m.list.cursor != 0 {
		t.Errorf("expected cursor at 0 after 'gg', got %d", m.list.cursor)
	}
	if m.list.IsPendingG() {
		t.Error("expected pendingG to be false after 'gg'")
	}
}

func TestHandleListKey_GGResetByOtherKey(t *testing.T) {
	m := newTestModelWithTasks(20)
	m.list.cursor = 10

	// Press "g" then "j" — should NOT go to top, should move down
	gMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}
	result, _ := m.handleListKey(gMsg)
	m = result.(Model)

	jMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	result, _ = m.handleListKey(jMsg)
	m = result.(Model)

	if m.list.cursor != 11 {
		t.Errorf("expected cursor at 11 after g+j, got %d", m.list.cursor)
	}
	if m.list.IsPendingG() {
		t.Error("expected pendingG to be false after non-g key")
	}
}

func TestHandleListKey_ShiftGGoesToBottom(t *testing.T) {
	m := newTestModelWithTasks(20)
	m.list.cursor = 0

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	if updated.list.cursor != 19 {
		t.Errorf("expected cursor at 19 (last task) after 'G', got %d", updated.list.cursor)
	}
}

func TestHandleListKey_CtrlDPageDown(t *testing.T) {
	m := newTestModelWithTasks(30)
	m.list.cursor = 0

	msg := tea.KeyMsg{Type: tea.KeyCtrlD}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	// height=30, visibleRows=25, half=12
	if updated.list.cursor != 12 {
		t.Errorf("expected cursor at 12 after ctrl+d, got %d", updated.list.cursor)
	}
}

func TestHandleListKey_CtrlUPageUp(t *testing.T) {
	m := newTestModelWithTasks(30)
	m.list.cursor = 20

	msg := tea.KeyMsg{Type: tea.KeyCtrlU}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	// height=30, visibleRows=25, half=12
	if updated.list.cursor != 8 {
		t.Errorf("expected cursor at 8 after ctrl+u, got %d", updated.list.cursor)
	}
}

func TestHandleListKey_CtrlDClampsToEnd(t *testing.T) {
	m := newTestModelWithTasks(10)
	m.list.cursor = 8

	msg := tea.KeyMsg{Type: tea.KeyCtrlD}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	if updated.list.cursor != 9 {
		t.Errorf("expected cursor clamped to 9 (last task) after ctrl+d, got %d", updated.list.cursor)
	}
}

func TestHandleListKey_CtrlUClampsToStart(t *testing.T) {
	m := newTestModelWithTasks(10)
	m.list.cursor = 2

	msg := tea.KeyMsg{Type: tea.KeyCtrlU}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	if updated.list.cursor != 0 {
		t.Errorf("expected cursor clamped to 0 after ctrl+u, got %d", updated.list.cursor)
	}
}

func TestListView_GotoTopAndBottom(t *testing.T) {
	l := newListView(false)
	tasks := make([]daemon.TaskInfo, 10)
	for i := range tasks {
		tasks[i] = daemon.TaskInfo{ID: int64(i + 1), Title: fmt.Sprintf("Task %d", i+1), Status: "pending"}
	}
	l.SetTasks(tasks)

	l.GotoBottom()
	if l.cursor != 9 {
		t.Errorf("expected cursor at 9 after GotoBottom, got %d", l.cursor)
	}

	l.GotoTop()
	if l.cursor != 0 {
		t.Errorf("expected cursor at 0 after GotoTop, got %d", l.cursor)
	}
}

func TestListView_PageDownPageUp(t *testing.T) {
	l := newListView(false)
	tasks := make([]daemon.TaskInfo, 30)
	for i := range tasks {
		tasks[i] = daemon.TaskInfo{ID: int64(i + 1), Title: fmt.Sprintf("Task %d", i+1), Status: "pending"}
	}
	l.SetTasks(tasks)
	l.SetSize(100, 30) // visibleRows=25, half=12

	l.PageDown()
	if l.cursor != 12 {
		t.Errorf("expected cursor at 12 after PageDown, got %d", l.cursor)
	}

	l.PageDown()
	if l.cursor != 24 {
		t.Errorf("expected cursor at 24 after second PageDown, got %d", l.cursor)
	}

	l.PageUp()
	if l.cursor != 12 {
		t.Errorf("expected cursor at 12 after PageUp, got %d", l.cursor)
	}
}

func TestListView_ShowsRealTaskID(t *testing.T) {
	l := newListView(false)
	// Use non-sequential IDs to prove the ID column shows real task IDs,
	// not positional indices (e.g., 1, 2, 3).
	l.SetTasks([]daemon.TaskInfo{
		{ID: 42, Title: "First task", Status: "running"},
		{ID: 7, Title: "Second task", Status: "pending"},
		{ID: 137, Title: "Third task", Status: "completed"},
	})
	l.SetSize(100, 24)

	output := l.View()

	if !strings.Contains(output, "#42") {
		t.Error("expected list to show real task ID #42, not a sequential number")
	}
	if !strings.Contains(output, "#7") {
		t.Error("expected list to show real task ID #7, not a sequential number")
	}
	if !strings.Contains(output, "#137") {
		t.Error("expected list to show real task ID #137, not a sequential number")
	}
	// Ensure sequential indices are NOT shown (tasks should not appear as #1, #2, #3)
	// Check that #1, #2, #3 don't appear (they shouldn't since IDs are 42, 7, 137)
	// Note: #1 could appear inside other text, so check more specifically
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		// Skip header line
		if strings.Contains(line, "TITLE") {
			continue
		}
		if strings.Contains(line, "#1 ") && !strings.Contains(line, "#137") {
			t.Error("list should show real task IDs, not sequential indices like #1")
		}
		if strings.Contains(line, "#2 ") {
			t.Error("list should show real task IDs, not sequential indices like #2")
		}
		if strings.Contains(line, "#3 ") {
			t.Error("list should show real task IDs, not sequential indices like #3")
		}
	}
}

func TestHandleListKey_RShowsTaskSelection(t *testing.T) {
	m := Model{
		keys:        newKeyMap(),
		client:      &client.Client{},
		list:        newListView(false),
		detail:      newDetailView(),
		view:        viewList,
		projectPath: "/tmp/test-project",
		cfg: &config.Config{
			Tasks: []config.TaskConfig{
				{Name: "Housekeeping", Description: "Clean up code"},
				{Name: "Security", Description: "Security scan"},
			},
		},
	}
	// Set a non-failed task so "r" doesn't trigger retry
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Running task", Status: "running"},
	})

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}
	result, cmd := m.handleListKey(msg)
	updated := result.(Model)

	if !updated.selectingTask {
		t.Error("expected selectingTask to be true")
	}
	if updated.taskCursor != 0 {
		t.Errorf("expected taskCursor to be 0, got %d", updated.taskCursor)
	}
	if cmd != nil {
		t.Error("expected no command (selection screen shown), got non-nil")
	}
}

func TestHandleListKey_RRetriesFailedTask(t *testing.T) {
	m := Model{
		keys:        newKeyMap(),
		client:      &client.Client{},
		list:        newListView(false),
		detail:      newDetailView(),
		view:        viewList,
		projectPath: "/tmp/test-project",
		cfg: &config.Config{
			Tasks: []config.TaskConfig{
				{Name: "Housekeeping", Description: "Clean up code"},
			},
		},
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 5, Title: "Failed task", Status: "failed"},
	})

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}
	result, cmd := m.handleListKey(msg)
	updated := result.(Model)

	// Should retry, not show task selection
	if updated.selectingTask {
		t.Error("expected selectingTask to be false when retrying failed task")
	}
	if cmd == nil {
		t.Error("expected retry command, got nil")
	}
}

func TestHandleListKey_RRefreshesWithNoTasks(t *testing.T) {
	m := Model{
		keys:        newKeyMap(),
		client:      &client.Client{},
		list:        newListView(false),
		detail:      newDetailView(),
		view:        viewList,
		projectPath: "/tmp/test-project",
		cfg:         &config.Config{},
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Running task", Status: "running"},
	})

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}
	result, cmd := m.handleListKey(msg)
	updated := result.(Model)

	// No predefined tasks configured — should just refresh
	if updated.selectingTask {
		t.Error("expected selectingTask to be false when no predefined tasks")
	}
	if cmd == nil {
		t.Error("expected refresh command, got nil")
	}
}

func TestHandleTaskSelectKey_Navigation(t *testing.T) {
	m := Model{
		keys:          newKeyMap(),
		client:        &client.Client{},
		list:          newListView(false),
		detail:        newDetailView(),
		view:          viewList,
		selectingTask: true,
		taskCursor:    0,
		projectPath:   "/tmp/test",
		cfg: &config.Config{
			Tasks: []config.TaskConfig{
				{Name: "Task A", Description: "Desc A"},
				{Name: "Task B", Description: "Desc B"},
				{Name: "Task C", Description: "Desc C"},
			},
		},
	}

	// Move down
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	result, _ := m.handleTaskSelectKey(msg)
	updated := result.(Model)
	if updated.taskCursor != 1 {
		t.Errorf("expected cursor at 1 after j, got %d", updated.taskCursor)
	}

	// Move down again
	m = updated
	result, _ = m.handleTaskSelectKey(msg)
	updated = result.(Model)
	if updated.taskCursor != 2 {
		t.Errorf("expected cursor at 2 after j, got %d", updated.taskCursor)
	}

	// Move down at bottom — should stay
	m = updated
	result, _ = m.handleTaskSelectKey(msg)
	updated = result.(Model)
	if updated.taskCursor != 2 {
		t.Errorf("expected cursor to stay at 2, got %d", updated.taskCursor)
	}

	// Move up
	m = updated
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}
	result, _ = m.handleTaskSelectKey(msg)
	updated = result.(Model)
	if updated.taskCursor != 1 {
		t.Errorf("expected cursor at 1 after k, got %d", updated.taskCursor)
	}
}

func TestHandleTaskSelectKey_EscCancels(t *testing.T) {
	m := Model{
		keys:          newKeyMap(),
		selectingTask: true,
		taskCursor:    1,
		cfg: &config.Config{
			Tasks: []config.TaskConfig{
				{Name: "Task A"},
			},
		},
	}

	msg := tea.KeyMsg{Type: tea.KeyEscape}
	result, _ := m.handleTaskSelectKey(msg)
	updated := result.(Model)

	if updated.selectingTask {
		t.Error("expected selectingTask to be false after esc")
	}
}

func TestHandleTaskSelectKey_EnterCreatesTask(t *testing.T) {
	m := Model{
		keys:          newKeyMap(),
		client:        &client.Client{},
		list:          newListView(false),
		detail:        newDetailView(),
		view:          viewList,
		selectingTask: true,
		taskCursor:    0,
		projectPath:   "/tmp/test",
		cfg: &config.Config{
			Tasks: []config.TaskConfig{
				{Name: "Housekeeping", Description: "Clean up code"},
			},
		},
	}

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	result, cmd := m.handleTaskSelectKey(msg)
	updated := result.(Model)

	if updated.selectingTask {
		t.Error("expected selectingTask to be false after enter")
	}
	if updated.selectedWorkflow != "task:Housekeeping" {
		t.Errorf("expected selectedWorkflow 'task:Housekeeping', got %q", updated.selectedWorkflow)
	}
	if cmd == nil {
		t.Error("expected create task command, got nil")
	}
}

func TestHandleTaskSelectKey_NumberKeyCreatesTask(t *testing.T) {
	m := Model{
		keys:          newKeyMap(),
		client:        &client.Client{},
		list:          newListView(false),
		detail:        newDetailView(),
		view:          viewList,
		selectingTask: true,
		taskCursor:    0,
		projectPath:   "/tmp/test",
		cfg: &config.Config{
			Tasks: []config.TaskConfig{
				{Name: "First", Description: "First task"},
				{Name: "Second", Description: "Second task"},
			},
		},
	}

	// Press "2" to select second task
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}}
	result, cmd := m.handleTaskSelectKey(msg)
	updated := result.(Model)

	if updated.selectingTask {
		t.Error("expected selectingTask to be false after number key")
	}
	if updated.selectedWorkflow != "task:Second" {
		t.Errorf("expected selectedWorkflow 'task:Second', got %q", updated.selectedWorkflow)
	}
	if cmd == nil {
		t.Error("expected create task command, got nil")
	}
}

func TestHandleTaskSelectKey_UsesNameWhenNoDescription(t *testing.T) {
	m := Model{
		keys:          newKeyMap(),
		client:        &client.Client{},
		list:          newListView(false),
		detail:        newDetailView(),
		view:          viewList,
		selectingTask: true,
		taskCursor:    0,
		projectPath:   "/tmp/test",
		cfg: &config.Config{
			Tasks: []config.TaskConfig{
				{Name: "NoDesc"},
			},
		},
	}

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	result, cmd := m.handleTaskSelectKey(msg)
	updated := result.(Model)

	if updated.selectingTask {
		t.Error("expected selectingTask to be false")
	}
	// When description is empty, the task name is used as description
	if updated.selectedWorkflow != "task:NoDesc" {
		t.Errorf("expected selectedWorkflow 'task:NoDesc', got %q", updated.selectedWorkflow)
	}
	if cmd == nil {
		t.Error("expected create task command, got nil")
	}
}

func TestViewRendersTaskSelection(t *testing.T) {
	m := Model{
		keys:          newKeyMap(),
		list:          newListView(false),
		detail:        newDetailView(),
		view:          viewList,
		selectingTask: true,
		taskCursor:    0,
		cfg: &config.Config{
			Tasks: []config.TaskConfig{
				{Name: "Housekeeping", Description: "Clean up code"},
				{Name: "Security Scan", Description: "Run security audit"},
			},
		},
	}

	output := m.View()

	if !strings.Contains(output, "Run Predefined Task") {
		t.Error("expected task selection screen to contain title 'Run Predefined Task'")
	}
	if !strings.Contains(output, "Housekeeping") {
		t.Error("expected task selection screen to contain 'Housekeeping'")
	}
	if !strings.Contains(output, "Security Scan") {
		t.Error("expected task selection screen to contain 'Security Scan'")
	}
	if !strings.Contains(output, "Clean up code") {
		t.Error("expected task selection screen to show description for selected task")
	}
}

func TestListView_PageWithSmallHeight(t *testing.T) {
	l := newListView(false)
	tasks := make([]daemon.TaskInfo, 10)
	for i := range tasks {
		tasks[i] = daemon.TaskInfo{ID: int64(i + 1), Title: fmt.Sprintf("Task %d", i+1), Status: "pending"}
	}
	l.SetTasks(tasks)
	l.SetSize(100, 6) // visibleRows=1, half=1 (minimum)

	l.PageDown()
	if l.cursor != 1 {
		t.Errorf("expected cursor at 1 with small height, got %d", l.cursor)
	}
}

func TestHandlePromptKey_CtrlGOpensEditor(t *testing.T) {
	m := Model{
		keys:        newKeyMap(),
		client:      &client.Client{},
		prompt:      newPromptView(),
		view:        viewPrompt,
		projectPath: "/tmp/test",
	}
	m.prompt.SetSize(80, 24)

	msg := tea.KeyMsg{Type: tea.KeyCtrlG}
	_, cmd := m.handlePromptKey(msg)

	if cmd == nil {
		t.Error("expected editor command from ctrl+g, got nil")
	}
}

func TestHandlePromptKey_EnterSubmitsTask(t *testing.T) {
	m := Model{
		keys:        newKeyMap(),
		client:      &client.Client{},
		prompt:      newPromptView(),
		view:        viewPrompt,
		projectPath: "/tmp/test",
	}
	m.prompt.SetSize(80, 24)
	m.prompt.textarea.SetValue("test description")

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	result, cmd := m.handlePromptKey(msg)
	updated := result.(Model)

	if updated.view != viewList {
		t.Errorf("expected view to be viewList after enter, got %d", updated.view)
	}
	if cmd == nil {
		t.Error("expected create task command from enter, got nil")
	}
}

func TestHandlePromptKey_EnterEmptyDoesNothing(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		client: &client.Client{},
		prompt: newPromptView(),
		view:   viewPrompt,
	}
	m.prompt.SetSize(80, 24)

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	result, cmd := m.handlePromptKey(msg)
	updated := result.(Model)

	if updated.view != viewPrompt {
		t.Errorf("expected view to remain viewPrompt with empty input, got %d", updated.view)
	}
	if cmd != nil {
		t.Error("expected no command with empty input, got non-nil")
	}
}

func TestHandlePromptKey_EscCancels(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		prompt: newPromptView(),
		view:   viewPrompt,
	}

	msg := tea.KeyMsg{Type: tea.KeyEscape}
	result, cmd := m.handlePromptKey(msg)
	updated := result.(Model)

	if updated.view != viewList {
		t.Errorf("expected view to be viewList after esc, got %d", updated.view)
	}
	if cmd != nil {
		t.Error("expected no command, got non-nil")
	}
}

func TestEditorPromptFinishedMsg_SetsTextareaValue(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		prompt: newPromptView(),
		view:   viewPrompt,
	}
	m.prompt.SetSize(80, 24)

	// Create a temp file with content
	f, err := os.CreateTemp("", "test-editor-*.md")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("edited content from editor")
	f.Close()

	msg := editorPromptFinishedMsg{path: f.Name()}
	result, _ := m.Update(msg)
	updated := result.(Model)

	if updated.prompt.Value() != "edited content from editor" {
		t.Errorf("expected textarea value to be 'edited content from editor', got %q", updated.prompt.Value())
	}
}

func TestEditorPromptFinishedMsg_EmptyFilePreservesTextarea(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		prompt: newPromptView(),
		view:   viewPrompt,
	}
	m.prompt.SetSize(80, 24)
	m.prompt.textarea.SetValue("original content")

	// Create an empty temp file
	f, err := os.CreateTemp("", "test-editor-*.md")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	msg := editorPromptFinishedMsg{path: f.Name()}
	result, _ := m.Update(msg)
	updated := result.(Model)

	// Empty editor content should preserve original textarea value
	if updated.prompt.Value() != "original content" {
		t.Errorf("expected textarea to keep 'original content', got %q", updated.prompt.Value())
	}
}

func TestPromptView_HelpShowsEditorShortcut(t *testing.T) {
	p := newPromptView()
	p.SetSize(100, 24)

	output := p.View()

	if !strings.Contains(output, "ctrl+g") {
		t.Error("expected prompt help to contain 'ctrl+g'")
	}
	if !strings.Contains(output, "editor") {
		t.Error("expected prompt help to contain 'editor'")
	}
}

func TestPromptView_HelpShowsEnterAndCtrlJ(t *testing.T) {
	p := newPromptView()
	p.SetSize(100, 24)

	output := p.View()

	if !strings.Contains(output, "enter") {
		t.Error("expected prompt help to contain 'enter'")
	}
	if !strings.Contains(output, "submit") {
		t.Error("expected prompt help to contain 'submit'")
	}
	if !strings.Contains(output, "ctrl+j") {
		t.Error("expected prompt help to contain 'ctrl+j'")
	}
	if !strings.Contains(output, "newline") {
		t.Error("expected prompt help to contain 'newline'")
	}
	if strings.Contains(output, "ctrl+d") {
		t.Error("expected prompt help to NOT contain 'ctrl+d'")
	}
}

func TestHandlePromptKey_CtrlJInsertsNewline(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		prompt: newPromptView(),
		view:   viewPrompt,
	}
	m.prompt.SetSize(80, 24)
	m.prompt.textarea.SetValue("line one")

	// Move cursor to end of text
	msg := tea.KeyMsg{Type: tea.KeyCtrlE}
	m.prompt.Update(msg)

	// Ctrl+J should insert a newline (handled by textarea's InsertNewline binding)
	msg = tea.KeyMsg{Type: tea.KeyCtrlJ}
	m.prompt.Update(msg)

	val := m.prompt.textarea.Value()
	if !strings.Contains(val, "\n") {
		t.Errorf("expected ctrl+j to insert newline, got %q", val)
	}
}

func TestHandleListKey_CPOpensPrioritySelection(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		client: &client.Client{},
		list:   newListView(false),
		detail: newDetailView(),
		view:   viewList,
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 20, Title: "Test task", Status: "pending", Priority: "medium"},
	})

	// First "c" sets pendingC
	cMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}
	result, _ := m.handleListKey(cMsg)
	updated := result.(Model)
	if !updated.pendingC {
		t.Error("expected pendingC to be true after 'c'")
	}

	// "p" opens priority selection
	pMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}}
	result, _ = updated.handleListKey(pMsg)
	updated = result.(Model)

	if !updated.selectingPriority {
		t.Error("expected selectingPriority to be true after 'cp'")
	}
	if updated.priorityTaskID != 20 {
		t.Errorf("expected priorityTaskID to be 20, got %d", updated.priorityTaskID)
	}
}

func TestHandlePrioritySelectKey_EscCancels(t *testing.T) {
	m := Model{
		keys:              newKeyMap(),
		client:            &client.Client{},
		list:              newListView(false),
		detail:            newDetailView(),
		view:              viewList,
		selectingPriority: true,
		priorityTaskID:    1,
	}

	msg := tea.KeyMsg{Type: tea.KeyEsc}
	result, _ := m.handlePrioritySelectKey(msg)
	updated := result.(Model)

	if updated.selectingPriority {
		t.Error("expected selectingPriority to be false after esc")
	}
}

func TestHandlePrioritySelectKey_Navigation(t *testing.T) {
	m := Model{
		keys:              newKeyMap(),
		client:            &client.Client{},
		list:              newListView(false),
		detail:            newDetailView(),
		view:              viewList,
		selectingPriority: true,
		priorityCursor:    0,
		priorityTaskID:    1,
	}

	// Move down
	downMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	result, _ := m.handlePrioritySelectKey(downMsg)
	updated := result.(Model)
	if updated.priorityCursor != 1 {
		t.Errorf("expected cursor at 1, got %d", updated.priorityCursor)
	}

	// Move up
	upMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}
	result, _ = updated.handlePrioritySelectKey(upMsg)
	updated = result.(Model)
	if updated.priorityCursor != 0 {
		t.Errorf("expected cursor at 0, got %d", updated.priorityCursor)
	}
}

func TestListView_RendersPriorityBadge(t *testing.T) {
	l := newListView(false)
	l.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Urgent task", Status: "pending", Priority: "urgent"},
		{ID: 2, Title: "Low task", Status: "pending", Priority: "low"},
	})
	l.SetSize(120, 30)

	output := l.View()
	if !strings.Contains(output, " P ") {
		t.Error("expected priority header 'P' in list view")
	}
	if !strings.Contains(output, "U") {
		t.Error("expected 'U' badge for urgent task")
	}
	if !strings.Contains(output, "L") {
		t.Error("expected 'L' badge for low task")
	}
}

func TestPriorityBadge(t *testing.T) {
	tests := []struct {
		priority string
		want     string
	}{
		{"urgent", "U"},
		{"high", "H"},
		{"medium", "M"},
		{"low", "L"},
		{"", "M"},
		{"unknown", "M"},
	}
	for _, tt := range tests {
		t.Run(tt.priority, func(t *testing.T) {
			got := priorityBadge(tt.priority)
			if got != tt.want {
				t.Errorf("priorityBadge(%q) = %q, want %q", tt.priority, got, tt.want)
			}
		})
	}
}

func TestTaskInfoView_ShowsPriority(t *testing.T) {
	v := newTaskInfoView()
	v.SetSize(100, 50)
	task := &daemon.TaskInfo{
		ID:       1,
		Title:    "Test task",
		Status:   "pending",
		Priority: "high",
	}
	v.SetTask(task)

	output := v.renderMetadata()
	if !strings.Contains(output, "Priority:") {
		t.Error("expected task info to show priority label")
	}
	if !strings.Contains(output, "high") {
		t.Error("expected task info to show priority value 'high'")
	}
}
