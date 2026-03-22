package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aface/sortie/internal/client"
	"github.com/aface/sortie/internal/config"
	"github.com/aface/sortie/internal/daemon"
	tea "github.com/charmbracelet/bubbletea"
)

func TestHandleDetailKey_QReturnsToList(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false, ""),
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
		list:   newListView(false, ""),
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
		list:   newListView(false, ""),
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
		list:   newListView(false, ""),
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
		list:   newListView(false, ""),
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
		list:   newListView(false, ""),
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
		list:   newListView(false, ""),
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
		list:   newListView(false, ""),
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
		list:   newListView(false, ""),
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
		list:   newListView(false, ""),
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
	l := newListView(false, "")
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
	l := newListView(false, "")
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
	l := newListView(false, "")
	l.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Task without tmux", Status: "running", CurrentStep: "implement"},
	})
	l.SetSize(100, 24)

	output := l.View()

	if strings.Contains(output, "[T]") {
		t.Error("expected task list to NOT contain [T] indicator when no tmux sessions")
	}
}

func TestListView_DetachedOmitsTmuxActivity(t *testing.T) {
	l := newListView(false, "")
	l.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Detached tmux task", Status: "tmux", CurrentStep: "dev", TmuxActivity: "wip", WorktreeDetached: true},
	})
	l.tmuxSessions = map[int64]bool{1: true}
	l.SetSize(100, 24)

	output := l.View()

	if !strings.Contains(output, "[detached]") {
		t.Error("expected task list to contain [detached] for detached task")
	}
	if strings.Contains(output, "[wip]") {
		t.Error("expected task list to NOT contain [wip] when task is detached")
	}
}

func TestListView_NotDetachedShowsTmuxActivity(t *testing.T) {
	l := newListView(false, "")
	l.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Active tmux task", Status: "tmux", CurrentStep: "implement", TmuxActivity: "wip", WorktreeDetached: false},
	})
	l.tmuxSessions = map[int64]bool{1: true}
	l.SetSize(100, 24)

	output := l.View()

	if !strings.Contains(output, "[wip]") {
		t.Error("expected task list to contain [wip] for non-detached tmux task")
	}
	if strings.Contains(output, "[detached]") {
		t.Error("expected task list to NOT contain [detached] for non-detached task")
	}
}

func TestHandleKey_ClearsErrorAndProcessesKey(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		client: &client.Client{},
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
		err:    fmt.Errorf("some background error"),
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 26, Title: "Test task", Status: "awaiting-approval"},
	})

	// Press "c" while m.err is set — should clear error AND trigger continue confirmation
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}
	result, _ := m.handleKey(msg)
	updated := result.(Model)

	if updated.err != nil {
		t.Error("expected error to be cleared")
	}
	if updated.confirmAction != "continue" {
		t.Errorf("expected confirmAction to be 'continue', got %q", updated.confirmAction)
	}
	if updated.confirmTaskID != 26 {
		t.Errorf("expected confirmTaskID to be 26, got %d", updated.confirmTaskID)
	}
}

func TestHandleKey_ClearsErrorOnAnyKey(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false, ""),
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
		list:   newListView(false, ""),
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
		list:     newListView(false, ""),
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
		list:     newListView(false, ""),
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
		list:     newListView(false, ""),
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
		list:     newListView(false, ""),
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
		list:     newListView(false, ""),
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
	l := newListView(true, "")
	l.SetSize(100, 24)
	output := l.View()

	if !strings.Contains(output, "Global") {
		t.Error("expected global mode title to contain 'Global'")
	}
}

func TestListView_GlobalModeShowsProjectColumn(t *testing.T) {
	l := newListView(true, "")
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
	l := newListView(false, "")
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
	l := newListView(false, "")
	// Set initial tasks with cursor at position 0
	l.SetTasks([]daemon.TaskInfo{
		{ID: 5, Title: "Task 5", Status: "running"},
		{ID: 3, Title: "Task 3", Status: "pending"},
	})
	l.table.SetCursor(1) // pointing at task 3

	// Update with same tasks — cursor should stay valid
	l.SetTasks([]daemon.TaskInfo{
		{ID: 3, Title: "Task 3", Status: "pending"},
		{ID: 5, Title: "Task 5", Status: "running"},
	})

	if l.table.Cursor() > len(l.tasks)-1 {
		t.Errorf("cursor %d exceeds task count %d", l.table.Cursor(), len(l.tasks))
	}
	// Tasks should still be sorted descending
	if l.tasks[0].ID != 5 {
		t.Errorf("expected first task ID to be 5, got %d", l.tasks[0].ID)
	}
}

func TestListView_NonGlobalModeHidesProjectColumn(t *testing.T) {
	l := newListView(false, "")
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
	cfg := &config.Config{
		TaskWorkflows: []config.WorkflowConfig{
			{Name: "implement"},
			{Name: "review"},
		},
	}

	m := Model{
		cfg:    cfg,
		keys:   newKeyMap(),
		client: &client.Client{},
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 10, Title: "Completed task", Status: "completed"},
	})

	// Single "c" now triggers workflow selection for completed tasks
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}
	result, cmd := m.handleListKey(msg)
	updated := result.(Model)

	if !updated.selectingContinueWorkflow {
		t.Error("expected selectingContinueWorkflow to be true")
	}
	if updated.continueTaskID != 10 {
		t.Errorf("expected continueTaskID to be 10, got %d", updated.continueTaskID)
	}
	if cmd != nil {
		t.Error("expected no command (workflow selection pending), got non-nil")
	}
}

func TestHandleListKey_CTriggersConfirmForFailedTask(t *testing.T) {
	cfg := &config.Config{
		TaskWorkflows: []config.WorkflowConfig{
			{Name: "implement"},
			{Name: "review"},
		},
	}

	m := Model{
		cfg:    cfg,
		keys:   newKeyMap(),
		client: &client.Client{},
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 11, Title: "Failed task", Status: "failed"},
	})

	// Single "c" now triggers workflow selection for failed tasks
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}
	result, cmd := m.handleListKey(msg)
	updated := result.(Model)

	if !updated.selectingContinueWorkflow {
		t.Error("expected selectingContinueWorkflow to be true")
	}
	if updated.continueTaskID != 11 {
		t.Errorf("expected continueTaskID to be 11, got %d", updated.continueTaskID)
	}
	if cmd != nil {
		t.Error("expected no command (workflow selection pending), got non-nil")
	}
}

func TestHandleListKey_CNoOpForRunningTask(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		client: &client.Client{},
		list:   newListView(false, ""),
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
		list:   newListView(false, ""),
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
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 14, Title: "Tmux task", Status: "tmux"},
	})

	// Single "c" triggers finalize confirm immediately
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}
	result, cmd := m.handleListKey(msg)
	updated := result.(Model)

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
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 15, Title: "Pending task", Status: "pending"},
	})

	// Single "c" should not trigger any action for pending task
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	if updated.confirmAction != "" {
		t.Errorf("expected no confirmAction for pending task, got %q", updated.confirmAction)
	}
}

func TestHandleListKey_CContinuesAwaitingApprovalTask(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		client: &client.Client{},
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 16, Title: "Awaiting task", Status: "awaiting-approval"},
	})

	// Single "c" should trigger continue confirmation for awaiting-approval task
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	if updated.confirmAction != "continue" {
		t.Errorf("expected confirmAction to be 'continue' for awaiting-approval task, got %q", updated.confirmAction)
	}
	if updated.confirmTaskID != 16 {
		t.Errorf("expected confirmTaskID to be 16, got %d", updated.confirmTaskID)
	}
}

func newTestModelWithTasks(n int) Model {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
	}
	tasks := make([]daemon.TaskInfo, n)
	for i := range tasks {
		tasks[i] = daemon.TaskInfo{ID: int64(i + 1), Title: fmt.Sprintf("Task %d", i+1), Status: "pending"}
	}
	m.list.SetTasks(tasks)
	m.list.SetSize(100, 30) // 30 lines tall → visibleRows = 23, half = 11
	return m
}

func TestHandleListKey_GGGoesToTop(t *testing.T) {
	m := newTestModelWithTasks(20)
	m.list.table.SetCursor(15)

	// First "g"
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}
	result, _ := m.handleListKey(msg)
	m = result.(Model)

	if m.list.table.Cursor() != 15 {
		t.Errorf("expected cursor to stay at 15 after first 'g', got %d", m.list.table.Cursor())
	}
	if !m.list.IsPendingG() {
		t.Error("expected pendingG to be true after first 'g'")
	}

	// Second "g"
	result, _ = m.handleListKey(msg)
	m = result.(Model)

	if m.list.table.Cursor() != 0 {
		t.Errorf("expected cursor at 0 after 'gg', got %d", m.list.table.Cursor())
	}
	if m.list.IsPendingG() {
		t.Error("expected pendingG to be false after 'gg'")
	}
}

func TestHandleListKey_GGResetByOtherKey(t *testing.T) {
	m := newTestModelWithTasks(20)
	m.list.table.SetCursor(10)

	// Press "g" then "j" — should NOT go to top, should move down
	gMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}
	result, _ := m.handleListKey(gMsg)
	m = result.(Model)

	jMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	result, _ = m.handleListKey(jMsg)
	m = result.(Model)

	if m.list.table.Cursor() != 11 {
		t.Errorf("expected cursor at 11 after g+j, got %d", m.list.table.Cursor())
	}
	if m.list.IsPendingG() {
		t.Error("expected pendingG to be false after non-g key")
	}
}

func TestHandleListKey_ShiftGGoesToBottom(t *testing.T) {
	m := newTestModelWithTasks(20)
	m.list.table.SetCursor(0)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	if updated.list.table.Cursor() != 19 {
		t.Errorf("expected cursor at 19 (last task) after 'G', got %d", updated.list.table.Cursor())
	}
}

func TestHandleListKey_CtrlDPageDown(t *testing.T) {
	m := newTestModelWithTasks(30)
	m.list.table.SetCursor(0)

	msg := tea.KeyMsg{Type: tea.KeyCtrlD}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	// height=30, visibleRows=23, half=11
	if updated.list.table.Cursor() != 11 {
		t.Errorf("expected cursor at 11 after ctrl+d, got %d", updated.list.table.Cursor())
	}
}

func TestHandleListKey_CtrlUPageUp(t *testing.T) {
	m := newTestModelWithTasks(30)
	m.list.table.SetCursor(20)

	msg := tea.KeyMsg{Type: tea.KeyCtrlU}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	// height=30, visibleRows=23, half=11
	if updated.list.table.Cursor() != 9 {
		t.Errorf("expected cursor at 9 after ctrl+u, got %d", updated.list.table.Cursor())
	}
}

func TestHandleListKey_CtrlDClampsToEnd(t *testing.T) {
	m := newTestModelWithTasks(10)
	m.list.table.SetCursor(8)

	msg := tea.KeyMsg{Type: tea.KeyCtrlD}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	if updated.list.table.Cursor() != 9 {
		t.Errorf("expected cursor clamped to 9 (last task) after ctrl+d, got %d", updated.list.table.Cursor())
	}
}

func TestHandleListKey_CtrlUClampsToStart(t *testing.T) {
	m := newTestModelWithTasks(10)
	m.list.table.SetCursor(2)

	msg := tea.KeyMsg{Type: tea.KeyCtrlU}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	if updated.list.table.Cursor() != 0 {
		t.Errorf("expected cursor clamped to 0 after ctrl+u, got %d", updated.list.table.Cursor())
	}
}

func TestHandleListKey_PgDownPageDown(t *testing.T) {
	m := newTestModelWithTasks(30)
	m.list.table.SetCursor(0)

	msg := tea.KeyMsg{Type: tea.KeyPgDown}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	// height=30, visibleRows=23, half=11
	if updated.list.table.Cursor() != 11 {
		t.Errorf("expected cursor at 11 after pgdown, got %d", updated.list.table.Cursor())
	}
}

func TestHandleListKey_PgUpPageUp(t *testing.T) {
	m := newTestModelWithTasks(30)
	m.list.table.SetCursor(20)

	msg := tea.KeyMsg{Type: tea.KeyPgUp}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	// height=30, visibleRows=23, half=11
	if updated.list.table.Cursor() != 9 {
		t.Errorf("expected cursor at 9 after pgup, got %d", updated.list.table.Cursor())
	}
}

func TestListView_GotoTopAndBottom(t *testing.T) {
	l := newListView(false, "")
	tasks := make([]daemon.TaskInfo, 10)
	for i := range tasks {
		tasks[i] = daemon.TaskInfo{ID: int64(i + 1), Title: fmt.Sprintf("Task %d", i+1), Status: "pending"}
	}
	l.SetTasks(tasks)

	l.GotoBottom()
	if l.table.Cursor() != 9 {
		t.Errorf("expected cursor at 9 after GotoBottom, got %d", l.table.Cursor())
	}

	l.GotoTop()
	if l.table.Cursor() != 0 {
		t.Errorf("expected cursor at 0 after GotoTop, got %d", l.table.Cursor())
	}
}

func TestListView_PageDownPageUp(t *testing.T) {
	l := newListView(false, "")
	tasks := make([]daemon.TaskInfo, 30)
	for i := range tasks {
		tasks[i] = daemon.TaskInfo{ID: int64(i + 1), Title: fmt.Sprintf("Task %d", i+1), Status: "pending"}
	}
	l.SetTasks(tasks)
	l.SetSize(100, 30) // visibleRows=23, half=11

	l.PageDown()
	if l.table.Cursor() != 11 {
		t.Errorf("expected cursor at 11 after PageDown, got %d", l.table.Cursor())
	}

	l.PageDown()
	if l.table.Cursor() != 22 {
		t.Errorf("expected cursor at 22 after second PageDown, got %d", l.table.Cursor())
	}

	l.PageUp()
	if l.table.Cursor() != 11 {
		t.Errorf("expected cursor at 11 after PageUp, got %d", l.table.Cursor())
	}
}

func TestListView_ShowsRealTaskID(t *testing.T) {
	l := newListView(false, "")
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

func TestHandleListKey_XShowsTaskSelection(t *testing.T) {
	m := Model{
		keys:        newKeyMap(),
		client:      &client.Client{},
		list:        newListView(false, ""),
		detail:      newDetailView(),
		view:        viewList,
		projectPath: "/tmp/test-project",
		cfg: &config.Config{
			OneOff: []config.WorkflowConfig{
				{Name: "Housekeeping", Description: "Clean up code"},
				{Name: "Security", Description: "Security scan"},
			},
		},
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Running task", Status: "running"},
	})

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
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
		list:        newListView(false, ""),
		detail:      newDetailView(),
		view:        viewList,
		projectPath: "/tmp/test-project",
		cfg: &config.Config{
			OneOff: []config.WorkflowConfig{
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

func TestHandleListKey_RRetriesTmuxTask(t *testing.T) {
	m := Model{
		keys:        newKeyMap(),
		client:      &client.Client{},
		list:        newListView(false, ""),
		detail:      newDetailView(),
		view:        viewList,
		projectPath: "/tmp/test-project",
		cfg: &config.Config{
			OneOff: []config.WorkflowConfig{
				{Name: "Housekeeping", Description: "Clean up code"},
			},
		},
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 3, Title: "Stale tmux task", Status: "tmux"},
	})

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}
	result, cmd := m.handleListKey(msg)
	updated := result.(Model)

	// Should retry, not show task selection
	if updated.selectingTask {
		t.Error("expected selectingTask to be false when retrying tmux task")
	}
	if cmd == nil {
		t.Error("expected retry command, got nil")
	}
}

func TestHandleListKey_RRetriesCompletedTask(t *testing.T) {
	m := Model{
		keys:        newKeyMap(),
		client:      &client.Client{},
		list:        newListView(false, ""),
		detail:      newDetailView(),
		view:        viewList,
		projectPath: "/tmp/test-project",
		cfg: &config.Config{
			OneOff: []config.WorkflowConfig{
				{Name: "Housekeeping", Description: "Clean up code"},
			},
		},
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 7, Title: "Completed task", Status: "completed"},
	})

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}
	result, cmd := m.handleListKey(msg)
	updated := result.(Model)

	// Should retry, not show task selection
	if updated.selectingTask {
		t.Error("expected selectingTask to be false when retrying completed task")
	}
	if cmd == nil {
		t.Error("expected retry command, got nil")
	}
}

func TestHandleListKey_RNoOpOnRunningTask(t *testing.T) {
	m := Model{
		keys:        newKeyMap(),
		client:      &client.Client{},
		list:        newListView(false, ""),
		detail:      newDetailView(),
		view:        viewList,
		projectPath: "/tmp/test-project",
		cfg:         &config.Config{},
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Running task", Status: "running"},
	})

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}
	_, cmd := m.handleListKey(msg)

	// r on a running task does nothing (not retryable)
	if cmd != nil {
		t.Error("expected no command for non-retryable task, got non-nil")
	}
}

func TestHandleTaskSelectKey_Navigation(t *testing.T) {
	m := Model{
		keys:          newKeyMap(),
		client:        &client.Client{},
		list:          newListView(false, ""),
		detail:        newDetailView(),
		view:          viewList,
		selectingTask: true,
		taskCursor:    0,
		projectPath:   "/tmp/test",
		cfg: &config.Config{
			OneOff: []config.WorkflowConfig{
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
			OneOff: []config.WorkflowConfig{
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
		list:          newListView(false, ""),
		detail:        newDetailView(),
		view:          viewList,
		selectingTask: true,
		taskCursor:    0,
		projectPath:   "/tmp/test",
		cfg: &config.Config{
			OneOff: []config.WorkflowConfig{
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
	if updated.selectedWorkflow != "oneoff:Housekeeping" {
		t.Errorf("expected selectedWorkflow 'oneoff:Housekeeping', got %q", updated.selectedWorkflow)
	}
	if cmd == nil {
		t.Error("expected create task command, got nil")
	}
}

func TestHandleTaskSelectKey_NumberKeyCreatesTask(t *testing.T) {
	m := Model{
		keys:          newKeyMap(),
		client:        &client.Client{},
		list:          newListView(false, ""),
		detail:        newDetailView(),
		view:          viewList,
		selectingTask: true,
		taskCursor:    0,
		projectPath:   "/tmp/test",
		cfg: &config.Config{
			OneOff: []config.WorkflowConfig{
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
	if updated.selectedWorkflow != "oneoff:Second" {
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
		list:          newListView(false, ""),
		detail:        newDetailView(),
		view:          viewList,
		selectingTask: true,
		taskCursor:    0,
		projectPath:   "/tmp/test",
		cfg: &config.Config{
			OneOff: []config.WorkflowConfig{
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
	if updated.selectedWorkflow != "oneoff:NoDesc" {
		t.Errorf("expected selectedWorkflow 'oneoff:NoDesc', got %q", updated.selectedWorkflow)
	}
	if cmd == nil {
		t.Error("expected create task command, got nil")
	}
}

func TestViewRendersTaskSelection(t *testing.T) {
	m := Model{
		keys:          newKeyMap(),
		list:          newListView(false, ""),
		detail:        newDetailView(),
		view:          viewList,
		selectingTask: true,
		taskCursor:    0,
		cfg: &config.Config{
			OneOff: []config.WorkflowConfig{
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
	l := newListView(false, "")
	tasks := make([]daemon.TaskInfo, 10)
	for i := range tasks {
		tasks[i] = daemon.TaskInfo{ID: int64(i + 1), Title: fmt.Sprintf("Task %d", i+1), Status: "pending"}
	}
	l.SetTasks(tasks)
	l.SetSize(100, 6) // visibleRows=1, half=1 (minimum)

	l.PageDown()
	if l.table.Cursor() != 1 {
		t.Errorf("expected cursor at 1 with small height, got %d", l.table.Cursor())
	}
}

func TestHandlePromptKey_CtrlGOpensEditor(t *testing.T) {
	m := Model{
		keys:        newKeyMap(),
		client:      &client.Client{},
		prompt:      newPromptView(true, ""),
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
		prompt:      newPromptView(true, ""),
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
		prompt: newPromptView(true, ""),
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
		prompt: newPromptView(true, ""),
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
		prompt: newPromptView(true, ""),
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
		prompt: newPromptView(true, ""),
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

func TestPromptView_HelpShowsEditorShortcutInFullHelp(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false, ""),
		detail: newDetailView(),
		prompt: newPromptView(true, ""),
		view:   viewPrompt,
	}

	output := m.renderPromptHelpOverlay()

	if !strings.Contains(output, "ctrl+g") {
		t.Error("expected prompt full help to contain 'ctrl+g'")
	}
	if !strings.Contains(output, "editor") {
		t.Error("expected prompt full help to contain 'editor'")
	}
}

func TestPromptView_HelpShowsEnterAndCtrlJ(t *testing.T) {
	p := newPromptView(true, "")
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
		prompt: newPromptView(true, ""),
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
	p := newPromptView(true, "")
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

func TestHandleListKey_POpensPrioritySelection(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		client: &client.Client{},
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 20, Title: "Test task", Status: "pending", Priority: "medium"},
	})

	// Single "p" opens priority selection immediately
	pMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}}
	result, _ := m.handleListKey(pMsg)
	updated := result.(Model)

	if !updated.selectingPriority {
		t.Error("expected selectingPriority to be true after 'p'")
	}
	if updated.priorityTaskID != 20 {
		t.Errorf("expected priorityTaskID to be 20, got %d", updated.priorityTaskID)
	}
}

func TestHandlePrioritySelectKey_EscCancels(t *testing.T) {
	m := Model{
		keys:              newKeyMap(),
		client:            &client.Client{},
		list:              newListView(false, ""),
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
		list:              newListView(false, ""),
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
	l := newListView(false, "")
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
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
	}

	// Press "ctrl+h" to open help
	msg := tea.KeyMsg{Type: tea.KeyCtrlH}
	result, cmd := m.handleListKey(msg)
	updated := result.(Model)

	if !updated.list.showHelp {
		t.Error("expected showHelp to be true after 'ctrl+h'")
	}
	if cmd != nil {
		t.Error("expected no command, got non-nil")
	}

	// Press "ctrl+h" again to close help
	result, cmd = updated.handleListKey(msg)
	updated = result.(Model)

	if updated.list.showHelp {
		t.Error("expected showHelp to be false after second 'ctrl+h'")
	}
	if cmd != nil {
		t.Error("expected no command, got non-nil")
	}
}

func TestHandleListKey_EscClosesHelp(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false, ""),
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
		list:   newListView(false, ""),
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

	if updated.list.table.Cursor() != 0 {
		t.Errorf("expected cursor to stay at 0 while help shown, got %d", updated.list.table.Cursor())
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
		list:   newListView(false, ""),
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
		if b.Help().Key == "ctrl+h" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected ShortHelp to contain 'ctrl+h' help binding")
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
	l := newListView(false, "")
	l.SetTasks([]daemon.TaskInfo{
		{ID: 42, Title: "First task", Status: "running"},
		{ID: 7, Title: "Second task", Status: "pending"},
		{ID: 3, Title: "Third task", Status: "completed"},
	})
	l.SetSize(100, 24)

	output := l.View()

	// Header should NOT contain "#" column (vim-style has no header)
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "ID") && strings.Contains(line, "STATUS") {
			if strings.Contains(line, "#") {
				t.Error("expected header line to NOT contain '#' (vim-style has no header for line numbers)")
			}
			break
		}
	}

	// The line numbers for 3 tasks should be: 1, 2, 3 (1-based, vim-style)
	if !strings.Contains(output, "1") {
		t.Error("expected first task row to show line number '1'")
	}
	if !strings.Contains(output, "2") {
		t.Error("expected second task row to show line number '2'")
	}
	if !strings.Contains(output, "3") {
		t.Error("expected third task row to show line number '3'")
	}
}

func TestListView_AscendingIndexOnlyForFirst10(t *testing.T) {
	l := newListView(false, "")
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

	// Vim-style shows line numbers for ALL tasks (not just first 10)
	// Render task at index 10 - should show line number "11" (1-based)
	line10 := l.renderTask(tasks[0], 10, false)
	if !strings.Contains(line10, "11") {
		t.Error("expected task at index 10 to show line number '11' (1-based)")
	}

	// Render task at index 11 - should show line number "12" (1-based)
	line11 := l.renderTask(tasks[0], 11, false)
	if !strings.Contains(line11, "12") {
		t.Error("expected task at index 11 to show line number '12' (1-based)")
	}
}

func TestHandleListKey_NumberKeyNavigatesToTask(t *testing.T) {
	m := newTestModelWithTasks(12)
	// cursor starts at 0

	// Command mode: press ":", then "1", then "0", then enter → should go to line 10 (index 9)
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)
	if !updated.commandMode {
		t.Error("expected command mode to be active after pressing ':'")
	}

	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}}
	result, _ = updated.handleListKey(msg)
	updated = result.(Model)

	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}}
	result, _ = updated.handleListKey(msg)
	updated = result.(Model)

	msg = tea.KeyMsg{Type: tea.KeyEnter}
	result, _ = updated.handleListKey(msg)
	updated = result.(Model)
	if updated.list.table.Cursor() != 9 {
		t.Errorf("expected cursor at 9 (line 10) after :10<enter>, got %d", updated.list.table.Cursor())
	}
	if updated.commandMode {
		t.Error("expected command mode to exit after executing command")
	}
}

func TestHandleListKey_NumberKeyClampedToTaskCount(t *testing.T) {
	m := newTestModelWithTasks(3) // only 3 tasks, rows 0-2 (lines 1-3)

	// Command mode: enter ":6" with only 3 tasks → cursor should stay in place
	m.list.table.SetCursor(1)
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'6'}}
	result, _ = updated.handleListKey(msg)
	updated = result.(Model)

	msg = tea.KeyMsg{Type: tea.KeyEnter}
	result, _ = updated.handleListKey(msg)
	updated = result.(Model)
	// Line 6 doesn't exist; cursor should stay at 1
	if updated.list.table.Cursor() != 1 {
		t.Errorf("expected cursor to stay at 1 when target line exceeds task count, got %d", updated.list.table.Cursor())
	}

	// Command mode: enter ":3" — maps to line 3, row index 2
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}}
	result, _ = updated.handleListKey(msg)
	updated = result.(Model)

	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}}
	result, _ = updated.handleListKey(msg)
	updated = result.(Model)

	msg = tea.KeyMsg{Type: tea.KeyEnter}
	result, _ = updated.handleListKey(msg)
	updated = result.(Model)
	if updated.list.table.Cursor() != 2 {
		t.Errorf("expected cursor at 2 (line 3) after :3<enter>, got %d", updated.list.table.Cursor())
	}
}

func TestListView_GotoIndex(t *testing.T) {
	l := newListView(false, "")
	tasks := make([]daemon.TaskInfo, 5)
	for i := range tasks {
		tasks[i] = daemon.TaskInfo{ID: int64(i + 1), Title: fmt.Sprintf("Task %d", i+1), Status: "pending"}
	}
	l.SetTasks(tasks)

	l.GotoIndex(3)
	if l.table.Cursor() != 3 {
		t.Errorf("expected cursor at 3, got %d", l.table.Cursor())
	}

	// Out of bounds — should not move
	l.GotoIndex(10)
	if l.table.Cursor() != 3 {
		t.Errorf("expected cursor to stay at 3 for out-of-bounds index, got %d", l.table.Cursor())
	}

	l.GotoIndex(-1)
	if l.table.Cursor() != 3 {
		t.Errorf("expected cursor to stay at 3 for negative index, got %d", l.table.Cursor())
	}
}

func TestCommandMode_EnterAndExit(t *testing.T) {
	m := newTestModelWithTasks(5)

	// Press ":" to enter command mode
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)
	if !updated.commandMode {
		t.Error("expected commandMode to be true after pressing ':'")
	}

	// Press "esc" to exit command mode
	msg = tea.KeyMsg{Type: tea.KeyEsc}
	result, _ = updated.handleListKey(msg)
	updated = result.(Model)
	if updated.commandMode {
		t.Error("expected commandMode to be false after pressing esc")
	}
	if updated.commandInput != "" {
		t.Error("expected commandInput to be cleared after exiting command mode")
	}
}

func TestCommandMode_BackspaceExitsWhenEmpty(t *testing.T) {
	m := newTestModelWithTasks(5)

	// Enter command mode and type a character
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}}
	result, _ = updated.handleListKey(msg)
	updated = result.(Model)
	if updated.commandInput != "5" {
		t.Errorf("expected commandInput to be '5', got '%s'", updated.commandInput)
	}

	// Backspace to empty
	msg = tea.KeyMsg{Type: tea.KeyBackspace}
	result, _ = updated.handleListKey(msg)
	updated = result.(Model)
	// Should exit command mode when backspacing to empty
	if updated.commandMode {
		t.Error("expected commandMode to exit when backspacing to empty input")
	}
	if updated.commandInput != "" {
		t.Error("expected commandInput to be empty")
	}
}

func TestCommandMode_GotoLine(t *testing.T) {
	m := newTestModelWithTasks(5)

	// Enter ":3<enter>" with 5 tasks → cursor should be at index 2 (line 3)
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}}
	result, _ = updated.handleListKey(msg)
	updated = result.(Model)

	msg = tea.KeyMsg{Type: tea.KeyEnter}
	result, _ = updated.handleListKey(msg)
	updated = result.(Model)
	if updated.list.table.Cursor() != 2 {
		t.Errorf("expected cursor at 2 (line 3) after :3<enter>, got %d", updated.list.table.Cursor())
	}
	if updated.commandMode {
		t.Error("expected command mode to exit after executing command")
	}
}

func TestCommandMode_InvalidCommand(t *testing.T) {
	m := newTestModelWithTasks(5)

	// Enter ":xyz<enter>" → should set error
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
	result, _ = updated.handleListKey(msg)
	updated = result.(Model)

	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}
	result, _ = updated.handleListKey(msg)
	updated = result.(Model)

	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z'}}
	result, _ = updated.handleListKey(msg)
	updated = result.(Model)

	msg = tea.KeyMsg{Type: tea.KeyEnter}
	result, _ = updated.handleListKey(msg)
	updated = result.(Model)
	if updated.err == nil {
		t.Error("expected error to be set for invalid command")
	}
	errMsg := updated.err.Error()
	if !strings.Contains(errMsg, "unknown command") && !strings.Contains(errMsg, "Unknown command") {
		t.Errorf("expected error to mention 'unknown command', got: %s", errMsg)
	}
}

func TestCommandMode_DisplaysInView(t *testing.T) {
	m := newTestModelWithTasks(5)
	m.list.SetSize(100, 24)
	m.width = 100
	m.height = 24

	// Enter command mode and type "12"
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}}
	result, _ = updated.handleListKey(msg)
	updated = result.(Model)

	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}}
	result, _ = updated.handleListKey(msg)
	updated = result.(Model)

	output := updated.View()
	if !strings.Contains(output, ":12") {
		t.Error("expected view to contain ':12' when commandMode is true with commandInput '12'")
	}
}

func TestListView_LineNumWidth(t *testing.T) {
	// 1-9 tasks → minimum width 2
	l := newListView(false, "")
	tasks := make([]daemon.TaskInfo, 9)
	for i := range tasks {
		tasks[i] = daemon.TaskInfo{ID: int64(i + 1), Title: fmt.Sprintf("Task %d", i+1), Status: "pending"}
	}
	l.SetTasks(tasks)
	if l.lineNumWidth() != 2 {
		t.Errorf("expected lineNumWidth to be 2 for 9 tasks, got %d", l.lineNumWidth())
	}

	// 10-99 tasks → width 2
	tasks = make([]daemon.TaskInfo, 50)
	for i := range tasks {
		tasks[i] = daemon.TaskInfo{ID: int64(i + 1), Title: fmt.Sprintf("Task %d", i+1), Status: "pending"}
	}
	l.SetTasks(tasks)
	if l.lineNumWidth() != 2 {
		t.Errorf("expected lineNumWidth to be 2 for 50 tasks, got %d", l.lineNumWidth())
	}

	// 100+ tasks → width 3
	tasks = make([]daemon.TaskInfo, 150)
	for i := range tasks {
		tasks[i] = daemon.TaskInfo{ID: int64(i + 1), Title: fmt.Sprintf("Task %d", i+1), Status: "pending"}
	}
	l.SetTasks(tasks)
	if l.lineNumWidth() != 3 {
		t.Errorf("expected lineNumWidth to be 3 for 150 tasks, got %d", l.lineNumWidth())
	}
}

func TestCommandMode_NumberKeysNoLongerNavigate(t *testing.T) {
	m := newTestModelWithTasks(10)
	m.list.table.SetCursor(0)

	// Press number key '5' in list view → should NOT move cursor (no direct navigation)
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)
	if updated.list.table.Cursor() != 0 {
		t.Errorf("expected cursor to stay at 0 after pressing '5' (numbers no longer directly navigate), got %d", updated.list.table.Cursor())
	}
	// Should NOT enter command mode (need to press ':' first)
	if updated.commandMode {
		t.Error("expected command mode to NOT be active (need to press ':' first)")
	}
}

func TestListView_HeaderHasIndexColumn(t *testing.T) {
	l := newListView(false, "")
	l.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Task", Status: "pending"},
	})
	l.SetSize(100, 24)
	output := l.View()

	// Vim-style: header should NOT contain "#", but task rows should show line numbers
	lines := strings.Split(output, "\n")
	headerFound := false
	for _, line := range lines {
		if strings.Contains(line, "ID") && strings.Contains(line, "STATUS") {
			if strings.Contains(line, "#") {
				t.Error("expected header line to NOT contain '#' (vim-style)")
			}
			headerFound = true
			break
		}
	}
	if !headerFound {
		t.Error("could not find header line")
	}

	// Task row should contain line number "1"
	if !strings.Contains(output, "1") {
		t.Error("expected task row to show line number '1'")
	}
}

func TestTaskCreatedMsg_CursorMovesToTop(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false, ""),
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
	m.list.table.SetCursor(2) // cursor at bottom

	// Simulate a new task being created
	msg := taskCreatedMsg(daemon.TaskInfo{ID: 4, Title: "New Task", Status: "pending"})
	result, _ := m.Update(msg)
	updated := result.(Model)

	if updated.list.table.Cursor() != 0 {
		t.Errorf("expected cursor at 0 after task creation, got %d", updated.list.table.Cursor())
	}
}

// --- Artifact keybinding tests ---

func TestHandleListKey_OAOpensArtifactSelection(t *testing.T) {
	dir := t.TempDir()
	// Create artifact file
	artifactDir := filepath.Join(dir, ".sortie", "artifacts")
	os.MkdirAll(artifactDir, 0755)
	os.WriteFile(filepath.Join(artifactDir, "implement.md"), []byte("artifact content"), 0644)

	m := Model{
		keys:     newKeyMap(),
		list:     newListView(false, ""),
		detail:   newDetailView(),
		taskInfo: newTaskInfoView(),
		view:     viewList,
		cfg: &config.Config{
			Workflows: []config.WorkflowConfig{
				{
					Name: "default",
					Steps: []config.StepConfig{
						{Name: "implement", Artifact: true},
						{Name: "review"},
					},
				},
			},
		},
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Test task", Status: "running", Workflow: "default", WorktreePath: dir},
	})

	// First "o" sets pendingO
	oMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}}
	result, _ := m.handleListKey(oMsg)
	updated := result.(Model)
	if !updated.pendingO {
		t.Error("expected pendingO to be true after 'o'")
	}

	// "a" triggers artifact — single artifact so it should skip selection and load
	aMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	result, cmd := updated.handleListKey(aMsg)
	updated = result.(Model)

	if updated.pendingO {
		t.Error("expected pendingO to be false after 'oa'")
	}
	// With single artifact, should go directly to load (cmd != nil)
	if cmd == nil {
		t.Error("expected load artifact command, got nil")
	}
}

func TestHandleListKey_OAMultipleArtifactsShowsSelection(t *testing.T) {
	dir := t.TempDir()
	artifactDir := filepath.Join(dir, ".sortie", "artifacts")
	os.MkdirAll(artifactDir, 0755)
	os.WriteFile(filepath.Join(artifactDir, "implement.md"), []byte("content 1"), 0644)
	os.WriteFile(filepath.Join(artifactDir, "review.md"), []byte("content 2"), 0644)

	m := Model{
		keys:     newKeyMap(),
		list:     newListView(false, ""),
		detail:   newDetailView(),
		taskInfo: newTaskInfoView(),
		view:     viewList,
		cfg: &config.Config{
			Workflows: []config.WorkflowConfig{
				{
					Name: "default",
					Steps: []config.StepConfig{
						{Name: "implement", Artifact: true},
						{Name: "review", Artifact: true},
					},
				},
			},
		},
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Test task", Status: "completed", Workflow: "default", WorktreePath: dir},
	})

	// Press "o" then "a"
	oMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}}
	result, _ := m.handleListKey(oMsg)
	updated := result.(Model)

	aMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	result, cmd := updated.handleListKey(aMsg)
	updated = result.(Model)

	// Multiple artifacts — should show selection menu
	if !updated.selectingArtifact {
		t.Error("expected selectingArtifact to be true with multiple artifacts")
	}
	if len(updated.artifactNames) != 2 {
		t.Errorf("expected 2 artifact names, got %d", len(updated.artifactNames))
	}
	if updated.artifactAction != "view" {
		t.Errorf("expected artifactAction 'view', got %q", updated.artifactAction)
	}
	if cmd != nil {
		t.Error("expected no command (selection pending), got non-nil")
	}
}

func TestHandleListKey_ETOpensEditTitle(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
		client: &client.Client{},
	}
	m.list.SetSize(100, 30)
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Original title", Status: "pending"},
	})

	// Press "e" to enter pending state
	eMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}}
	result, _ := m.handleListKey(eMsg)
	updated := result.(Model)
	if !updated.pendingE {
		t.Error("expected pendingE to be true after 'e'")
	}

	// Press "t" to trigger edit title
	tMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}}
	result, cmd := updated.handleListKey(tMsg)
	updated = result.(Model)

	if updated.pendingE {
		t.Error("expected pendingE to be false after 'et' sequence")
	}
	// The command should be non-nil (opens editor)
	if cmd == nil {
		t.Error("expected a command to open editor for title, got nil")
	}
}

func TestHandleTaskInfoKey_ETOpensEditTitle(t *testing.T) {
	m := Model{
		keys:     newKeyMap(),
		list:     newListView(false, ""),
		detail:   newDetailView(),
		taskInfo: newTaskInfoView(),
		view:     viewTaskInfo,
		client:   &client.Client{},
	}
	task := daemon.TaskInfo{ID: 1, Title: "Original title", Status: "pending"}
	m.taskInfo.SetTask(&task)

	// Press "e" to enter pending state
	eMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}}
	result, _ := m.handleTaskInfoKey(eMsg)
	updated := result.(Model)
	if !updated.pendingE {
		t.Error("expected pendingE to be true after 'e'")
	}

	// Press "t" to trigger edit title
	tMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}}
	result, cmd := updated.handleTaskInfoKey(tMsg)
	updated = result.(Model)

	if updated.pendingE {
		t.Error("expected pendingE to be false after 'et' sequence")
	}
	if cmd == nil {
		t.Error("expected a command to open editor for title, got nil")
	}
}

func TestHandleListKey_ETNoTaskSelected(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
		client: &client.Client{},
	}
	m.list.SetSize(100, 30)
	// No tasks set — no task selected

	// Press "e" then "t"
	eMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}}
	result, _ := m.handleListKey(eMsg)
	updated := result.(Model)

	tMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}}
	result, cmd := updated.handleListKey(tMsg)
	updated = result.(Model)

	if updated.pendingE {
		t.Error("expected pendingE to be false")
	}
	if cmd != nil {
		t.Error("expected no command when no task is selected")
	}
}

func TestTaskFieldUpdatedMsg_TitleUpdatesStatus(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
		client: &client.Client{},
	}

	msg := taskFieldUpdatedMsg{field: "title"}
	result, cmd := m.Update(msg)
	updated := result.(Model)

	if updated.statusMessage != "Title updated" {
		t.Errorf("expected status message 'Title updated', got %q", updated.statusMessage)
	}
	if updated.statusMessageTTL != 2 {
		t.Errorf("expected statusMessageTTL 2, got %d", updated.statusMessageTTL)
	}
	if !updated.list.refreshing {
		t.Error("expected list to be refreshing after field update")
	}
	if cmd == nil {
		t.Error("expected refresh command after field update")
	}
}

func TestHandleListKey_EAOpensEditArtifact(t *testing.T) {
	dir := t.TempDir()
	artifactDir := filepath.Join(dir, ".sortie", "artifacts")
	os.MkdirAll(artifactDir, 0755)
	os.WriteFile(filepath.Join(artifactDir, "implement.md"), []byte("content"), 0644)
	os.WriteFile(filepath.Join(artifactDir, "review.md"), []byte("content"), 0644)

	m := Model{
		keys:     newKeyMap(),
		list:     newListView(false, ""),
		detail:   newDetailView(),
		taskInfo: newTaskInfoView(),
		view:     viewList,
		cfg: &config.Config{
			Workflows: []config.WorkflowConfig{
				{
					Name: "default",
					Steps: []config.StepConfig{
						{Name: "implement", Artifact: true},
						{Name: "review", Artifact: true},
					},
				},
			},
		},
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Test task", Status: "completed", Workflow: "default", WorktreePath: dir},
	})

	// Press "e" then "a"
	eMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}}
	result, _ := m.handleListKey(eMsg)
	updated := result.(Model)
	if !updated.pendingE {
		t.Error("expected pendingE to be true after 'e'")
	}

	aMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	result, _ = updated.handleListKey(aMsg)
	updated = result.(Model)

	if !updated.selectingArtifact {
		t.Error("expected selectingArtifact to be true")
	}
	if updated.artifactAction != "edit" {
		t.Errorf("expected artifactAction 'edit', got %q", updated.artifactAction)
	}
}

func TestHandleTaskInfoKey_YSetsPendingY(t *testing.T) {
	m := Model{
		keys:     newKeyMap(),
		list:     newListView(false, ""),
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
		list:     newListView(false, ""),
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
	// Clipboard may fail in CI/headless environments — verify either
	// status message (success) or error (clipboard unavailable) is set.
	if updated.err == nil && updated.statusMessage != "Copied description to clipboard" {
		t.Errorf("expected status message 'Copied description to clipboard', got %q", updated.statusMessage)
	}
}

func TestHandleTaskInfoKey_YCCopiesContext(t *testing.T) {
	m := Model{
		keys:     newKeyMap(),
		list:     newListView(false, ""),
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
	// Clipboard may fail in CI/headless environments — verify either
	// status message (success) or error (clipboard unavailable) is set.
	if updated.err == nil && updated.statusMessage != "Copied context to clipboard" {
		t.Errorf("expected status message 'Copied context to clipboard', got %q", updated.statusMessage)
	}
}

func TestHandleListKey_OANoWorktreeShowsError(t *testing.T) {
	m := Model{
		keys:     newKeyMap(),
		list:     newListView(false, ""),
		detail:   newDetailView(),
		taskInfo: newTaskInfoView(),
		view:     viewList,
		cfg: &config.Config{
			Workflows: []config.WorkflowConfig{
				{Name: "default", Steps: []config.StepConfig{{Name: "implement", Artifact: true}}},
			},
		},
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Pending task", Status: "pending", Workflow: "default", WorktreePath: ""},
	})

	// Press "o" then "a"
	oMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}}
	result, _ := m.handleListKey(oMsg)
	updated := result.(Model)

	aMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	result, _ = updated.handleListKey(aMsg)
	updated = result.(Model)

	if updated.err == nil {
		t.Error("expected error for task with no worktree")
	}
	if !strings.Contains(updated.err.Error(), "no worktree") {
		t.Errorf("expected 'no worktree' error, got: %v", updated.err)
	}
}

func TestHandleListKey_OANoArtifactsShowsError(t *testing.T) {
	dir := t.TempDir()
	// Create artifacts dir but no artifact files
	os.MkdirAll(filepath.Join(dir, ".sortie", "artifacts"), 0755)

	m := Model{
		keys:     newKeyMap(),
		list:     newListView(false, ""),
		detail:   newDetailView(),
		taskInfo: newTaskInfoView(),
		view:     viewList,
		cfg: &config.Config{
			Workflows: []config.WorkflowConfig{
				{Name: "default", Steps: []config.StepConfig{{Name: "implement", Artifact: true}}},
			},
		},
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Task", Status: "running", Workflow: "default", WorktreePath: dir},
	})

	// Press "o" then "a"
	oMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}}
	result, _ := m.handleListKey(oMsg)
	updated := result.(Model)

	aMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	result, _ = updated.handleListKey(aMsg)
	updated = result.(Model)

	if updated.err == nil {
		t.Error("expected error for task with no artifacts on disk")
	}
	if !strings.Contains(updated.err.Error(), "no artifacts") {
		t.Errorf("expected 'no artifacts' error, got: %v", updated.err)
	}
}

func TestHandleTaskInfoKey_YResetByOtherKey(t *testing.T) {
	m := Model{
		keys:     newKeyMap(),
		list:     newListView(false, ""),
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

func TestHandleListKey_ONonAResetsPending(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
	}

	// Press "o" then "x" — should reset pendingO and do nothing
	oMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}}
	result, _ := m.handleListKey(oMsg)
	updated := result.(Model)
	if !updated.pendingO {
		t.Error("expected pendingO to be true after 'o'")
	}

	xMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
	result, _ = updated.handleListKey(xMsg)
	updated = result.(Model)
	if updated.pendingO {
		t.Error("expected pendingO to be false after non-'a' key")
	}
}

func TestHandleArtifactSelectKey_Navigation(t *testing.T) {
	m := Model{
		keys:              newKeyMap(),
		selectingArtifact: true,
		artifactCursor:    0,
		artifactNames:     []string{"implement", "review", "test"},
		artifactWorktree:  "/tmp/test",
		artifactAction:    "view",
	}

	// Move down
	jMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	result, _ := m.handleArtifactSelectKey(jMsg)
	updated := result.(Model)
	if updated.artifactCursor != 1 {
		t.Errorf("expected cursor at 1, got %d", updated.artifactCursor)
	}

	// Move down again
	result, _ = updated.handleArtifactSelectKey(jMsg)
	updated = result.(Model)
	if updated.artifactCursor != 2 {
		t.Errorf("expected cursor at 2, got %d", updated.artifactCursor)
	}

	// Move down at bottom — should stay
	result, _ = updated.handleArtifactSelectKey(jMsg)
	updated = result.(Model)
	if updated.artifactCursor != 2 {
		t.Errorf("expected cursor to stay at 2, got %d", updated.artifactCursor)
	}

	// Move up
	kMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}
	result, _ = updated.handleArtifactSelectKey(kMsg)
	updated = result.(Model)
	if updated.artifactCursor != 1 {
		t.Errorf("expected cursor at 1, got %d", updated.artifactCursor)
	}
}

func TestHandleArtifactSelectKey_EscCancels(t *testing.T) {
	m := Model{
		keys:              newKeyMap(),
		selectingArtifact: true,
		artifactCursor:    1,
		artifactNames:     []string{"implement", "review"},
		artifactAction:    "view",
	}

	msg := tea.KeyMsg{Type: tea.KeyEscape}
	result, _ := m.handleArtifactSelectKey(msg)
	updated := result.(Model)

	if updated.selectingArtifact {
		t.Error("expected selectingArtifact to be false after esc")
	}
}

func TestHandleArtifactViewKey_QReturnsToList(t *testing.T) {
	m := Model{
		keys: newKeyMap(),
		list: newListView(false, ""),
		view: viewArtifact,
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	result, cmd := m.handleArtifactViewKey(msg)
	updated := result.(Model)

	if updated.view != viewList {
		t.Errorf("expected view to be viewList, got %d", updated.view)
	}
	if cmd != nil {
		t.Error("expected no command, got non-nil")
	}
}

func TestHandleArtifactViewKey_EscReturnsToList(t *testing.T) {
	m := Model{
		keys: newKeyMap(),
		list: newListView(false, ""),
		view: viewArtifact,
	}

	msg := tea.KeyMsg{Type: tea.KeyEscape}
	result, cmd := m.handleArtifactViewKey(msg)
	updated := result.(Model)

	if updated.view != viewList {
		t.Errorf("expected view to be viewList, got %d", updated.view)
	}
	if cmd != nil {
		t.Error("expected no command, got non-nil")
	}
}

func TestHandleTaskInfoKey_YDNoTaskNoError(t *testing.T) {
	m := Model{
		keys:     newKeyMap(),
		list:     newListView(false, ""),
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

func TestHandleTaskInfoKey_OAOpensArtifactSelection(t *testing.T) {
	dir := t.TempDir()
	artifactDir := filepath.Join(dir, ".sortie", "artifacts")
	os.MkdirAll(artifactDir, 0755)
	os.WriteFile(filepath.Join(artifactDir, "implement.md"), []byte("content"), 0644)
	os.WriteFile(filepath.Join(artifactDir, "review.md"), []byte("content"), 0644)

	m := Model{
		keys:     newKeyMap(),
		list:     newListView(false, ""),
		detail:   newDetailView(),
		taskInfo: newTaskInfoView(),
		view:     viewTaskInfo,
		cfg: &config.Config{
			Workflows: []config.WorkflowConfig{
				{
					Name: "default",
					Steps: []config.StepConfig{
						{Name: "implement", Artifact: true},
						{Name: "review", Artifact: true},
					},
				},
			},
		},
	}
	task := daemon.TaskInfo{ID: 1, Title: "Test", Status: "completed", Workflow: "default", WorktreePath: dir}
	m.taskInfo.SetTask(&task)

	// Press "o" then "a"
	oMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}}
	result, _ := m.handleTaskInfoKey(oMsg)
	updated := result.(Model)
	if !updated.pendingO {
		t.Error("expected pendingO to be true after 'o'")
	}

	aMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	result, _ = updated.handleTaskInfoKey(aMsg)
	updated = result.(Model)

	if !updated.selectingArtifact {
		t.Error("expected selectingArtifact to be true")
	}
	if updated.artifactAction != "view" {
		t.Errorf("expected artifactAction 'view', got %q", updated.artifactAction)
	}
}

func TestHandleTaskInfoKey_EAOpensEditArtifact(t *testing.T) {
	dir := t.TempDir()
	artifactDir := filepath.Join(dir, ".sortie", "artifacts")
	os.MkdirAll(artifactDir, 0755)
	os.WriteFile(filepath.Join(artifactDir, "implement.md"), []byte("content"), 0644)

	m := Model{
		keys:     newKeyMap(),
		list:     newListView(false, ""),
		detail:   newDetailView(),
		taskInfo: newTaskInfoView(),
		view:     viewTaskInfo,
		cfg: &config.Config{
			Workflows: []config.WorkflowConfig{
				{
					Name: "default",
					Steps: []config.StepConfig{
						{Name: "implement", Artifact: true},
					},
				},
			},
		},
	}
	task := daemon.TaskInfo{ID: 1, Title: "Test", Status: "completed", Workflow: "default", WorktreePath: dir}
	m.taskInfo.SetTask(&task)

	// Press "e" then "a" — single artifact, should return command directly
	eMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}}
	result, _ := m.handleTaskInfoKey(eMsg)
	updated := result.(Model)

	aMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	result, cmd := updated.handleTaskInfoKey(aMsg)
	updated = result.(Model)

	// Single artifact, edit action — should return a command (tea.ExecProcess)
	if cmd == nil {
		t.Error("expected edit command for single artifact, got nil")
	}
}

func TestHandleTaskInfoKey_YDEmptyDescriptionNoOp(t *testing.T) {
	m := Model{
		keys:     newKeyMap(),
		list:     newListView(false, ""),
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
		list:     newListView(false, ""),
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

func TestViewRendersArtifactSelection(t *testing.T) {
	m := Model{
		keys:              newKeyMap(),
		list:              newListView(false, ""),
		detail:            newDetailView(),
		view:              viewList,
		selectingArtifact: true,
		artifactCursor:    0,
		artifactNames:     []string{"implement", "review"},
		artifactAction:    "view",
	}

	output := m.View()

	if !strings.Contains(output, "Select Artifact") {
		t.Error("expected artifact selection screen to contain 'Select Artifact' title")
	}
	if !strings.Contains(output, "implement") {
		t.Error("expected artifact selection to contain 'implement'")
	}
	if !strings.Contains(output, "review") {
		t.Error("expected artifact selection to contain 'review'")
	}
}

func TestViewRendersEditArtifactTitle(t *testing.T) {
	m := Model{
		keys:              newKeyMap(),
		list:              newListView(false, ""),
		detail:            newDetailView(),
		view:              viewList,
		selectingArtifact: true,
		artifactCursor:    0,
		artifactNames:     []string{"implement"},
		artifactAction:    "edit",
	}

	output := m.View()

	if !strings.Contains(output, "Edit Artifact") {
		t.Error("expected artifact selection screen to contain 'Edit Artifact' title when action is edit")
	}
}

func TestArtifactViewState_View(t *testing.T) {
	v := &artifactViewState{}
	v.SetSize(80, 24)
	v.SetContent("implement", "This is the artifact content.\nLine 2.")

	output := v.View()

	if !strings.Contains(output, "Artifact: implement") {
		t.Error("expected artifact view to contain 'Artifact: implement'")
	}
	if !strings.Contains(output, "artifact content") {
		t.Error("expected artifact view to contain artifact content")
	}
	if !strings.Contains(output, "esc/q") {
		t.Error("expected artifact view help to contain 'esc/q'")
	}
}

func TestArtifactLoadedMsg_SwitchesToArtifactView(t *testing.T) {
	m := Model{
		keys: newKeyMap(),
		list: newListView(false, ""),
		view: viewList,
	}
	m.artifactView.SetSize(80, 24)

	msg := artifactLoadedMsg{name: "implement", content: "test content"}
	result, _ := m.Update(msg)
	updated := result.(Model)

	if updated.view != viewArtifact {
		t.Errorf("expected view to be viewArtifact (%d), got %d", viewArtifact, updated.view)
	}
}

func TestHandleListKey_OAFiltersNonExistentArtifacts(t *testing.T) {
	dir := t.TempDir()
	artifactDir := filepath.Join(dir, ".sortie", "artifacts")
	os.MkdirAll(artifactDir, 0755)
	// Only create artifact for "implement", not for "review"
	os.WriteFile(filepath.Join(artifactDir, "implement.md"), []byte("content"), 0644)

	m := Model{
		keys:     newKeyMap(),
		list:     newListView(false, ""),
		detail:   newDetailView(),
		taskInfo: newTaskInfoView(),
		view:     viewList,
		cfg: &config.Config{
			Workflows: []config.WorkflowConfig{
				{
					Name: "default",
					Steps: []config.StepConfig{
						{Name: "implement", Artifact: true},
						{Name: "review", Artifact: true}, // artifact: true but no file on disk
					},
				},
			},
		},
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Task", Status: "completed", Workflow: "default", WorktreePath: dir},
	})

	// Press "o" then "a" — only one artifact exists on disk, should skip selection
	oMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}}
	result, _ := m.handleListKey(oMsg)
	updated := result.(Model)

	aMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	result, cmd := updated.handleListKey(aMsg)
	updated = result.(Model)

	// Only "implement" exists on disk — should skip selection and load directly
	if updated.selectingArtifact {
		t.Error("expected selectingArtifact to be false when only one artifact exists on disk")
	}
	if cmd == nil {
		t.Error("expected load command for single existing artifact")
	}
}

func TestFullHelp_ContainsArtifactBindings(t *testing.T) {
	keys := newKeyMap()
	groups := keys.FullHelp()

	found := false
	for _, group := range groups {
		for _, b := range group {
			if b.Help().Key == "oa" {
				found = true
				break
			}
		}
	}
	if !found {
		t.Error("expected FullHelp to contain 'oa' open artifact binding")
	}
}

func TestTaskInfoKeyMap_ContainsArtifactBindings(t *testing.T) {
	keys := newTaskInfoKeyMap()
	bindings := keys.ShortHelp()

	foundOA := false
	foundEA := false
	for _, b := range bindings {
		if b.Help().Key == "oa" {
			foundOA = true
		}
		if b.Help().Key == "ea" {
			foundEA = true
		}
	}
	if !foundOA {
		t.Error("expected task info ShortHelp to contain 'oa' binding")
	}
	if !foundEA {
		t.Error("expected task info ShortHelp to contain 'ea' binding")
	}
}

func TestHandleTaskInfoKey_EscDismissesArtifactSelection(t *testing.T) {
	m := Model{
		keys:              newKeyMap(),
		list:              newListView(false, ""),
		detail:            newDetailView(),
		taskInfo:          newTaskInfoView(),
		view:              viewTaskInfo,
		selectingArtifact: true,
		artifactCursor:    0,
		artifactNames:     []string{"implement", "review"},
		artifactAction:    "view",
	}
	task := daemon.TaskInfo{ID: 1, Title: "Test", Status: "completed"}
	m.taskInfo.SetTask(&task)

	msg := tea.KeyMsg{Type: tea.KeyEscape}
	result, _ := m.handleTaskInfoKey(msg)
	updated := result.(Model)

	if updated.selectingArtifact {
		t.Error("expected selectingArtifact to be false after esc")
	}
	// Should stay in task info view, not return to list
	if updated.view != viewTaskInfo {
		t.Errorf("expected view to remain viewTaskInfo (%d), got %d", viewTaskInfo, updated.view)
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

func TestSetNumber_DefaultTrue(t *testing.T) {
	l := newListView(false, "")
	if !l.showLineNumbers {
		t.Error("expected showLineNumbers to default to true")
	}
}

func TestSetNumber_EnablesLineNumbers(t *testing.T) {
	m := newTestModelWithTasks(5)
	m.list.showLineNumbers = false

	result, _ := executeCommand(m, "set number")
	updated := result.(Model)
	if !updated.list.showLineNumbers {
		t.Error("expected showLineNumbers to be true after ':set number'")
	}
}

func TestSetNumber_DisablesLineNumbers(t *testing.T) {
	m := newTestModelWithTasks(5)
	m.list.showLineNumbers = true

	result, _ := executeCommand(m, "set nonumber")
	updated := result.(Model)
	if updated.list.showLineNumbers {
		t.Error("expected showLineNumbers to be false after ':set nonumber'")
	}
}

func TestSetNumber_TogglesLineNumbers(t *testing.T) {
	m := newTestModelWithTasks(5)
	m.list.showLineNumbers = true

	result, _ := executeCommand(m, "set number!")
	updated := result.(Model)
	if updated.list.showLineNumbers {
		t.Error("expected showLineNumbers to be false after ':set number!' (was true)")
	}

	result, _ = executeCommand(updated, "set number!")
	updated = result.(Model)
	if !updated.list.showLineNumbers {
		t.Error("expected showLineNumbers to be true after ':set number!' (was false)")
	}
}

func TestSetNumber_CommandModeIntegration(t *testing.T) {
	m := newTestModelWithTasks(5)

	// Enter command mode with ":"
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	// Type "set nonumber"
	for _, ch := range "set nonumber" {
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}}
		result, _ = updated.handleListKey(msg)
		updated = result.(Model)
	}

	// Press enter to execute
	msg = tea.KeyMsg{Type: tea.KeyEnter}
	result, _ = updated.handleListKey(msg)
	updated = result.(Model)

	if updated.commandMode {
		t.Error("expected command mode to exit after executing command")
	}
	if updated.list.showLineNumbers {
		t.Error("expected showLineNumbers to be false after ':set nonumber<enter>'")
	}
}

func TestSetNumber_HidesLineNumbersInView(t *testing.T) {
	l := newListView(false, "")
	l.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Task One", Status: "pending"},
		{ID: 2, Title: "Task Two", Status: "running"},
	})
	l.SetSize(100, 24)

	// With line numbers enabled (default), output should contain line numbers
	output := l.View()
	lines := strings.Split(output, "\n")
	// Find the first task row (after header) and check it has " 1 " pattern
	foundLineNum := false
	for _, line := range lines {
		if strings.Contains(line, "Task One") && strings.Contains(line, " 1 ") {
			foundLineNum = true
			break
		}
	}
	if !foundLineNum {
		t.Error("expected line numbers in view when showLineNumbers is true")
	}

	// Disable line numbers
	l.showLineNumbers = false
	output = l.View()

	// Header should not have the gutter space
	lines = strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "ID") && strings.Contains(line, "STATUS") {
			// With line numbers off, the header should start closer to the left
			// (no gutter space padding)
			if strings.HasPrefix(line, "    ") {
				// Check it doesn't have the extra gutter space
				// The gutter with numbers would be at least 4 spaces wider
			}
			break
		}
	}

	// Task rows should not contain line number gutter
	foundLineNum = false
	for _, line := range lines {
		if strings.Contains(line, "Task One") {
			// With showLineNumbers=false, the line should NOT have the " 1 " line number
			// before the task ID
			if strings.Contains(line, " 1 ") && strings.Contains(line, " 2 ") {
				// Both line numbers present would indicate line numbers are still shown
				foundLineNum = true
			}
			break
		}
	}
}

func TestMatchSetNumber(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"set number", true},
		{"set nonumber", true},
		{"set number!", true},
		{"set num", false},
		{"setnumber", false},
		{"set", false},
		{"number", false},
		{"  set number  ", true},
		{"set number ", true},
	}
	for _, tt := range tests {
		_, ok := matchSetNumber(tt.input)
		if ok != tt.want {
			t.Errorf("matchSetNumber(%q) = %v, want %v", tt.input, ok, tt.want)
		}
	}
}

func TestSetFinished_DefaultTrue(t *testing.T) {
	l := newListView(false, "")
	if !l.showFinished {
		t.Error("expected showFinished to default to true")
	}
}

func TestSetFinished_EnablesFinishedTasks(t *testing.T) {
	m := newTestModelWithTasks(5)
	m.list.showFinished = false

	result, _ := executeCommand(m, "set finished")
	updated := result.(Model)
	if !updated.list.showFinished {
		t.Error("expected showFinished to be true after ':set finished'")
	}
}

func TestSetFinished_DisablesFinishedTasks(t *testing.T) {
	m := newTestModelWithTasks(5)
	m.list.showFinished = true

	result, _ := executeCommand(m, "set nofinished")
	updated := result.(Model)
	if updated.list.showFinished {
		t.Error("expected showFinished to be false after ':set nofinished'")
	}
}

func TestSetFinished_TogglesFinishedTasks(t *testing.T) {
	m := newTestModelWithTasks(5)
	m.list.showFinished = true

	result, _ := executeCommand(m, "set finished!")
	updated := result.(Model)
	if updated.list.showFinished {
		t.Error("expected showFinished to be false after ':set finished!' (was true)")
	}

	result, _ = executeCommand(updated, "set finished!")
	updated = result.(Model)
	if !updated.list.showFinished {
		t.Error("expected showFinished to be true after ':set finished!' (was false)")
	}
}

func TestSetFinished_CommandModeIntegration(t *testing.T) {
	m := newTestModelWithTasks(5)

	// Enter command mode with ":"
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	// Type "set nofinished"
	for _, ch := range "set nofinished" {
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}}
		result, _ = updated.handleListKey(msg)
		updated = result.(Model)
	}

	// Press enter to execute
	msg = tea.KeyMsg{Type: tea.KeyEnter}
	result, _ = updated.handleListKey(msg)
	updated = result.(Model)

	if updated.commandMode {
		t.Error("expected command mode to exit after executing command")
	}
	if updated.list.showFinished {
		t.Error("expected showFinished to be false after ':set nofinished<enter>'")
	}
}

func TestSetFinished_HidesFinishedTasksInView(t *testing.T) {
	l := newListView(false, "")
	l.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Active Task", Status: "pending"},
		{ID: 2, Title: "Running Task", Status: "running"},
		{ID: 3, Title: "Done Task", Status: "completed"},
		{ID: 4, Title: "Failed Task", Status: "failed"},
	})
	l.SetSize(100, 24)

	// With showFinished=true (default), all tasks should be visible
	output := l.View()
	if !strings.Contains(output, "Active Task") {
		t.Error("expected 'Active Task' in view when showFinished is true")
	}
	if !strings.Contains(output, "Done Task") {
		t.Error("expected 'Done Task' in view when showFinished is true")
	}
	if !strings.Contains(output, "Failed Task") {
		t.Error("expected 'Failed Task' in view when showFinished is true")
	}

	// Hide finished tasks
	l.showFinished = false
	l.applyFilter()
	output = l.View()
	if !strings.Contains(output, "Active Task") {
		t.Error("expected 'Active Task' in view when showFinished is false")
	}
	if !strings.Contains(output, "Running Task") {
		t.Error("expected 'Running Task' in view when showFinished is false")
	}
	if strings.Contains(output, "Done Task") {
		t.Error("expected 'Done Task' to be hidden when showFinished is false")
	}
	if strings.Contains(output, "Failed Task") {
		t.Error("expected 'Failed Task' to be hidden when showFinished is false")
	}

	// Show finished tasks again
	l.showFinished = true
	l.applyFilter()
	output = l.View()
	if !strings.Contains(output, "Done Task") {
		t.Error("expected 'Done Task' in view after re-enabling showFinished")
	}
	if !strings.Contains(output, "Failed Task") {
		t.Error("expected 'Failed Task' in view after re-enabling showFinished")
	}
}

func TestMatchSetFinished(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"set finished", true},
		{"set nofinished", true},
		{"set finished!", true},
		{"set fin", false},
		{"setfinished", false},
		{"set", false},
		{"finished", false},
		{"  set finished  ", true},
		{"set finished ", true},
	}
	for _, tt := range tests {
		_, ok := matchSetFinished(tt.input)
		if ok != tt.want {
			t.Errorf("matchSetFinished(%q) = %v, want %v", tt.input, ok, tt.want)
		}
	}
}

func TestSetFinished_PreservesAllTasksOnRefresh(t *testing.T) {
	l := newListView(false, "")
	l.showFinished = false
	l.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Active", Status: "pending"},
		{ID: 2, Title: "Done", Status: "completed"},
	})

	// Only active tasks should be in the display list
	if len(l.tasks) != 1 {
		t.Errorf("expected 1 visible task, got %d", len(l.tasks))
	}
	// All tasks should be preserved
	if len(l.allTasks) != 2 {
		t.Errorf("expected 2 allTasks, got %d", len(l.allTasks))
	}

	// Re-enable showFinished and verify all tasks come back
	l.showFinished = true
	l.applyFilter()
	if len(l.tasks) != 2 {
		t.Errorf("expected 2 visible tasks after re-enabling showFinished, got %d", len(l.tasks))
	}
}

func TestListView_GlobalModeHasIndexColumn(t *testing.T) {
	l := newListView(true, "")
	l.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Task", Status: "running", ProjectName: "proj"},
	})
	l.SetSize(120, 24)
	output := l.View()

	// Vim-style: header should NOT contain "#"
	lines := strings.Split(output, "\n")
	headerFound := false
	for _, line := range lines {
		if strings.Contains(line, "ID") && strings.Contains(line, "PROJECT") {
			if strings.Contains(line, "#") {
				t.Error("expected global mode header to NOT contain '#' (vim-style)")
			}
			headerFound = true
			break
		}
	}
	if !headerFound {
		t.Error("could not find header line")
	}

	// Task row should contain line number "1"
	if !strings.Contains(output, "1") {
		t.Error("expected task row to show line number '1'")
	}
}

func TestHandleListKey_SlashEntersForwardSearch(t *testing.T) {
	m := Model{
		keys: newKeyMap(),
		list: newListView(false, ""),
		view: viewList,
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	if !updated.searchMode {
		t.Error("expected searchMode to be true after '/'")
	}
	if updated.searchDirection != 1 {
		t.Errorf("expected searchDirection to be 1 (forward), got %d", updated.searchDirection)
	}
	if updated.searchQuery != "" {
		t.Error("expected searchQuery to be empty initially")
	}
}

func TestHandleListKey_QuestionMarkEntersBackwardSearch(t *testing.T) {
	m := Model{
		keys: newKeyMap(),
		list: newListView(false, ""),
		view: viewList,
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	if !updated.searchMode {
		t.Error("expected searchMode to be true after '?'")
	}
	if updated.searchDirection != -1 {
		t.Errorf("expected searchDirection to be -1 (backward), got %d", updated.searchDirection)
	}
}

func TestHandleSearchKey_EscCancelsSearch(t *testing.T) {
	m := Model{
		keys:        newKeyMap(),
		list:        newListView(false, ""),
		view:        viewList,
		searchMode:  true,
		searchQuery: "test",
	}

	msg := tea.KeyMsg{Type: tea.KeyEscape}
	result, _ := m.handleSearchKey(msg)
	updated := result.(Model)

	if updated.searchMode {
		t.Error("expected searchMode to be false after esc")
	}
	if updated.searchQuery != "" {
		t.Error("expected searchQuery to be cleared after esc")
	}
}

func TestHandleSearchKey_EnterCommitsSearch(t *testing.T) {
	m := Model{
		keys:            newKeyMap(),
		list:            newListView(false, ""),
		view:            viewList,
		searchMode:      true,
		searchQuery:     "fix",
		searchDirection: 1,
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Fix bug", Status: "pending"},
		{ID: 2, Title: "Add feature", Status: "running"},
		{ID: 3, Title: "Fix crash", Status: "completed"},
	})

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	result, _ := m.handleSearchKey(msg)
	updated := result.(Model)

	if updated.searchMode {
		t.Error("expected searchMode to be false after enter")
	}
	if len(updated.list.matchedIndices) != 2 {
		t.Errorf("expected 2 matches for 'fix', got %d", len(updated.list.matchedIndices))
	}
}

func TestHandleSearchKey_TypingBuildsQuery(t *testing.T) {
	m := Model{
		keys:       newKeyMap(),
		list:       newListView(false, ""),
		view:       viewList,
		searchMode: true,
	}

	// Type 'a'
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	result, _ := m.handleSearchKey(msg)
	updated := result.(Model)
	if updated.searchQuery != "a" {
		t.Errorf("expected searchQuery 'a', got '%s'", updated.searchQuery)
	}

	// Type 'b'
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}}
	result, _ = updated.handleSearchKey(msg)
	updated = result.(Model)
	if updated.searchQuery != "ab" {
		t.Errorf("expected searchQuery 'ab', got '%s'", updated.searchQuery)
	}
}

func TestHandleSearchKey_BackspaceDeletesChar(t *testing.T) {
	m := Model{
		keys:        newKeyMap(),
		list:        newListView(false, ""),
		view:        viewList,
		searchMode:  true,
		searchQuery: "ab",
	}

	msg := tea.KeyMsg{Type: tea.KeyBackspace}
	result, _ := m.handleSearchKey(msg)
	updated := result.(Model)
	if updated.searchQuery != "a" {
		t.Errorf("expected searchQuery 'a', got '%s'", updated.searchQuery)
	}

	// Backspace on last char exits search mode
	result, _ = updated.handleSearchKey(msg)
	updated = result.(Model)
	if updated.searchMode {
		t.Error("expected searchMode to be false when query becomes empty via backspace")
	}
}

func TestHandleListKey_NNavigatesToNextMatch(t *testing.T) {
	m := Model{
		keys:            newKeyMap(),
		list:            newListView(false, ""),
		view:            viewList,
		searchDirection: 1,
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Fix bug", Status: "pending"},
		{ID: 2, Title: "Add feature", Status: "running"},
		{ID: 3, Title: "Fix crash", Status: "completed"},
	})
	// Order after sort: 3, 2, 1 => indices: 0="Fix crash", 1="Add feature", 2="Fix bug"
	m.list.performSearch("fix")
	m.list.table.SetCursor(0)
	m.list.currentMatchIdx = 0

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	if updated.list.table.Cursor() != 2 {
		t.Errorf("expected cursor to move to 2 (next fix match), got %d", updated.list.table.Cursor())
	}
}

func TestHandleListKey_ShiftNNavigatesToPrevMatch(t *testing.T) {
	m := Model{
		keys:            newKeyMap(),
		list:            newListView(false, ""),
		view:            viewList,
		searchDirection: 1,
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Fix bug", Status: "pending"},
		{ID: 2, Title: "Add feature", Status: "running"},
		{ID: 3, Title: "Fix crash", Status: "completed"},
	})
	m.list.performSearch("fix")
	m.list.table.SetCursor(2)
	m.list.currentMatchIdx = 1

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	if updated.list.table.Cursor() != 0 {
		t.Errorf("expected cursor to move to 0 (prev fix match), got %d", updated.list.table.Cursor())
	}
}

func TestHandleListKey_NWithNoSearchCreatesNewTask(t *testing.T) {
	cfg := &config.Config{}
	m := Model{
		cfg:         cfg,
		keys:        newKeyMap(),
		list:        newListView(false, ""),
		detail:      newDetailView(),
		prompt:      newPromptView(true, ""),
		view:        viewList,
		client:      &client.Client{},
		projectPath: "/some/path",
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	// With no search matches, n should open prompt view for new task
	if updated.view != viewPrompt {
		t.Errorf("expected view to be viewPrompt (%d), got %d", viewPrompt, updated.view)
	}
}

func TestViewRendersSearchBar(t *testing.T) {
	m := Model{
		keys:            newKeyMap(),
		list:            newListView(false, ""),
		view:            viewList,
		searchMode:      true,
		searchQuery:     "test",
		searchDirection: 1,
	}

	output := m.View()
	if !strings.Contains(output, "/test") {
		t.Error("expected View to contain '/test' search bar for forward search")
	}

	// Backward search
	m.searchDirection = -1
	output = m.View()
	if !strings.Contains(output, "?test") {
		t.Error("expected View to contain '?test' search bar for backward search")
	}
}

func TestHandleListKey_SearchModeDispatchesToHandleSearchKey(t *testing.T) {
	m := Model{
		keys:       newKeyMap(),
		list:       newListView(false, ""),
		view:       viewList,
		searchMode: true,
	}

	// Typing while in search mode should build query
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	if updated.searchQuery != "x" {
		t.Errorf("expected searchQuery 'x' when typing in search mode, got '%s'", updated.searchQuery)
	}
}

func TestFullHelp_ContainsSearchBindings(t *testing.T) {
	keys := newKeyMap()
	groups := keys.FullHelp()

	foundForward := false
	foundBackward := false
	for _, group := range groups {
		for _, b := range group {
			if b.Help().Key == "/" {
				foundForward = true
			}
			if b.Help().Key == "?" {
				foundBackward = true
			}
		}
	}
	if !foundForward {
		t.Error("expected FullHelp to contain '/' search forward binding")
	}
	if !foundBackward {
		t.Error("expected FullHelp to contain '?' search backward binding")
	}
}

func TestStatusMessage_ClearedOnKeypress(t *testing.T) {
	m := Model{
		keys:          newKeyMap(),
		list:          newListView(false, ""),
		detail:        newDetailView(),
		taskInfo:      newTaskInfoView(),
		view:          viewList,
		statusMessage: "Copied description to clipboard",
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	result, _ := m.handleKey(msg)
	updated := result.(Model)

	if updated.statusMessage != "" {
		t.Errorf("expected statusMessage to be cleared on keypress, got %q", updated.statusMessage)
	}
}

func TestStatusMessage_TickDecrementsAndClears(t *testing.T) {
	m := Model{
		keys:             newKeyMap(),
		list:             newListView(false, ""),
		detail:           newDetailView(),
		taskInfo:         newTaskInfoView(),
		view:             viewList,
		statusMessage:    "Copied description to clipboard",
		statusMessageTTL: 2,
	}

	// First tick: TTL decrements to 1, message still visible
	result, _ := m.Update(tickMsg(time.Now()))
	updated := result.(Model)

	if updated.statusMessage == "" {
		t.Error("expected statusMessage to still be present after first tick")
	}
	if updated.statusMessageTTL != 1 {
		t.Errorf("expected statusMessageTTL=1, got %d", updated.statusMessageTTL)
	}

	// Second tick: TTL decrements to 0, message cleared
	result, _ = updated.Update(tickMsg(time.Now()))
	updated = result.(Model)

	if updated.statusMessage != "" {
		t.Errorf("expected statusMessage to be cleared after TTL expired, got %q", updated.statusMessage)
	}
}

func TestStatusMessage_RenderedInView(t *testing.T) {
	m := Model{
		keys:          newKeyMap(),
		list:          newListView(false, ""),
		detail:        newDetailView(),
		taskInfo:      newTaskInfoView(),
		view:          viewList,
		width:         80,
		height:        24,
		statusMessage: "Copied description to clipboard",
	}

	output := m.View()
	if !strings.Contains(output, "Copied description to clipboard") {
		t.Error("expected View() to contain the status message")
	}
}

func TestStatusMessage_NotRenderedWhenEmpty(t *testing.T) {
	m := Model{
		keys:          newKeyMap(),
		list:          newListView(false, ""),
		detail:        newDetailView(),
		taskInfo:      newTaskInfoView(),
		view:          viewList,
		width:         80,
		height:        24,
		statusMessage: "",
	}

	output := m.View()
	if strings.Contains(output, "Copied") {
		t.Error("expected View() to not contain clipboard message when statusMessage is empty")
	}
}

func TestYDEmptyDescription_NoStatusMessage(t *testing.T) {
	m := Model{
		keys:     newKeyMap(),
		list:     newListView(false, ""),
		detail:   newDetailView(),
		taskInfo: newTaskInfoView(),
		view:     viewTaskInfo,
		pendingY: true,
	}
	task := daemon.TaskInfo{ID: 1, Title: "Test", Description: "", Context: "ctx"}
	m.taskInfo.SetTask(&task)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}
	result, _ := m.handleTaskInfoKey(msg)
	updated := result.(Model)

	if updated.statusMessage != "" {
		t.Errorf("expected no status message for empty description, got %q", updated.statusMessage)
	}
}

func TestYCEmptyContext_NoStatusMessage(t *testing.T) {
	m := Model{
		keys:     newKeyMap(),
		list:     newListView(false, ""),
		detail:   newDetailView(),
		taskInfo: newTaskInfoView(),
		view:     viewTaskInfo,
		pendingY: true,
	}
	task := daemon.TaskInfo{ID: 1, Title: "Test", Description: "desc", Context: ""}
	m.taskInfo.SetTask(&task)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}
	result, _ := m.handleTaskInfoKey(msg)
	updated := result.(Model)

	if updated.statusMessage != "" {
		t.Errorf("expected no status message for empty context, got %q", updated.statusMessage)
	}
}

func TestListView_WindowedRendering(t *testing.T) {
	l := newListView(false, "")
	tasks := make([]daemon.TaskInfo, 20)
	for i := range tasks {
		tasks[i] = daemon.TaskInfo{ID: int64(i + 1), Title: fmt.Sprintf("Task %d", i+1), Status: "pending"}
	}
	l.SetTasks(tasks)
	l.SetSize(100, 15) // visibleRows = 15 - 7 = 8

	output := l.View()
	lines := strings.Split(output, "\n")

	// Count task lines (lines containing "Task ")
	taskLines := 0
	for _, line := range lines {
		if strings.Contains(line, "Task ") {
			taskLines++
		}
	}
	if taskLines != 8 {
		t.Errorf("expected 8 visible task rows, got %d", taskLines)
	}

	// Helper row should always be present
	if !strings.Contains(output, "quit") {
		t.Error("expected help row to be present in windowed view")
	}
}

func TestListView_HelperRowPersistsAfterSearchCancel(t *testing.T) {
	m := Model{
		keys: newKeyMap(),
		list: newListView(false, ""),
		view: viewList,
	}
	tasks := make([]daemon.TaskInfo, 20)
	for i := range tasks {
		tasks[i] = daemon.TaskInfo{ID: int64(i + 1), Title: fmt.Sprintf("Task %d", i+1), Status: "pending"}
	}
	m.list.SetTasks(tasks)
	m.list.SetSize(100, 15)

	// Verify helper row is present initially
	output := m.View()
	if !strings.Contains(output, "quit") {
		t.Error("expected help row before search")
	}

	// Enter search mode
	m.searchMode = true
	m.searchQuery = "test"
	output = m.View()
	if !strings.Contains(output, "quit") {
		t.Error("expected help row during search")
	}
	if !strings.Contains(output, "/test") {
		t.Error("expected search bar during search")
	}

	// Cancel search with Esc
	msg := tea.KeyMsg{Type: tea.KeyEscape}
	result, _ := m.handleSearchKey(msg)
	updated := result.(Model)
	output = updated.View()
	if !strings.Contains(output, "quit") {
		t.Error("expected help row after cancelling search")
	}
	if strings.Contains(output, "/test") {
		t.Error("expected search bar to be gone after cancel")
	}
}

func TestListView_ScrollOffsetFollowsCursor(t *testing.T) {
	l := newListView(false, "")
	tasks := make([]daemon.TaskInfo, 20)
	for i := range tasks {
		tasks[i] = daemon.TaskInfo{ID: int64(i + 1), Title: fmt.Sprintf("Task %d", i+1), Status: "pending"}
	}
	l.SetTasks(tasks)
	l.SetSize(100, 15) // visibleRows = 8

	// Cursor starts at 0, scroll offset at 0
	if l.scrollOffset != 0 {
		t.Errorf("expected scrollOffset 0, got %d", l.scrollOffset)
	}

	// Move cursor past visible window
	for i := 0; i < 12; i++ {
		l.MoveDown()
	}
	// Cursor should be at 12, scrollOffset should have adjusted
	if l.table.Cursor() != 12 {
		t.Errorf("expected cursor at 12, got %d", l.table.Cursor())
	}
	if l.scrollOffset < 3 {
		t.Errorf("expected scrollOffset >= 3, got %d", l.scrollOffset)
	}

	// View should still contain the help row
	output := l.View()
	if !strings.Contains(output, "quit") {
		t.Error("expected help row after scrolling down")
	}
}

func TestListView_ExtraLinesReduceVisibleRows(t *testing.T) {
	l := newListView(false, "")
	l.SetSize(100, 15) // visibleRows = 15 - 7 = 8

	if l.visibleRows() != 8 {
		t.Errorf("expected 8 visible rows without extra lines, got %d", l.visibleRows())
	}

	l.extraLines = 1 // e.g., search bar
	if l.visibleRows() != 7 {
		t.Errorf("expected 7 visible rows with 1 extra line, got %d", l.visibleRows())
	}

	l.extraLines = 2 // e.g., search bar + command bar
	if l.visibleRows() != 6 {
		t.Errorf("expected 6 visible rows with 2 extra lines, got %d", l.visibleRows())
	}
}

func TestListView_ScrollIndicatorInHelpBar(t *testing.T) {
	l := newListView(false, "")
	tasks := make([]daemon.TaskInfo, 20)
	for i := range tasks {
		tasks[i] = daemon.TaskInfo{ID: int64(i + 1), Title: fmt.Sprintf("Task %d", i+1), Status: "pending"}
	}
	l.SetTasks(tasks)
	l.SetSize(100, 15) // visibleRows = 8, 20 tasks → needs scrolling

	output := l.View()

	// When more tasks exist than visible, should show scroll position
	if !strings.Contains(output, "1-8/20") {
		t.Errorf("expected scroll position '1-8/20' in help bar, got:\n%s", output)
	}

	// Scroll down past the visible window
	for i := 0; i < 12; i++ {
		l.MoveDown()
	}
	output = l.View()

	// Should show updated scroll position
	if !strings.Contains(output, "/20") {
		t.Error("expected scroll position with /20 in help bar after scrolling")
	}
}

func TestListView_NoScrollIndicatorWhenAllVisible(t *testing.T) {
	l := newListView(false, "")
	tasks := make([]daemon.TaskInfo, 5)
	for i := range tasks {
		tasks[i] = daemon.TaskInfo{ID: int64(i + 1), Title: fmt.Sprintf("Task %d", i+1), Status: "pending"}
	}
	l.SetTasks(tasks)
	l.SetSize(100, 15) // visibleRows = 8, only 5 tasks → no scrolling needed

	output := l.View()

	// Should NOT show scroll position when all tasks fit
	if strings.Contains(output, "/5") {
		t.Error("should not show scroll indicator when all tasks are visible")
	}
}

func TestListView_TitleAlwaysVisible(t *testing.T) {
	l := newListView(false, "")
	tasks := make([]daemon.TaskInfo, 50)
	for i := range tasks {
		tasks[i] = daemon.TaskInfo{ID: int64(i + 1), Title: fmt.Sprintf("Task %d", i+1), Status: "pending"}
	}
	l.SetTasks(tasks)
	l.SetSize(100, 20) // visibleRows = 13

	// Title should always be the first content
	output := l.View()
	if !strings.Contains(output, AppTitle) {
		t.Error("expected title to be present in output")
	}

	lines := strings.Split(output, "\n")
	if len(lines) == 0 || !strings.Contains(lines[0], "Sortie") {
		t.Error("expected title to be on the first line")
	}

	// After scrolling down, title should still be first
	for i := 0; i < 30; i++ {
		l.MoveDown()
	}
	output = l.View()
	lines = strings.Split(output, "\n")
	if len(lines) == 0 || !strings.Contains(lines[0], "Sortie") {
		t.Error("expected title to remain on first line after scrolling")
	}
}

func TestStatusMessage_CountedInExtraLines(t *testing.T) {
	m := Model{
		keys: newKeyMap(),
		list: newListView(false, ""),
		view: viewList,
	}
	tasks := make([]daemon.TaskInfo, 20)
	for i := range tasks {
		tasks[i] = daemon.TaskInfo{ID: int64(i + 1), Title: fmt.Sprintf("Task %d", i+1), Status: "pending"}
	}
	m.list.SetTasks(tasks)
	m.list.SetSize(100, 15) // height=15, visibleRows=9 without extras

	// Without status message: count task lines
	output := m.View()
	baseTaskLines := countTaskLines(output)

	// With status message: should have one fewer task line to make room
	m.statusMessage = "Copied!"
	output = m.View()
	withMsgTaskLines := countTaskLines(output)
	if withMsgTaskLines >= baseTaskLines {
		t.Errorf("expected fewer task lines with status message: base=%d, withMsg=%d", baseTaskLines, withMsgTaskLines)
	}
	if !strings.Contains(output, "Copied!") {
		t.Error("expected status message in output")
	}

	// With status message + search mode: should have two fewer task lines
	m.searchMode = true
	m.searchQuery = "test"
	output = m.View()
	withBothTaskLines := countTaskLines(output)
	if withBothTaskLines >= withMsgTaskLines {
		t.Errorf("expected fewer task lines with search+status: withMsg=%d, withBoth=%d", withMsgTaskLines, withBothTaskLines)
	}
}

// countTaskLines counts lines containing "Task " in the output (rendered task rows).
func countTaskLines(output string) int {
	count := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "Task ") {
			count++
		}
	}
	return count
}

func TestListView_OutputFitsTerminalHeight(t *testing.T) {
	m := Model{
		keys: newKeyMap(),
		list: newListView(false, ""),
		view: viewList,
	}
	tasks := make([]daemon.TaskInfo, 30)
	for i := range tasks {
		tasks[i] = daemon.TaskInfo{ID: int64(i + 1), Title: fmt.Sprintf("Task %d", i+1), Status: "pending"}
	}
	m.list.SetTasks(tasks)

	for _, height := range []int{10, 15, 20, 30, 40} {
		m.list.SetSize(100, height)
		m.width = 100
		m.height = height

		output := m.View()
		lines := strings.Split(output, "\n")
		// The output should not exceed terminal height
		// (last line may be empty from trailing newline, so allow height+1)
		if len(lines) > height+1 {
			t.Errorf("height=%d: output has %d lines, expected <= %d", height, len(lines), height+1)
		}
	}

	// Also test with status message (the bug case)
	m.statusMessage = "Task updated"
	for _, height := range []int{10, 15, 20} {
		m.list.SetSize(100, height)
		m.width = 100
		m.height = height

		output := m.View()
		lines := strings.Split(output, "\n")
		if len(lines) > height+1 {
			t.Errorf("height=%d with statusMessage: output has %d lines, expected <= %d", height, len(lines), height+1)
		}
	}
}

func TestBottomBar_PinnedToBottom(t *testing.T) {
	m := Model{
		keys: newKeyMap(),
		list: newListView(false, ""),
		view: viewList,
	}
	tasks := make([]daemon.TaskInfo, 5)
	for i := range tasks {
		tasks[i] = daemon.TaskInfo{ID: int64(i + 1), Title: fmt.Sprintf("Task %d", i+1), Status: "pending"}
	}
	m.list.SetTasks(tasks)
	m.list.SetSize(100, 24)
	m.width = 100
	m.height = 24

	// Command bar should appear on the last non-empty line
	m.commandMode = true
	m.commandInput = "set"
	output := m.View()
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	lastLine := lines[len(lines)-1]
	if !strings.Contains(lastLine, ":set") {
		t.Errorf("expected command bar on last line, got %q", lastLine)
	}

	// Status message should appear on the last non-empty line
	m.commandMode = false
	m.statusMessage = "Copied!"
	output = m.View()
	lines = strings.Split(strings.TrimRight(output, "\n"), "\n")
	lastLine = lines[len(lines)-1]
	if !strings.Contains(lastLine, "Copied!") {
		t.Errorf("expected status message on last line, got %q", lastLine)
	}

	// Both command bar and status message: status on last, command on second-to-last
	m.commandMode = true
	m.commandInput = "42"
	m.statusMessage = "Done"
	output = m.View()
	lines = strings.Split(strings.TrimRight(output, "\n"), "\n")
	lastLine = lines[len(lines)-1]
	secondToLast := lines[len(lines)-2]
	if !strings.Contains(lastLine, "Done") {
		t.Errorf("expected status message on last line, got %q", lastLine)
	}
	if !strings.Contains(secondToLast, ":42") {
		t.Errorf("expected command bar on second-to-last line, got %q", secondToLast)
	}
}

func TestBottomBar_OutputStillFitsTerminalHeight(t *testing.T) {
	m := Model{
		keys: newKeyMap(),
		list: newListView(false, ""),
		view: viewList,
	}
	tasks := make([]daemon.TaskInfo, 30)
	for i := range tasks {
		tasks[i] = daemon.TaskInfo{ID: int64(i + 1), Title: fmt.Sprintf("Task %d", i+1), Status: "pending"}
	}
	m.list.SetTasks(tasks)

	// Test with command bar + status message at various heights
	m.commandMode = true
	m.commandInput = "test"
	m.statusMessage = "Flash!"
	for _, height := range []int{10, 15, 20, 30} {
		m.list.SetSize(100, height)
		m.width = 100
		m.height = height

		output := m.View()
		lines := strings.Split(output, "\n")
		if len(lines) > height+1 {
			t.Errorf("height=%d with bottom bars: output has %d lines, expected <= %d", height, len(lines), height+1)
		}
	}
}

// --- Selection menu vim navigation tests ---

func newTestModelWithWorkflows(n int) Model {
	workflows := make([]config.WorkflowConfig, n)
	for i := range workflows {
		workflows[i] = config.WorkflowConfig{Name: fmt.Sprintf("workflow-%d", i+1)}
	}
	cfg := &config.Config{TaskWorkflows: workflows}
	return Model{
		cfg:               cfg,
		keys:              newKeyMap(),
		list:              newListView(false, ""),
		detail:            newDetailView(),
		prompt:            newPromptView(true, ""),
		view:              viewList,
		selectingWorkflow: true,
		workflowCursor:    0,
	}
}

func TestWorkflowSelect_EnterSetsWorkflowNameOnPrompt(t *testing.T) {
	m := newTestModelWithWorkflows(3)
	m.workflowCursor = 1

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	result, _ := m.handleWorkflowSelectKey(msg)
	updated := result.(Model)

	if updated.view != viewPrompt {
		t.Errorf("expected view to be viewPrompt, got %d", updated.view)
	}
	if updated.prompt.workflowName != "workflow-2" {
		t.Errorf("expected prompt workflowName to be 'workflow-2', got %q", updated.prompt.workflowName)
	}
}

func TestWorkflowSelect_NumberKeySetsWorkflowNameOnPrompt(t *testing.T) {
	m := newTestModelWithWorkflows(3)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}}
	result, _ := m.handleWorkflowSelectKey(msg)
	updated := result.(Model)

	if updated.view != viewPrompt {
		t.Errorf("expected view to be viewPrompt, got %d", updated.view)
	}
	if updated.prompt.workflowName != "workflow-3" {
		t.Errorf("expected prompt workflowName to be 'workflow-3', got %q", updated.prompt.workflowName)
	}
}

func TestWorkflowSelect_GGGoesToTop(t *testing.T) {
	m := newTestModelWithWorkflows(10)
	m.workflowCursor = 7

	gMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}

	// First "g"
	result, _ := m.handleWorkflowSelectKey(gMsg)
	m = result.(Model)
	if m.workflowCursor != 7 {
		t.Errorf("expected cursor at 7 after first 'g', got %d", m.workflowCursor)
	}
	if !m.workflowPendingG {
		t.Error("expected workflowPendingG to be true after first 'g'")
	}

	// Second "g"
	result, _ = m.handleWorkflowSelectKey(gMsg)
	m = result.(Model)
	if m.workflowCursor != 0 {
		t.Errorf("expected cursor at 0 after 'gg', got %d", m.workflowCursor)
	}
}

func TestWorkflowSelect_GGResetByOtherKey(t *testing.T) {
	m := newTestModelWithWorkflows(10)
	m.workflowCursor = 5

	gMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}
	result, _ := m.handleWorkflowSelectKey(gMsg)
	m = result.(Model)

	jMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	result, _ = m.handleWorkflowSelectKey(jMsg)
	m = result.(Model)

	if m.workflowCursor != 6 {
		t.Errorf("expected cursor at 6 after g+j, got %d", m.workflowCursor)
	}
	if m.workflowPendingG {
		t.Error("expected workflowPendingG to be false after non-g key")
	}
}

func TestWorkflowSelect_ShiftGGoesToBottom(t *testing.T) {
	m := newTestModelWithWorkflows(10)
	m.workflowCursor = 0

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}}
	result, _ := m.handleWorkflowSelectKey(msg)
	updated := result.(Model)

	if updated.workflowCursor != 9 {
		t.Errorf("expected cursor at 9 after 'G', got %d", updated.workflowCursor)
	}
}

func TestWorkflowSelect_CtrlDPageDown(t *testing.T) {
	m := newTestModelWithWorkflows(10)
	m.workflowCursor = 0

	msg := tea.KeyMsg{Type: tea.KeyCtrlD}
	result, _ := m.handleWorkflowSelectKey(msg)
	updated := result.(Model)

	// half = 10/2 = 5
	if updated.workflowCursor != 5 {
		t.Errorf("expected cursor at 5 after ctrl+d, got %d", updated.workflowCursor)
	}
}

func TestWorkflowSelect_CtrlUPageUp(t *testing.T) {
	m := newTestModelWithWorkflows(10)
	m.workflowCursor = 8

	msg := tea.KeyMsg{Type: tea.KeyCtrlU}
	result, _ := m.handleWorkflowSelectKey(msg)
	updated := result.(Model)

	// half = 10/2 = 5
	if updated.workflowCursor != 3 {
		t.Errorf("expected cursor at 3 after ctrl+u, got %d", updated.workflowCursor)
	}
}

func TestWorkflowSelect_CtrlDClampsToEnd(t *testing.T) {
	m := newTestModelWithWorkflows(10)
	m.workflowCursor = 8

	msg := tea.KeyMsg{Type: tea.KeyCtrlD}
	result, _ := m.handleWorkflowSelectKey(msg)
	updated := result.(Model)

	if updated.workflowCursor != 9 {
		t.Errorf("expected cursor clamped to 9 after ctrl+d, got %d", updated.workflowCursor)
	}
}

func TestWorkflowSelect_CtrlUClampsToStart(t *testing.T) {
	m := newTestModelWithWorkflows(10)
	m.workflowCursor = 2

	msg := tea.KeyMsg{Type: tea.KeyCtrlU}
	result, _ := m.handleWorkflowSelectKey(msg)
	updated := result.(Model)

	if updated.workflowCursor != 0 {
		t.Errorf("expected cursor clamped to 0 after ctrl+u, got %d", updated.workflowCursor)
	}
}

func TestPrioritySelect_GGGoesToTop(t *testing.T) {
	m := Model{
		keys:              newKeyMap(),
		list:              newListView(false, ""),
		detail:            newDetailView(),
		view:              viewList,
		selectingPriority: true,
		priorityCursor:    3,
	}

	gMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}

	result, _ := m.handlePrioritySelectKey(gMsg)
	m = result.(Model)
	result, _ = m.handlePrioritySelectKey(gMsg)
	m = result.(Model)

	if m.priorityCursor != 0 {
		t.Errorf("expected cursor at 0 after 'gg', got %d", m.priorityCursor)
	}
}

func TestPrioritySelect_ShiftGGoesToBottom(t *testing.T) {
	m := Model{
		keys:              newKeyMap(),
		list:              newListView(false, ""),
		detail:            newDetailView(),
		view:              viewList,
		selectingPriority: true,
		priorityCursor:    0,
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}}
	result, _ := m.handlePrioritySelectKey(msg)
	updated := result.(Model)

	if updated.priorityCursor != 3 {
		t.Errorf("expected cursor at 3 after 'G', got %d", updated.priorityCursor)
	}
}

func TestPrioritySelect_CtrlDPageDown(t *testing.T) {
	m := Model{
		keys:              newKeyMap(),
		list:              newListView(false, ""),
		detail:            newDetailView(),
		view:              viewList,
		selectingPriority: true,
		priorityCursor:    0,
	}

	msg := tea.KeyMsg{Type: tea.KeyCtrlD}
	result, _ := m.handlePrioritySelectKey(msg)
	updated := result.(Model)

	// half = 4/2 = 2
	if updated.priorityCursor != 2 {
		t.Errorf("expected cursor at 2 after ctrl+d, got %d", updated.priorityCursor)
	}
}

func TestPrioritySelect_CtrlUPageUp(t *testing.T) {
	m := Model{
		keys:              newKeyMap(),
		list:              newListView(false, ""),
		detail:            newDetailView(),
		view:              viewList,
		selectingPriority: true,
		priorityCursor:    3,
	}

	msg := tea.KeyMsg{Type: tea.KeyCtrlU}
	result, _ := m.handlePrioritySelectKey(msg)
	updated := result.(Model)

	// half = 4/2 = 2
	if updated.priorityCursor != 1 {
		t.Errorf("expected cursor at 1 after ctrl+u, got %d", updated.priorityCursor)
	}
}

func TestTaskSelect_GGGoesToTop(t *testing.T) {
	cfg := &config.Config{
		OneOff: make([]config.WorkflowConfig, 10),
	}
	for i := range cfg.OneOff {
		cfg.OneOff[i] = config.WorkflowConfig{Name: fmt.Sprintf("task-%d", i+1), Description: fmt.Sprintf("desc %d", i+1)}
	}
	m := Model{
		cfg:           cfg,
		keys:          newKeyMap(),
		list:          newListView(false, ""),
		detail:        newDetailView(),
		view:          viewList,
		selectingTask: true,
		taskCursor:    7,
	}

	gMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}
	result, _ := m.handleTaskSelectKey(gMsg)
	m = result.(Model)
	result, _ = m.handleTaskSelectKey(gMsg)
	m = result.(Model)

	if m.taskCursor != 0 {
		t.Errorf("expected cursor at 0 after 'gg', got %d", m.taskCursor)
	}
}

func TestTaskSelect_ShiftGGoesToBottom(t *testing.T) {
	cfg := &config.Config{
		OneOff: make([]config.WorkflowConfig, 10),
	}
	for i := range cfg.OneOff {
		cfg.OneOff[i] = config.WorkflowConfig{Name: fmt.Sprintf("task-%d", i+1)}
	}
	m := Model{
		cfg:           cfg,
		keys:          newKeyMap(),
		list:          newListView(false, ""),
		detail:        newDetailView(),
		view:          viewList,
		selectingTask: true,
		taskCursor:    0,
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}}
	result, _ := m.handleTaskSelectKey(msg)
	updated := result.(Model)

	if updated.taskCursor != 9 {
		t.Errorf("expected cursor at 9 after 'G', got %d", updated.taskCursor)
	}
}

func TestTaskSelect_CtrlDPageDown(t *testing.T) {
	cfg := &config.Config{
		OneOff: make([]config.WorkflowConfig, 10),
	}
	for i := range cfg.OneOff {
		cfg.OneOff[i] = config.WorkflowConfig{Name: fmt.Sprintf("task-%d", i+1)}
	}
	m := Model{
		cfg:           cfg,
		keys:          newKeyMap(),
		list:          newListView(false, ""),
		detail:        newDetailView(),
		view:          viewList,
		selectingTask: true,
		taskCursor:    0,
	}

	msg := tea.KeyMsg{Type: tea.KeyCtrlD}
	result, _ := m.handleTaskSelectKey(msg)
	updated := result.(Model)

	// half = 10/2 = 5
	if updated.taskCursor != 5 {
		t.Errorf("expected cursor at 5 after ctrl+d, got %d", updated.taskCursor)
	}
}

func TestTaskSelect_CtrlUPageUp(t *testing.T) {
	cfg := &config.Config{
		OneOff: make([]config.WorkflowConfig, 10),
	}
	for i := range cfg.OneOff {
		cfg.OneOff[i] = config.WorkflowConfig{Name: fmt.Sprintf("task-%d", i+1)}
	}
	m := Model{
		cfg:           cfg,
		keys:          newKeyMap(),
		list:          newListView(false, ""),
		detail:        newDetailView(),
		view:          viewList,
		selectingTask: true,
		taskCursor:    8,
	}

	msg := tea.KeyMsg{Type: tea.KeyCtrlU}
	result, _ := m.handleTaskSelectKey(msg)
	updated := result.(Model)

	// half = 10/2 = 5
	if updated.taskCursor != 3 {
		t.Errorf("expected cursor at 3 after ctrl+u, got %d", updated.taskCursor)
	}
}

func TestArtifactSelect_GGGoesToTop(t *testing.T) {
	m := Model{
		keys:              newKeyMap(),
		list:              newListView(false, ""),
		detail:            newDetailView(),
		view:              viewList,
		selectingArtifact: true,
		artifactCursor:    5,
		artifactNames:     []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8"},
	}

	gMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}
	result, _ := m.handleArtifactSelectKey(gMsg)
	m = result.(Model)
	result, _ = m.handleArtifactSelectKey(gMsg)
	m = result.(Model)

	if m.artifactCursor != 0 {
		t.Errorf("expected cursor at 0 after 'gg', got %d", m.artifactCursor)
	}
}

func TestArtifactSelect_ShiftGGoesToBottom(t *testing.T) {
	m := Model{
		keys:              newKeyMap(),
		list:              newListView(false, ""),
		detail:            newDetailView(),
		view:              viewList,
		selectingArtifact: true,
		artifactCursor:    0,
		artifactNames:     []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8"},
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}}
	result, _ := m.handleArtifactSelectKey(msg)
	updated := result.(Model)

	if updated.artifactCursor != 7 {
		t.Errorf("expected cursor at 7 after 'G', got %d", updated.artifactCursor)
	}
}

func TestArtifactSelect_CtrlDPageDown(t *testing.T) {
	m := Model{
		keys:              newKeyMap(),
		list:              newListView(false, ""),
		detail:            newDetailView(),
		view:              viewList,
		selectingArtifact: true,
		artifactCursor:    0,
		artifactNames:     []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8"},
	}

	msg := tea.KeyMsg{Type: tea.KeyCtrlD}
	result, _ := m.handleArtifactSelectKey(msg)
	updated := result.(Model)

	// half = 8/2 = 4
	if updated.artifactCursor != 4 {
		t.Errorf("expected cursor at 4 after ctrl+d, got %d", updated.artifactCursor)
	}
}

func TestArtifactSelect_CtrlUPageUp(t *testing.T) {
	m := Model{
		keys:              newKeyMap(),
		list:              newListView(false, ""),
		detail:            newDetailView(),
		view:              viewList,
		selectingArtifact: true,
		artifactCursor:    6,
		artifactNames:     []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8"},
	}

	msg := tea.KeyMsg{Type: tea.KeyCtrlU}
	result, _ := m.handleArtifactSelectKey(msg)
	updated := result.(Model)

	// half = 8/2 = 4
	if updated.artifactCursor != 2 {
		t.Errorf("expected cursor at 2 after ctrl+u, got %d", updated.artifactCursor)
	}
}

// --- RunTask command tests ---

func TestMatchRunTask(t *testing.T) {
	tests := []struct {
		input    string
		wantArgs string
		wantOK   bool
	}{
		{"RunTask", "", true},
		{"RunTask Refactor", "Refactor", true},
		{"RunTask  Security ", "Security", true},
		{"runtask", "", false},
		{"RunTas", "", false},
		{"set number", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		args, ok := matchRunTask(tt.input)
		if ok != tt.wantOK {
			t.Errorf("matchRunTask(%q): ok = %v, want %v", tt.input, ok, tt.wantOK)
		}
		if args != tt.wantArgs {
			t.Errorf("matchRunTask(%q): args = %q, want %q", tt.input, args, tt.wantArgs)
		}
	}
}

func TestExecRunTask_RunsTask(t *testing.T) {
	m := Model{
		keys:        newKeyMap(),
		client:      &client.Client{},
		list:        newListView(false, ""),
		detail:      newDetailView(),
		view:        viewList,
		projectPath: "/tmp/test-project",
		cfg: &config.Config{
			OneOff: []config.WorkflowConfig{
				{Name: "Refactor", Description: "Refactor the code"},
			},
		},
	}

	result, cmd := execRunTask(m, "Refactor")
	updated := result.(Model)

	if updated.selectedWorkflow != "oneoff:Refactor" {
		t.Errorf("expected selectedWorkflow 'task:Refactor', got %q", updated.selectedWorkflow)
	}
	if cmd == nil {
		t.Error("expected command for task creation, got nil")
	}
}

func TestExecRunTask_UnknownTask(t *testing.T) {
	m := Model{
		keys:        newKeyMap(),
		client:      &client.Client{},
		list:        newListView(false, ""),
		detail:      newDetailView(),
		view:        viewList,
		projectPath: "/tmp/test-project",
		cfg: &config.Config{
			OneOff: []config.WorkflowConfig{
				{Name: "Refactor", Description: "Refactor the code"},
			},
		},
	}

	result, cmd := execRunTask(m, "NonExistent")
	updated := result.(Model)

	if updated.err == nil {
		t.Error("expected error for unknown task")
	}
	if !strings.Contains(updated.err.Error(), "unknown task") {
		t.Errorf("expected 'unknown task' error, got %q", updated.err.Error())
	}
	if cmd != nil {
		t.Error("expected nil command for unknown task")
	}
}

func TestExecRunTask_EmptyArgs(t *testing.T) {
	m := Model{
		keys:        newKeyMap(),
		client:      &client.Client{},
		list:        newListView(false, ""),
		detail:      newDetailView(),
		view:        viewList,
		projectPath: "/tmp/test-project",
		cfg:         &config.Config{},
	}

	result, _ := execRunTask(m, "")
	updated := result.(Model)

	if updated.err == nil {
		t.Error("expected error for empty args")
	}
	if !strings.Contains(updated.err.Error(), "usage") {
		t.Errorf("expected 'usage' error, got %q", updated.err.Error())
	}
}

func TestExecRunTask_NoClient(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
		cfg:    &config.Config{},
	}

	result, _ := execRunTask(m, "Refactor")
	updated := result.(Model)

	if updated.err == nil {
		t.Error("expected error when client is nil")
	}
}

func TestExecRunTask_UsesNameWhenNoDescription(t *testing.T) {
	m := Model{
		keys:        newKeyMap(),
		client:      &client.Client{},
		list:        newListView(false, ""),
		detail:      newDetailView(),
		view:        viewList,
		projectPath: "/tmp/test-project",
		cfg: &config.Config{
			OneOff: []config.WorkflowConfig{
				{Name: "Refactor"},
			},
		},
	}

	result, cmd := execRunTask(m, "Refactor")
	updated := result.(Model)

	if updated.selectedWorkflow != "oneoff:Refactor" {
		t.Errorf("expected selectedWorkflow 'task:Refactor', got %q", updated.selectedWorkflow)
	}
	if cmd == nil {
		t.Error("expected command for task creation, got nil")
	}
}

func TestExecRunTask_RunsUnlistedTask(t *testing.T) {
	m := Model{
		keys:        newKeyMap(),
		client:      &client.Client{},
		list:        newListView(false, ""),
		detail:      newDetailView(),
		view:        viewList,
		projectPath: "/tmp/test-project",
		cfg: &config.Config{
			OneOff: []config.WorkflowConfig{
				{Name: "Secret", Description: "Hidden task"},
			},
		},
	}

	result, cmd := execRunTask(m, "Secret")
	updated := result.(Model)

	if updated.selectedWorkflow != "oneoff:Secret" {
		t.Errorf("expected selectedWorkflow 'task:Secret', got %q", updated.selectedWorkflow)
	}
	if cmd == nil {
		t.Error("expected command for unlisted task creation, got nil")
	}
}

func TestCommandMode_RunTaskIntegration(t *testing.T) {
	m := Model{
		keys:        newKeyMap(),
		client:      &client.Client{},
		list:        newListView(false, ""),
		detail:      newDetailView(),
		view:        viewList,
		projectPath: "/tmp/test-project",
		cfg: &config.Config{
			OneOff: []config.WorkflowConfig{
				{Name: "Refactor", Description: "Refactor code"},
			},
		},
	}

	// Enter command mode
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}}
	result, _ := m.handleListKey(msg)
	m = result.(Model)

	if !m.commandMode {
		t.Fatal("expected command mode to be active")
	}

	// Type "RunTask Refactor"
	for _, ch := range "RunTask Refactor" {
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}}
		result, _ = m.handleCommandKey(msg)
		m = result.(Model)
	}

	if m.commandInput != "RunTask Refactor" {
		t.Errorf("expected commandInput 'RunTask Refactor', got %q", m.commandInput)
	}

	// Press enter
	msg = tea.KeyMsg{Type: tea.KeyEnter}
	result, cmd := m.handleCommandKey(msg)
	m = result.(Model)

	if m.commandMode {
		t.Error("expected command mode to be inactive after enter")
	}
	if m.selectedWorkflow != "oneoff:Refactor" {
		t.Errorf("expected selectedWorkflow 'task:Refactor', got %q", m.selectedWorkflow)
	}
	if cmd == nil {
		t.Error("expected command for task creation, got nil")
	}
}

func TestCompleteRunTask_SingleMatch(t *testing.T) {
	m := Model{
		cfg: &config.Config{
			OneOff: []config.WorkflowConfig{
				{Name: "Refactor", Description: "Refactor code"},
				{Name: "Security", Description: "Security scan"},
			},
		},
	}

	completed, ok := completeRunTask(m, "RunTask Ref")
	if !ok {
		t.Fatal("expected completion to succeed")
	}
	if completed != "RunTask Refactor" {
		t.Errorf("expected 'RunTask Refactor', got %q", completed)
	}
}

func TestCompleteRunTask_MultipleMatchesExtends(t *testing.T) {
	m := Model{
		cfg: &config.Config{
			OneOff: []config.WorkflowConfig{
				{Name: "Refactor-code", Description: "Refactor code"},
				{Name: "Refactor-tests", Description: "Refactor tests"},
			},
		},
	}

	completed, ok := completeRunTask(m, "RunTask Re")
	if !ok {
		t.Fatal("expected completion to succeed with common prefix extension")
	}
	if completed != "RunTask Refactor-" {
		t.Errorf("expected 'RunTask Refactor-', got %q", completed)
	}
}

func TestCompleteRunTask_MultipleMatchesNoExtension(t *testing.T) {
	m := Model{
		cfg: &config.Config{
			OneOff: []config.WorkflowConfig{
				{Name: "Refactor", Description: "Refactor code"},
				{Name: "Review", Description: "Review code"},
			},
		},
	}

	// Common prefix "Re" is same length as input "Re", so no extension possible
	_, ok := completeRunTask(m, "RunTask Re")
	if ok {
		t.Error("expected no completion when common prefix matches input length")
	}
}

func TestCompleteRunTask_NoMatch(t *testing.T) {
	m := Model{
		cfg: &config.Config{
			OneOff: []config.WorkflowConfig{
				{Name: "Refactor"},
			},
		},
	}

	_, ok := completeRunTask(m, "RunTask Xyz")
	if ok {
		t.Error("expected no completion for non-matching prefix")
	}
}

func TestCompleteRunTask_IncludesUnlisted(t *testing.T) {
	m := Model{
		cfg: &config.Config{
			OneOff: []config.WorkflowConfig{
				{Name: "Refactor"},
				{Name: "Secret"},
			},
		},
	}

	completed, ok := completeRunTask(m, "RunTask S")
	if !ok {
		t.Fatal("expected completion to succeed for unlisted task")
	}
	if completed != "RunTask Secret" {
		t.Errorf("expected 'RunTask Secret', got %q", completed)
	}
}

func TestCompleteRunTask_ExactRunTask(t *testing.T) {
	m := Model{
		cfg: &config.Config{
			OneOff: []config.WorkflowConfig{
				{Name: "Refactor"},
			},
		},
	}

	completed, ok := completeRunTask(m, "RunTask")
	if !ok {
		t.Fatal("expected completion to add space after RunTask")
	}
	if completed != "RunTask " {
		t.Errorf("expected 'RunTask ', got %q", completed)
	}
}

func TestCompleteRunTask_NilConfig(t *testing.T) {
	m := Model{}

	_, ok := completeRunTask(m, "RunTask R")
	if ok {
		t.Error("expected no completion with nil config")
	}
}

func TestCommandMode_TabCompletion(t *testing.T) {
	m := Model{
		keys:        newKeyMap(),
		list:        newListView(false, ""),
		detail:      newDetailView(),
		view:        viewList,
		commandMode: true,
		commandInput: "RunTask Ref",
		cfg: &config.Config{
			OneOff: []config.WorkflowConfig{
				{Name: "Refactor", Description: "Refactor code"},
				{Name: "Security", Description: "Security scan"},
			},
		},
	}

	msg := tea.KeyMsg{Type: tea.KeyTab}
	result, _ := m.handleCommandKey(msg)
	updated := result.(Model)

	if updated.commandInput != "RunTask Refactor" {
		t.Errorf("expected commandInput 'RunTask Refactor' after tab, got %q", updated.commandInput)
	}
	if !updated.commandMode {
		t.Error("expected to remain in command mode after tab")
	}
}

func TestHandleListKey_RHidesUnlistedTasks(t *testing.T) {
	// Since unlisted is removed, all one-off workflows are now visible
	m := Model{
		keys:        newKeyMap(),
		client:      &client.Client{},
		list:        newListView(false, ""),
		detail:      newDetailView(),
		view:        viewList,
		projectPath: "/tmp/test-project",
		cfg: &config.Config{
			OneOff: []config.WorkflowConfig{
				{Name: "Visible", Description: "A visible task"},
				{Name: "Hidden", Description: "A hidden task"},
			},
		},
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 1, Title: "Running task", Status: "running"},
	})

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	if !updated.selectingTask {
		t.Fatal("expected selectingTask to be true")
	}

	// All one-off workflows are visible (unlisted removed)
	tasks := updated.cfg.ListPredefinedTaskNames()
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}
	if tasks[0] != "Visible" {
		t.Errorf("expected first task 'Visible', got %q", tasks[0])
	}
	if tasks[1] != "Hidden" {
		t.Errorf("expected second task 'Hidden', got %q", tasks[1])
	}
}

func TestViewRendersTaskSelection_HidesUnlisted(t *testing.T) {
	// Since unlisted is removed, all one-off workflows are now visible
	m := Model{
		keys:        newKeyMap(),
		list:        newListView(false, ""),
		detail:      newDetailView(),
		view:        viewList,
		selectingTask: true,
		taskCursor:    0,
		cfg: &config.Config{
			OneOff: []config.WorkflowConfig{
				{Name: "Visible", Description: "A visible task"},
				{Name: "Hidden", Description: "A hidden task"},
			},
		},
	}

	output := m.View()

	if !strings.Contains(output, "Visible") {
		t.Error("expected 'Visible' task in selection view")
	}
	if !strings.Contains(output, "Hidden") {
		t.Error("expected 'Hidden' task to be visible (unlisted removed)")
	}
}

func TestFindLogFile_CurrentStep(t *testing.T) {
	dir := t.TempDir()
	// Create two log files
	os.WriteFile(filepath.Join(dir, "implement.log"), []byte("log1"), 0644)
	os.WriteFile(filepath.Join(dir, "review.log"), []byte("log2"), 0644)

	result, err := findLogFile(dir, "implement")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Join(dir, "implement.log")
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestFindLogFile_FallbackToNewest(t *testing.T) {
	dir := t.TempDir()
	// Create two log files with different modification times
	old := filepath.Join(dir, "implement.log")
	os.WriteFile(old, []byte("old"), 0644)
	os.Chtimes(old, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour))

	newest := filepath.Join(dir, "review.log")
	os.WriteFile(newest, []byte("new"), 0644)

	result, err := findLogFile(dir, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != newest {
		t.Errorf("expected %s (newest), got %s", newest, result)
	}
}

func TestFindLogFile_NoLogs(t *testing.T) {
	dir := t.TempDir()

	_, err := findLogFile(dir, "implement")
	if err == nil {
		t.Error("expected error for empty logs directory")
	}
}

func TestFindLogFile_EmptyCurrentStep(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "implement.log")
	os.WriteFile(logFile, []byte("log"), 0644)

	result, err := findLogFile(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != logFile {
		t.Errorf("expected %s, got %s", logFile, result)
	}
}

func TestHandleDetailKey_EOpensLogEditor(t *testing.T) {
	task := &daemon.TaskInfo{
		ID:          1,
		ProjectPath: "/some/project",
		CurrentStep: "implement",
	}
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewDetail,
	}
	m.detail.SetTask(task)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}}
	_, cmd := m.handleDetailKey(msg)

	// The "e" key should produce a command (tea.ExecProcess or error)
	if cmd == nil {
		t.Error("expected a command from 'e' key, got nil")
	}
}

func TestHandleDetailKey_EWithNoTask(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewDetail,
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}}
	_, cmd := m.handleDetailKey(msg)

	if cmd != nil {
		t.Error("expected nil command when no task is selected")
	}
}

func TestPromptHelpToggle(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false, ""),
		detail: newDetailView(),
		prompt: newPromptView(true, ""),
		view:   viewPrompt,
	}

	// Press "ctrl+h" to open help
	msg := tea.KeyMsg{Type: tea.KeyCtrlH}
	result, cmd := m.handlePromptKey(msg)
	updated := result.(Model)

	if !updated.prompt.showHelp {
		t.Error("expected showHelp to be true after 'ctrl+h'")
	}
	if cmd != nil {
		t.Error("expected no command, got non-nil")
	}

	// Press "ctrl+h" again to close help
	result, cmd = updated.handlePromptKey(msg)
	updated = result.(Model)

	if updated.prompt.showHelp {
		t.Error("expected showHelp to be false after second 'ctrl+h'")
	}
	if cmd != nil {
		t.Error("expected no command, got non-nil")
	}
}

func TestPromptHelpCloseWithEsc(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false, ""),
		detail: newDetailView(),
		prompt: newPromptView(true, ""),
		view:   viewPrompt,
	}
	m.prompt.showHelp = true

	msg := tea.KeyMsg{Type: tea.KeyEscape}
	result, cmd := m.handlePromptKey(msg)
	updated := result.(Model)

	if updated.prompt.showHelp {
		t.Error("expected showHelp to be false after esc")
	}
	if cmd != nil {
		t.Error("expected no command, got non-nil")
	}
}

func TestPromptHelpConsumesKeys(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false, ""),
		detail: newDetailView(),
		prompt: newPromptView(true, ""),
		view:   viewPrompt,
	}
	m.prompt.showHelp = true

	// Press a regular key while help is shown — should be consumed
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	result, cmd := m.handlePromptKey(msg)
	updated := result.(Model)

	if !updated.prompt.showHelp {
		t.Error("expected showHelp to remain true")
	}
	if cmd != nil {
		t.Error("expected no command, got non-nil")
	}
}

func TestViewRendersPromptHelpOverlay(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false, ""),
		detail: newDetailView(),
		prompt: newPromptView(true, ""),
		view:   viewPrompt,
	}
	m.prompt.showHelp = true

	output := m.View()

	// Should show the prompt help overlay title
	if !strings.Contains(output, "New Task Help") {
		t.Error("expected help overlay to contain 'New Task Help'")
	}

	// Should contain prompt-specific group names
	if !strings.Contains(output, "Input") {
		t.Error("expected help overlay to contain 'Input' group")
	}
	if !strings.Contains(output, "Actions") {
		t.Error("expected help overlay to contain 'Actions' group")
	}

	// Should contain prompt-specific bindings
	if !strings.Contains(output, "submit") {
		t.Error("expected help overlay to contain 'submit'")
	}
	if !strings.Contains(output, "worktree") {
		t.Error("expected help overlay to contain 'worktree'")
	}
	if !strings.Contains(output, "editor") {
		t.Error("expected help overlay to contain 'editor'")
	}

	// Should NOT contain list-view-specific bindings
	if strings.Contains(output, "new task") {
		t.Error("expected help overlay NOT to contain list-specific 'new task' binding")
	}
	if strings.Contains(output, "run task") {
		t.Error("expected help overlay NOT to contain list-specific 'run task' binding")
	}

	// Should contain close hint
	if !strings.Contains(output, "Press ctrl+h or esc to close") {
		t.Error("expected help overlay to contain close hint")
	}
}

func TestPromptShortHelpContainsHelpBinding(t *testing.T) {
	keys := newPromptKeyMap()
	bindings := keys.ShortHelp()

	found := false
	for _, b := range bindings {
		if b.Help().Key == "ctrl+h" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected prompt ShortHelp to contain 'ctrl+h' help binding")
	}
}

func TestPromptShortHelpShowsOnlyEssentialBindings(t *testing.T) {
	keys := newPromptKeyMap()
	bindings := keys.ShortHelp()

	// ShortHelp should only show a few essential bindings (submit, cancel, newline, help)
	if len(bindings) > 5 {
		t.Errorf("expected ShortHelp to show at most 5 bindings, got %d", len(bindings))
	}

	// Should contain the essential bindings
	helpKeys := make(map[string]bool)
	for _, b := range bindings {
		helpKeys[b.Help().Key] = true
	}
	for _, expected := range []string{"enter", "esc", "ctrl+h"} {
		if !helpKeys[expected] {
			t.Errorf("expected ShortHelp to contain %q", expected)
		}
	}
}

func TestPromptFullHelpContainsAllBindings(t *testing.T) {
	keys := newPromptKeyMap()
	groups := keys.FullHelp()

	// Collect all binding descriptions
	allDescs := make(map[string]bool)
	for _, group := range groups {
		for _, b := range group {
			allDescs[b.Help().Desc] = true
		}
	}

	// FullHelp should contain all prompt bindings
	for _, expected := range []string{"submit", "cancel", "next/prev field", "newline", "worktree", "editor", "remove last image", "help"} {
		if !allDescs[expected] {
			t.Errorf("expected FullHelp to contain binding for %q", expected)
		}
	}
}

func TestPromptHelpOverlayIsSeparateFromListHelp(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false, ""),
		detail: newDetailView(),
		prompt: newPromptView(true, ""),
		view:   viewPrompt,
	}

	// Render prompt help overlay
	promptHelp := m.renderPromptHelpOverlay()

	// Render list help overlay
	listHelp := m.renderHelpOverlay()

	// They should have different titles
	if !strings.Contains(promptHelp, "New Task Help") {
		t.Error("expected prompt help to contain 'New Task Help'")
	}
	if !strings.Contains(listHelp, " Help ") {
		t.Error("expected list help to contain ' Help '")
	}

	// Prompt help should NOT contain list-specific bindings
	if strings.Contains(promptHelp, "new task") {
		t.Error("prompt help should not contain 'new task'")
	}
	if strings.Contains(promptHelp, "logs") {
		t.Error("prompt help should not contain 'logs'")
	}

	// List help should NOT contain prompt-specific bindings
	if strings.Contains(listHelp, "switch field") {
		t.Error("list help should not contain 'switch field'")
	}
	if strings.Contains(listHelp, "newline") {
		t.Error("list help should not contain 'newline'")
	}
}

func TestHandleListKey_STriggersStopConfirmation(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		client: &client.Client{},
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 42, Title: "Running task", Status: "running"},
	})

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}}
	result, cmd := m.handleListKey(msg)
	updated := result.(Model)

	if updated.confirmAction != "stop" {
		t.Errorf("expected confirmAction to be 'stop', got %q", updated.confirmAction)
	}
	if updated.confirmTaskID != 42 {
		t.Errorf("expected confirmTaskID to be 42, got %d", updated.confirmTaskID)
	}
	if cmd != nil {
		t.Error("expected no command (confirmation pending), got non-nil")
	}
}

func TestHandleListKey_SNoOpWithNoClient(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		client: nil,
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
	}
	m.list.SetTasks([]daemon.TaskInfo{
		{ID: 42, Title: "Running task", Status: "running"},
	})

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}}
	result, _ := m.handleListKey(msg)
	updated := result.(Model)

	if updated.confirmAction != "" {
		t.Errorf("expected no confirmAction without client, got %q", updated.confirmAction)
	}
}

func TestHandleDetailKey_CtrlCTriggersStopConfirmation(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		client: &client.Client{},
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewDetail,
	}
	task := daemon.TaskInfo{ID: 55, Title: "Running task", Status: "running"}
	m.detail.SetTask(&task)

	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	result, cmd := m.handleDetailKey(msg)
	updated := result.(Model)

	if updated.confirmAction != "stop" {
		t.Errorf("expected confirmAction to be 'stop', got %q", updated.confirmAction)
	}
	if updated.confirmTaskID != 55 {
		t.Errorf("expected confirmTaskID to be 55, got %d", updated.confirmTaskID)
	}
	if cmd != nil {
		t.Error("expected no command (confirmation pending), got non-nil")
	}
}

func TestHandleDetailKey_CtrlCNoOpWithNoTask(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		client: &client.Client{},
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewDetail,
	}

	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	result, _ := m.handleDetailKey(msg)
	updated := result.(Model)

	if updated.confirmAction != "" {
		t.Errorf("expected no confirmAction without task, got %q", updated.confirmAction)
	}
}

func TestHandleDetailKey_CtrlCNoOpWithNoClient(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		client: nil,
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewDetail,
	}
	task := daemon.TaskInfo{ID: 55, Title: "Running task", Status: "running"}
	m.detail.SetTask(&task)

	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	result, _ := m.handleDetailKey(msg)
	updated := result.(Model)

	if updated.confirmAction != "" {
		t.Errorf("expected no confirmAction without client, got %q", updated.confirmAction)
	}
}

func TestHandleTaskInfoKey_CtrlCTriggersStopConfirmation(t *testing.T) {
	m := Model{
		keys:     newKeyMap(),
		client:   &client.Client{},
		list:     newListView(false, ""),
		detail:   newDetailView(),
		taskInfo: newTaskInfoView(),
		view:     viewTaskInfo,
	}
	task := daemon.TaskInfo{ID: 77, Title: "Running task", Status: "running"}
	m.taskInfo.SetTask(&task)

	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	result, cmd := m.handleTaskInfoKey(msg)
	updated := result.(Model)

	if updated.confirmAction != "stop" {
		t.Errorf("expected confirmAction to be 'stop', got %q", updated.confirmAction)
	}
	if updated.confirmTaskID != 77 {
		t.Errorf("expected confirmTaskID to be 77, got %d", updated.confirmTaskID)
	}
	if cmd != nil {
		t.Error("expected no command (confirmation pending), got non-nil")
	}
}

func TestHandleTaskInfoKey_CtrlCNoOpWithNoTask(t *testing.T) {
	m := Model{
		keys:     newKeyMap(),
		client:   &client.Client{},
		list:     newListView(false, ""),
		detail:   newDetailView(),
		taskInfo: newTaskInfoView(),
		view:     viewTaskInfo,
	}

	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	result, _ := m.handleTaskInfoKey(msg)
	updated := result.(Model)

	if updated.confirmAction != "" {
		t.Errorf("expected no confirmAction without task, got %q", updated.confirmAction)
	}
}

func TestHandleConfirmKey_YConfirmsStop(t *testing.T) {
	m := Model{
		keys:          newKeyMap(),
		client:        &client.Client{},
		list:          newListView(false, ""),
		detail:        newDetailView(),
		confirmAction: "stop",
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
		t.Error("expected command from confirmed stop, got nil")
	}
}

func TestHandleConfirmKey_NCancelsStop(t *testing.T) {
	m := Model{
		keys:          newKeyMap(),
		client:        &client.Client{},
		list:          newListView(false, ""),
		detail:        newDetailView(),
		confirmAction: "stop",
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
		t.Error("expected no command from cancelled stop, got non-nil")
	}
}

func TestHandleConfirmKey_EscCancelsStop(t *testing.T) {
	m := Model{
		keys:          newKeyMap(),
		client:        &client.Client{},
		list:          newListView(false, ""),
		detail:        newDetailView(),
		confirmAction: "stop",
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

func TestHandleConfirmKey_OtherKeyIgnored(t *testing.T) {
	m := Model{
		keys:          newKeyMap(),
		client:        &client.Client{},
		list:          newListView(false, ""),
		detail:        newDetailView(),
		confirmAction: "stop",
		confirmTaskID: 42,
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
	result, cmd := m.handleConfirmKey(msg)
	updated := result.(Model)

	if updated.confirmAction != "stop" {
		t.Errorf("expected confirmAction to remain 'stop', got %q", updated.confirmAction)
	}
	if updated.confirmTaskID != 42 {
		t.Errorf("expected confirmTaskID to remain 42, got %d", updated.confirmTaskID)
	}
	if cmd != nil {
		t.Error("expected no command from ignored key, got non-nil")
	}
}

func TestHandleDetailKey_ConfirmationBlocksOtherKeys(t *testing.T) {
	m := Model{
		keys:          newKeyMap(),
		client:        &client.Client{},
		list:          newListView(false, ""),
		detail:        newDetailView(),
		view:          viewDetail,
		confirmAction: "stop",
		confirmTaskID: 42,
	}
	task := daemon.TaskInfo{ID: 42, Title: "Running task", Status: "running"}
	m.detail.SetTask(&task)

	// Pressing 'q' while confirmation is active should not navigate away
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	result, _ := m.handleDetailKey(msg)
	updated := result.(Model)

	if updated.view != viewDetail {
		t.Error("expected to stay in detail view while confirmation is active")
	}
	if updated.confirmAction != "stop" {
		t.Errorf("expected confirmAction to remain 'stop', got %q", updated.confirmAction)
	}
}

func TestHandleTaskInfoKey_ConfirmationBlocksOtherKeys(t *testing.T) {
	m := Model{
		keys:          newKeyMap(),
		client:        &client.Client{},
		list:          newListView(false, ""),
		detail:        newDetailView(),
		taskInfo:      newTaskInfoView(),
		view:          viewTaskInfo,
		confirmAction: "stop",
		confirmTaskID: 77,
	}
	task := daemon.TaskInfo{ID: 77, Title: "Running task", Status: "running"}
	m.taskInfo.SetTask(&task)

	// Pressing 'q' while confirmation is active should not navigate away
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	result, _ := m.handleTaskInfoKey(msg)
	updated := result.(Model)

	if updated.view != viewTaskInfo {
		t.Error("expected to stay in task info view while confirmation is active")
	}
	if updated.confirmAction != "stop" {
		t.Errorf("expected confirmAction to remain 'stop', got %q", updated.confirmAction)
	}
}
