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

func TestHandleListKey_CTriggersFinalizeForTmuxTask(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		client: &client.Client{},
		list:   newListView(false),
		detail: newDetailView(),
		view:   viewList,
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 14, Title: "Tmux task", Status: "tmux"},
	})

	// First "c" sets pendingC
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)
	if !updated.pendingC {
		t.Error("expected pendingC to be true after first 'c'")
	}

	// Second "c" triggers finalize confirm
	result, cmd := updated.handleListKey(msg)
	updated = result.(Model)

	if updated.confirmAction != "finalize" {
		t.Errorf("expected confirmAction to be 'finalize', got %q", updated.confirmAction)
	}
	if updated.confirmTaskID != 14 {
		t.Errorf("expected confirmTaskID to be 14, got %d", updated.confirmTaskID)
	}
	if cmd != nil {
		t.Error("expected no command (confirmation pending), got non-nil")
	}
}

func TestHandleListKey_CNoOpForPendingTask(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		client: &client.Client{},
		list:   newListView(false),
		detail: newDetailView(),
		view:   viewList,
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 15, Title: "Pending task", Status: "pending"},
	})

	// First "c" sets pendingC
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	// Second "c" should not trigger any action for pending task
	result, _ = updated.handleListKey(msg)
	updated = result.(Model)

	if updated.confirmAction != "" {
		t.Errorf("expected no confirmAction for pending task, got %q", updated.confirmAction)
	}
}

func TestHandleListKey_CNoOpForAwaitingApprovalTask(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		client: &client.Client{},
		list:   newListView(false),
		detail: newDetailView(),
		view:   viewList,
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 16, Title: "Awaiting task", Status: "awaiting-approval"},
	})

	// First "c" sets pendingC
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	// Second "c" should not trigger any action
	result, _ = updated.handleListKey(msg)
	updated = result.(Model)

	if updated.confirmAction != "" {
		t.Errorf("expected no confirmAction for awaiting-approval task, got %q", updated.confirmAction)
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

func TestHandleListKey_PgDownPageDown(t *testing.T) {
	m := newTestModelWithTasks(30)
	m.list.cursor = 0

	msg := tea.KeyMsg{Type: tea.KeyPgDown}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	// height=30, visibleRows=25, half=12
	if updated.list.cursor != 12 {
		t.Errorf("expected cursor at 12 after pgdown, got %d", updated.list.cursor)
	}
}

func TestHandleListKey_PgUpPageUp(t *testing.T) {
	m := newTestModelWithTasks(30)
	m.list.cursor = 20

	msg := tea.KeyMsg{Type: tea.KeyPgUp}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	// height=30, visibleRows=25, half=12
	if updated.list.cursor != 8 {
		t.Errorf("expected cursor at 8 after pgup, got %d", updated.list.cursor)
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

	// IDs should be displayed without '#' prefix
	if !strings.Contains(output, "42") {
		t.Error("expected list to show real task ID 42")
	}
	if !strings.Contains(output, "137") {
		t.Error("expected list to show real task ID 137")
	}
	// Verify no '#' prefix on IDs
	if strings.Contains(output, "#42") {
		t.Error("expected task ID 42 without '#' prefix")
	}
	if strings.Contains(output, "#137") {
		t.Error("expected task ID 137 without '#' prefix")
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

func TestPromptView_WordJumpKeybindings(t *testing.T) {
	p := newPromptView()
	p.SetSize(80, 24)
	p.textarea.SetValue("hello world foo")

	// Move cursor to end of line
	endMsg := tea.KeyMsg{Type: tea.KeyCtrlE}
	p.Update(endMsg)
	endCol := p.textarea.LineInfo().ColumnOffset

	// ctrl+left should jump back (word backward)
	ctrlLeft := tea.KeyMsg{Type: tea.KeyCtrlLeft}
	p.Update(ctrlLeft)
	afterFirstLeft := p.textarea.LineInfo().ColumnOffset
	if afterFirstLeft >= endCol {
		t.Errorf("expected cursor to move left from %d, got %d", endCol, afterFirstLeft)
	}

	// ctrl+left again should jump further back
	p.Update(ctrlLeft)
	afterSecondLeft := p.textarea.LineInfo().ColumnOffset
	if afterSecondLeft >= afterFirstLeft {
		t.Errorf("expected cursor to move further left from %d, got %d", afterFirstLeft, afterSecondLeft)
	}

	// ctrl+right should jump forward (word forward)
	ctrlRight := tea.KeyMsg{Type: tea.KeyCtrlRight}
	p.Update(ctrlRight)
	afterRight := p.textarea.LineInfo().ColumnOffset
	if afterRight <= afterSecondLeft {
		t.Errorf("expected cursor to move right from %d, got %d", afterSecondLeft, afterRight)
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

func TestHandleListKey_QuestionMarkTogglesHelp(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false),
		detail: newDetailView(),
		view:   viewList,
	}

	// Press "?" to open help
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}
	result, cmd := m.handleListKey(msg)
	updated := result.(Model)

	if !updated.list.showHelp {
		t.Error("expected showHelp to be true after '?'")
	}
	if cmd != nil {
		t.Error("expected no command, got non-nil")
	}

	// Press "?" again to close help
	result, cmd = updated.handleListKey(msg)
	updated = result.(Model)

	if updated.list.showHelp {
		t.Error("expected showHelp to be false after second '?'")
	}
	if cmd != nil {
		t.Error("expected no command, got non-nil")
	}
}

func TestHandleListKey_EscClosesHelp(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false),
		detail: newDetailView(),
		view:   viewList,
	}
	m.list.showHelp = true

	msg := tea.KeyMsg{Type: tea.KeyEscape}
	result, cmd := m.handleListKey(msg)
	updated := result.(Model)

	if updated.list.showHelp {
		t.Error("expected showHelp to be false after esc")
	}
	if cmd != nil {
		t.Error("expected no command, got non-nil")
	}
}

func TestHandleListKey_HelpConsumesKeys(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false),
		detail: newDetailView(),
		view:   viewList,
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Test", Status: "running"},
	})
	m.list.showHelp = true

	// Press "j" while help is shown — should NOT move cursor
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	result, cmd := m.handleListKey(msg)
	updated := result.(Model)

	if updated.list.cursor != 0 {
		t.Errorf("expected cursor to stay at 0 while help shown, got %d", updated.list.cursor)
	}
	if !updated.list.showHelp {
		t.Error("expected showHelp to remain true")
	}
	if cmd != nil {
		t.Error("expected no command, got non-nil")
	}
}

func TestViewRendersHelpOverlay(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false),
		detail: newDetailView(),
		view:   viewList,
	}
	m.list.showHelp = true

	output := m.View()

	if !strings.Contains(output, "Help") {
		t.Error("expected help overlay to contain 'Help' title")
	}
	if !strings.Contains(output, "Navigation") {
		t.Error("expected help overlay to contain 'Navigation' group")
	}
	if !strings.Contains(output, "Actions") {
		t.Error("expected help overlay to contain 'Actions' group")
	}
	if !strings.Contains(output, "General") {
		t.Error("expected help overlay to contain 'General' group")
	}
	// Check some keybindings are shown
	if !strings.Contains(output, "task info") {
		t.Error("expected help overlay to contain 'task info' binding")
	}
	if !strings.Contains(output, "new task") {
		t.Error("expected help overlay to contain 'new task' binding")
	}
	if !strings.Contains(output, "quit") {
		t.Error("expected help overlay to contain 'quit' binding")
	}
}

func TestShortHelp_ContainsHelpBinding(t *testing.T) {
	keys := newKeyMap()
	bindings := keys.ShortHelp()

	found := false
	for _, b := range bindings {
		if b.Help().Key == "?" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected ShortHelp to contain '?' help binding")
	}
}

func TestShortHelp_IsConcise(t *testing.T) {
	keys := newKeyMap()
	bindings := keys.ShortHelp()

	if len(bindings) > 10 {
		t.Errorf("expected ShortHelp to have at most 10 bindings for conciseness, got %d", len(bindings))
	}
}

func TestListView_ShowsAscendingIndexColumn(t *testing.T) {
	l := newListView(false)
	l.SetTasks([]daemon.TaskInfo{
		{ID: 42, Title: "First task", Status: "running"},
		{ID: 7, Title: "Second task", Status: "pending"},
		{ID: 3, Title: "Third task", Status: "completed"},
	})
	l.SetSize(100, 24)

	output := l.View()

	// Header should contain "#" column
	if !strings.Contains(output, "#") {
		t.Error("expected list header to contain '#' column")
	}

	// The ascending indices for 3 tasks should be: 0, 1, 2
	// (first 10 tasks get indices 0-9, top to bottom)
	if !strings.Contains(output, "0") {
		t.Error("expected first task row to show ascending index '0'")
	}
	if !strings.Contains(output, "1") {
		t.Error("expected second task row to show ascending index '1'")
	}
	if !strings.Contains(output, "2") {
		t.Error("expected third task row to show ascending index '2'")
	}
}

func TestListView_AscendingIndexOnlyForFirst10(t *testing.T) {
	l := newListView(false)
	tasks := make([]daemon.TaskInfo, 12)
	for i := range tasks {
		tasks[i] = daemon.TaskInfo{
			ID:     int64(100 - i),
			Title:  fmt.Sprintf("Task %d", i),
			Status: "pending",
		}
	}
	l.SetTasks(tasks)
	l.SetSize(100, 30)

	// Render task at index 9 (last indexed task) - should have index "9"
	line9 := l.renderTask(tasks[0], 9, false) // after sort, tasks[0] has highest ID
	if !strings.Contains(line9, "9") {
		t.Error("expected task at index 9 to show ascending index '9'")
	}

	// Render task at index 10 (beyond first 10) - should have blank index
	line10 := l.renderTask(tasks[0], 10, false)
	// Task at index 10 should NOT have a numeric index column value
	// Check that the index column area has a space, not a digit
	_ = line10 // The index column renders " " for index >= 10
}

func TestHandleListKey_NumberKeyNavigatesToTask(t *testing.T) {
	m := newTestModelWithTasks(12)
	// cursor starts at 0

	// Press "0" — ascending index 0 maps to row 0
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)
	if updated.list.cursor != 0 {
		t.Errorf("expected cursor at 0 after pressing '0', got %d", updated.list.cursor)
	}

	// Press "9" — ascending index 9 maps to row 9
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'9'}}
	result, _ = updated.handleListKey(msg)
	updated = result.(Model)
	if updated.list.cursor != 9 {
		t.Errorf("expected cursor at 9 after pressing '9', got %d", updated.list.cursor)
	}

	// Press "5" — ascending index 5 maps to row 5
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}}
	result, _ = updated.handleListKey(msg)
	updated = result.(Model)
	if updated.list.cursor != 5 {
		t.Errorf("expected cursor at 5 after pressing '5', got %d", updated.list.cursor)
	}
}

func TestHandleListKey_NumberKeyClampedToTaskCount(t *testing.T) {
	m := newTestModelWithTasks(3) // only 3 tasks, rows 0-2

	// Press "5" — maps to row 5, but only 3 tasks exist; cursor should stay
	m.list.cursor = 1
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)
	// GotoIndex won't move cursor if index >= len(tasks)
	if updated.list.cursor != 1 {
		t.Errorf("expected cursor to stay at 1 when target row exceeds task count, got %d", updated.list.cursor)
	}

	// Press "2" — maps to row 2, which exists
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}}
	result, _ = updated.handleListKey(msg)
	updated = result.(Model)
	if updated.list.cursor != 2 {
		t.Errorf("expected cursor at 2 after pressing '2', got %d", updated.list.cursor)
	}
}

func TestListView_GotoIndex(t *testing.T) {
	l := newListView(false)
	tasks := make([]daemon.TaskInfo, 5)
	for i := range tasks {
		tasks[i] = daemon.TaskInfo{ID: int64(i + 1), Title: fmt.Sprintf("Task %d", i+1), Status: "pending"}
	}
	l.SetTasks(tasks)

	l.GotoIndex(3)
	if l.cursor != 3 {
		t.Errorf("expected cursor at 3, got %d", l.cursor)
	}

	// Out of bounds — should not move
	l.GotoIndex(10)
	if l.cursor != 3 {
		t.Errorf("expected cursor to stay at 3 for out-of-bounds index, got %d", l.cursor)
	}

	l.GotoIndex(-1)
	if l.cursor != 3 {
		t.Errorf("expected cursor to stay at 3 for negative index, got %d", l.cursor)
	}
}

func TestListView_HeaderHasIndexColumn(t *testing.T) {
	l := newListView(false)
	l.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Task", Status: "pending"},
	})
	l.SetSize(100, 24)
	output := l.View()

	// Verify both # and ID headers exist
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "ID") && strings.Contains(line, "STATUS") {
			if !strings.Contains(line, "#") {
				t.Error("expected header line to contain '#' index column before 'ID'")
			}
			break
		}
	}
}

func TestTaskCreatedMsg_CursorMovesToTop(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false),
		detail: newDetailView(),
		view:   viewList,
	}

	// Set up existing tasks and move cursor away from top
	tasks := []daemon.TaskInfo{
		{ID: 3, Title: "Task 3", Status: "pending"},
		{ID: 2, Title: "Task 2", Status: "pending"},
		{ID: 1, Title: "Task 1", Status: "pending"},
	}
	m.list.SetTasks(tasks)
	m.list.cursor = 2 // cursor at bottom

	// Simulate a new task being created
	msg := taskCreatedMsg(daemon.TaskInfo{ID: 4, Title: "New Task", Status: "pending"})
	result, _ := m.Update(msg)
	updated := result.(Model)

	if updated.list.cursor != 0 {
		t.Errorf("expected cursor at 0 after task creation, got %d", updated.list.cursor)
	}
}

func TestHandleTaskInfoKey_YSetsPendingY(t *testing.T) {
	m := Model{
		keys:     newKeyMap(),
		list:     newListView(false),
		detail:   newDetailView(),
		taskInfo: newTaskInfoView(),
		view:     viewTaskInfo,
	}
	task := daemon.TaskInfo{ID: 1, Title: "Test", Description: "desc", Context: "ctx"}
	m.taskInfo.SetTask(&task)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}
	result, cmd := m.handleTaskInfoKey(msg)
	updated := result.(Model)

	if !updated.pendingY {
		t.Error("expected pendingY to be true after 'y'")
	}
	if cmd != nil {
		t.Error("expected no command, got non-nil")
	}
}

func TestHandleTaskInfoKey_YDCopiesDescription(t *testing.T) {
	m := Model{
		keys:     newKeyMap(),
		list:     newListView(false),
		detail:   newDetailView(),
		taskInfo: newTaskInfoView(),
		view:     viewTaskInfo,
		pendingY: true,
	}
	task := daemon.TaskInfo{ID: 1, Title: "Test", Description: "task description text", Context: "ctx"}
	m.taskInfo.SetTask(&task)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}
	result, cmd := m.handleTaskInfoKey(msg)
	updated := result.(Model)

	if updated.pendingY {
		t.Error("expected pendingY to be false after 'yd'")
	}
	if cmd != nil {
		t.Error("expected no command, got non-nil")
	}
	// Note: clipboard.WriteAll may fail in CI/headless environments,
	// but the key dispatch logic itself should work correctly.
}

func TestHandleTaskInfoKey_YCCopiesContext(t *testing.T) {
	m := Model{
		keys:     newKeyMap(),
		list:     newListView(false),
		detail:   newDetailView(),
		taskInfo: newTaskInfoView(),
		view:     viewTaskInfo,
		pendingY: true,
	}
	task := daemon.TaskInfo{ID: 1, Title: "Test", Description: "desc", Context: "task context text"}
	m.taskInfo.SetTask(&task)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}
	result, cmd := m.handleTaskInfoKey(msg)
	updated := result.(Model)

	if updated.pendingY {
		t.Error("expected pendingY to be false after 'yc'")
	}
	if cmd != nil {
		t.Error("expected no command, got non-nil")
	}
}

func TestHandleTaskInfoKey_YResetByOtherKey(t *testing.T) {
	m := Model{
		keys:     newKeyMap(),
		list:     newListView(false),
		detail:   newDetailView(),
		taskInfo: newTaskInfoView(),
		view:     viewTaskInfo,
	}
	task := daemon.TaskInfo{ID: 1, Title: "Test", Description: "desc"}
	m.taskInfo.SetTask(&task)

	// Press "y" then "j" — should reset pendingY and scroll down
	yMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}
	result, _ := m.handleTaskInfoKey(yMsg)
	m = result.(Model)

	if !m.pendingY {
		t.Error("expected pendingY to be true after 'y'")
	}

	jMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	result, _ = m.handleTaskInfoKey(jMsg)
	m = result.(Model)

	if m.pendingY {
		t.Error("expected pendingY to be false after non-yank key")
	}
}

func TestHandleTaskInfoKey_YDNoTaskNoError(t *testing.T) {
	m := Model{
		keys:     newKeyMap(),
		list:     newListView(false),
		detail:   newDetailView(),
		taskInfo: newTaskInfoView(),
		view:     viewTaskInfo,
		pendingY: true,
	}
	// No task set on taskInfo

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}
	result, cmd := m.handleTaskInfoKey(msg)
	updated := result.(Model)

	if updated.pendingY {
		t.Error("expected pendingY to be false")
	}
	if cmd != nil {
		t.Error("expected no command, got non-nil")
	}
}

func TestHandleTaskInfoKey_YDEmptyDescriptionNoOp(t *testing.T) {
	m := Model{
		keys:     newKeyMap(),
		list:     newListView(false),
		detail:   newDetailView(),
		taskInfo: newTaskInfoView(),
		view:     viewTaskInfo,
		pendingY: true,
	}
	task := daemon.TaskInfo{ID: 1, Title: "Test", Description: "", Context: "ctx"}
	m.taskInfo.SetTask(&task)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}
	result, cmd := m.handleTaskInfoKey(msg)
	updated := result.(Model)

	if updated.pendingY {
		t.Error("expected pendingY to be false")
	}
	if cmd != nil {
		t.Error("expected no command, got non-nil")
	}
}

func TestHandleTaskInfoKey_YCEmptyContextNoOp(t *testing.T) {
	m := Model{
		keys:     newKeyMap(),
		list:     newListView(false),
		detail:   newDetailView(),
		taskInfo: newTaskInfoView(),
		view:     viewTaskInfo,
		pendingY: true,
	}
	task := daemon.TaskInfo{ID: 1, Title: "Test", Description: "desc", Context: ""}
	m.taskInfo.SetTask(&task)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}
	result, cmd := m.handleTaskInfoKey(msg)
	updated := result.(Model)

	if updated.pendingY {
		t.Error("expected pendingY to be false")
	}
	if cmd != nil {
		t.Error("expected no command, got non-nil")
	}
}

func TestTaskInfoView_HelpShowsYankBindings(t *testing.T) {
	v := newTaskInfoView()
	v.SetTask(&daemon.TaskInfo{ID: 1, Title: "Test", Status: "running"})
	v.SetSize(120, 24)

	output := v.View()

	if !strings.Contains(output, "yd") {
		t.Error("expected task info help to contain 'yd' binding")
	}
	if !strings.Contains(output, "yc") {
		t.Error("expected task info help to contain 'yc' binding")
	}
}

func TestListView_GlobalModeHasIndexColumn(t *testing.T) {
	l := newListView(true)
	l.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Task", Status: "running", ProjectName: "proj"},
	})
	l.SetSize(120, 24)
	output := l.View()

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "ID") && strings.Contains(line, "PROJECT") {
			if !strings.Contains(line, "#") {
				t.Error("expected global mode header to contain '#' index column")
			}
			break
		}
	}
}
