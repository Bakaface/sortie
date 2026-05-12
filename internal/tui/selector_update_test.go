package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aface/sortie/internal/client"
	"github.com/aface/sortie/internal/config"
	"github.com/aface/sortie/internal/daemon"
	tea "github.com/charmbracelet/bubbletea"
)

func TestHandleTaskSelectKey_Navigation(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		client: &client.Client{},
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
		selector: selector{
			kind:   selectorTask,
			cursor: 0,
			items:  []string{"Task A", "Task B", "Task C"},
		},
		projectPath: "/tmp/test",
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
	result, _ := m.handleSelectorKey(msg)
	updated := result.(Model)
	if updated.selector.cursor != 1 {
		t.Errorf("expected cursor at 1 after j, got %d", updated.selector.cursor)
	}

	// Move down again
	m = updated
	result, _ = m.handleSelectorKey(msg)
	updated = result.(Model)
	if updated.selector.cursor != 2 {
		t.Errorf("expected cursor at 2 after j, got %d", updated.selector.cursor)
	}

	// Move down at bottom — should stay
	m = updated
	result, _ = m.handleSelectorKey(msg)
	updated = result.(Model)
	if updated.selector.cursor != 2 {
		t.Errorf("expected cursor to stay at 2, got %d", updated.selector.cursor)
	}

	// Move up
	m = updated
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}
	result, _ = m.handleSelectorKey(msg)
	updated = result.(Model)
	if updated.selector.cursor != 1 {
		t.Errorf("expected cursor at 1 after k, got %d", updated.selector.cursor)
	}
}

func TestHandleTaskSelectKey_EscCancels(t *testing.T) {
	m := Model{
		keys: newKeyMap(),
		selector: selector{
			kind:   selectorTask,
			cursor: 1,
			items:  []string{"Task A"},
		},
		cfg: &config.Config{
			OneOff: []config.WorkflowConfig{
				{Name: "Task A"},
			},
		},
	}

	msg := tea.KeyMsg{Type: tea.KeyEscape}
	result, _ := m.handleSelectorKey(msg)
	updated := result.(Model)

	if updated.selector.kind == selectorTask {
		t.Error("expected selector kind not to be selectorTask after esc")
	}
}

func TestHandleTaskSelectKey_EnterCreatesTask(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		client: &client.Client{},
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
		selector: selector{
			kind:   selectorTask,
			cursor: 0,
			items:  []string{"Housekeeping"},
		},
		projectPath: "/tmp/test",
		cfg: &config.Config{
			OneOff: []config.WorkflowConfig{
				{Name: "Housekeeping", Description: "Clean up code"},
			},
		},
	}

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	result, cmd := m.handleSelectorKey(msg)
	updated := result.(Model)

	if updated.selector.kind == selectorTask {
		t.Error("expected selector kind not to be selectorTask after enter")
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
		keys:   newKeyMap(),
		client: &client.Client{},
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
		selector: selector{
			kind:   selectorTask,
			cursor: 0,
			items:  []string{"First", "Second"},
		},
		projectPath: "/tmp/test",
		cfg: &config.Config{
			OneOff: []config.WorkflowConfig{
				{Name: "First", Description: "First task"},
				{Name: "Second", Description: "Second task"},
			},
		},
	}

	// Press "2" to select second task
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}}
	result, cmd := m.handleSelectorKey(msg)
	updated := result.(Model)

	if updated.selector.kind == selectorTask {
		t.Error("expected selector kind not to be selectorTask after number key")
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
		keys:   newKeyMap(),
		client: &client.Client{},
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
		selector: selector{
			kind:   selectorTask,
			cursor: 0,
			items:  []string{"NoDesc"},
		},
		projectPath: "/tmp/test",
		cfg: &config.Config{
			OneOff: []config.WorkflowConfig{
				{Name: "NoDesc"},
			},
		},
	}

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	result, cmd := m.handleSelectorKey(msg)
	updated := result.(Model)

	if updated.selector.kind == selectorTask {
		t.Error("expected selector kind not to be selectorTask")
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
		keys:   newKeyMap(),
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
		selector: selector{
			kind:         selectorTask,
			title:        "Run Predefined Task",
			cursor:       0,
			items:        []string{"Housekeeping", "Security Scan"},
			descriptions: []string{"Clean up code", "Run security audit"},
		},
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

	if updated.selector.kind != selectorTask {
		t.Error("expected selector kind to be selectorTask")
	}
	if updated.selector.cursor != 0 {
		t.Errorf("expected selector cursor to be 0, got %d", updated.selector.cursor)
	}
	if cmd != nil {
		t.Error("expected no command (selection screen shown), got non-nil")
	}
}


func TestPrioritySelect_GGGoesToTop(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
		selector: selector{
			kind:   selectorPriority,
			cursor: 3,
			items:  []string{"low", "medium", "high", "urgent"},
		},
	}

	gMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}

	result, _ := m.handleSelectorKey(gMsg)
	m = result.(Model)
	result, _ = m.handleSelectorKey(gMsg)
	m = result.(Model)

	if m.selector.cursor != 0 {
		t.Errorf("expected cursor at 0 after 'gg', got %d", m.selector.cursor)
	}
}

func TestPrioritySelect_ShiftGGoesToBottom(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
		selector: selector{
			kind:   selectorPriority,
			cursor: 0,
			items:  []string{"low", "medium", "high", "urgent"},
		},
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}}
	result, _ := m.handleSelectorKey(msg)
	updated := result.(Model)

	if updated.selector.cursor != 3 {
		t.Errorf("expected cursor at 3 after 'G', got %d", updated.selector.cursor)
	}
}

func TestPrioritySelect_CtrlDPageDown(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
		selector: selector{
			kind:   selectorPriority,
			cursor: 0,
			items:  []string{"low", "medium", "high", "urgent"},
		},
	}

	msg := tea.KeyMsg{Type: tea.KeyCtrlD}
	result, _ := m.handleSelectorKey(msg)
	updated := result.(Model)

	// half = 4/2 = 2
	if updated.selector.cursor != 2 {
		t.Errorf("expected cursor at 2 after ctrl+d, got %d", updated.selector.cursor)
	}
}

func TestPrioritySelect_CtrlUPageUp(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
		selector: selector{
			kind:   selectorPriority,
			cursor: 3,
			items:  []string{"low", "medium", "high", "urgent"},
		},
	}

	msg := tea.KeyMsg{Type: tea.KeyCtrlU}
	result, _ := m.handleSelectorKey(msg)
	updated := result.(Model)

	// half = 4/2 = 2
	if updated.selector.cursor != 1 {
		t.Errorf("expected cursor at 1 after ctrl+u, got %d", updated.selector.cursor)
	}
}

func TestTaskSelect_GGGoesToTop(t *testing.T) {
	cfg := &config.Config{
		OneOff: make([]config.WorkflowConfig, 10),
	}
	items := make([]string, 10)
	for i := range cfg.OneOff {
		cfg.OneOff[i] = config.WorkflowConfig{Name: fmt.Sprintf("task-%d", i+1), Description: fmt.Sprintf("desc %d", i+1)}
		items[i] = cfg.OneOff[i].Name
	}
	m := Model{
		cfg:    cfg,
		keys:   newKeyMap(),
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
		selector: selector{
			kind:   selectorTask,
			cursor: 7,
			items:  items,
		},
	}

	gMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}
	result, _ := m.handleSelectorKey(gMsg)
	m = result.(Model)
	result, _ = m.handleSelectorKey(gMsg)
	m = result.(Model)

	if m.selector.cursor != 0 {
		t.Errorf("expected cursor at 0 after 'gg', got %d", m.selector.cursor)
	}
}

func TestTaskSelect_ShiftGGoesToBottom(t *testing.T) {
	cfg := &config.Config{
		OneOff: make([]config.WorkflowConfig, 10),
	}
	items := make([]string, 10)
	for i := range cfg.OneOff {
		cfg.OneOff[i] = config.WorkflowConfig{Name: fmt.Sprintf("task-%d", i+1)}
		items[i] = cfg.OneOff[i].Name
	}
	m := Model{
		cfg:    cfg,
		keys:   newKeyMap(),
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
		selector: selector{
			kind:   selectorTask,
			cursor: 0,
			items:  items,
		},
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}}
	result, _ := m.handleSelectorKey(msg)
	updated := result.(Model)

	if updated.selector.cursor != 9 {
		t.Errorf("expected cursor at 9 after 'G', got %d", updated.selector.cursor)
	}
}

func TestTaskSelect_CtrlDPageDown(t *testing.T) {
	cfg := &config.Config{
		OneOff: make([]config.WorkflowConfig, 10),
	}
	items := make([]string, 10)
	for i := range cfg.OneOff {
		cfg.OneOff[i] = config.WorkflowConfig{Name: fmt.Sprintf("task-%d", i+1)}
		items[i] = cfg.OneOff[i].Name
	}
	m := Model{
		cfg:    cfg,
		keys:   newKeyMap(),
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
		selector: selector{
			kind:   selectorTask,
			cursor: 0,
			items:  items,
		},
	}

	msg := tea.KeyMsg{Type: tea.KeyCtrlD}
	result, _ := m.handleSelectorKey(msg)
	updated := result.(Model)

	// half = 10/2 = 5
	if updated.selector.cursor != 5 {
		t.Errorf("expected cursor at 5 after ctrl+d, got %d", updated.selector.cursor)
	}
}

func TestTaskSelect_CtrlUPageUp(t *testing.T) {
	cfg := &config.Config{
		OneOff: make([]config.WorkflowConfig, 10),
	}
	items := make([]string, 10)
	for i := range cfg.OneOff {
		cfg.OneOff[i] = config.WorkflowConfig{Name: fmt.Sprintf("task-%d", i+1)}
		items[i] = cfg.OneOff[i].Name
	}
	m := Model{
		cfg:    cfg,
		keys:   newKeyMap(),
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
		selector: selector{
			kind:   selectorTask,
			cursor: 8,
			items:  items,
		},
	}

	msg := tea.KeyMsg{Type: tea.KeyCtrlU}
	result, _ := m.handleSelectorKey(msg)
	updated := result.(Model)

	// half = 10/2 = 5
	if updated.selector.cursor != 3 {
		t.Errorf("expected cursor at 3 after ctrl+u, got %d", updated.selector.cursor)
	}
}

func TestArtifactSelect_GGGoesToTop(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
		selector: selector{
			kind:   selectorArtifact,
			cursor: 5,
			items:  []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8"},
		},
	}

	gMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}
	result, _ := m.handleSelectorKey(gMsg)
	m = result.(Model)
	result, _ = m.handleSelectorKey(gMsg)
	m = result.(Model)

	if m.selector.cursor != 0 {
		t.Errorf("expected cursor at 0 after 'gg', got %d", m.selector.cursor)
	}
}

func TestArtifactSelect_ShiftGGoesToBottom(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
		selector: selector{
			kind:   selectorArtifact,
			cursor: 0,
			items:  []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8"},
		},
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}}
	result, _ := m.handleSelectorKey(msg)
	updated := result.(Model)

	if updated.selector.cursor != 7 {
		t.Errorf("expected cursor at 7 after 'G', got %d", updated.selector.cursor)
	}
}

func TestArtifactSelect_CtrlDPageDown(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
		selector: selector{
			kind:   selectorArtifact,
			cursor: 0,
			items:  []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8"},
		},
	}

	msg := tea.KeyMsg{Type: tea.KeyCtrlD}
	result, _ := m.handleSelectorKey(msg)
	updated := result.(Model)

	// half = 8/2 = 4
	if updated.selector.cursor != 4 {
		t.Errorf("expected cursor at 4 after ctrl+d, got %d", updated.selector.cursor)
	}
}

func TestArtifactSelect_CtrlUPageUp(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
		selector: selector{
			kind:   selectorArtifact,
			cursor: 6,
			items:  []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8"},
		},
	}

	msg := tea.KeyMsg{Type: tea.KeyCtrlU}
	result, _ := m.handleSelectorKey(msg)
	updated := result.(Model)

	// half = 8/2 = 4
	if updated.selector.cursor != 2 {
		t.Errorf("expected cursor at 2 after ctrl+u, got %d", updated.selector.cursor)
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

	if updated.selector.kind != selectorPriority {
		t.Error("expected selector kind to be selectorPriority after 'p'")
	}
	if updated.selector.taskID != 20 {
		t.Errorf("expected selector taskID to be 20, got %d", updated.selector.taskID)
	}
}

func TestHandlePrioritySelectKey_EscCancels(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		client: &client.Client{},
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
		selector: selector{
			kind:   selectorPriority,
			items:  []string{"low", "medium", "high", "urgent"},
			taskID: 1,
		},
	}

	msg := tea.KeyMsg{Type: tea.KeyEsc}
	result, _ := m.handleSelectorKey(msg)
	updated := result.(Model)

	if updated.selector.kind == selectorPriority {
		t.Error("expected selector kind not to be selectorPriority after esc")
	}
}

func TestHandlePrioritySelectKey_Navigation(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		client: &client.Client{},
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
		selector: selector{
			kind:   selectorPriority,
			cursor: 0,
			items:  []string{"low", "medium", "high", "urgent"},
			taskID: 1,
		},
	}

	// Move down
	downMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	result, _ := m.handleSelectorKey(downMsg)
	updated := result.(Model)
	if updated.selector.cursor != 1 {
		t.Errorf("expected cursor at 1, got %d", updated.selector.cursor)
	}

	// Move up
	upMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}
	result, _ = updated.handleSelectorKey(upMsg)
	updated = result.(Model)
	if updated.selector.cursor != 0 {
		t.Errorf("expected cursor at 0, got %d", updated.selector.cursor)
	}
}

func TestHandleArtifactSelectKey_Navigation(t *testing.T) {
	m := Model{
		keys: newKeyMap(),
		selector: selector{
			kind:   selectorArtifact,
			cursor: 0,
			items:  []string{"implement", "review", "test"},
			action: "view",
		},
		taskSteps: []daemon.TaskStepDetail{
			{Name: "implement", Status: stepStatusCompleted, Context: "implement content"},
			{Name: "review", Status: stepStatusCompleted, Context: "review content"},
			{Name: "test", Status: stepStatusCompleted, Context: "test content"},
		},
	}

	// Move down
	jMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	result, _ := m.handleSelectorKey(jMsg)
	updated := result.(Model)
	if updated.selector.cursor != 1 {
		t.Errorf("expected cursor at 1, got %d", updated.selector.cursor)
	}

	// Move down again
	result, _ = updated.handleSelectorKey(jMsg)
	updated = result.(Model)
	if updated.selector.cursor != 2 {
		t.Errorf("expected cursor at 2, got %d", updated.selector.cursor)
	}

	// Move down at bottom — should stay
	result, _ = updated.handleSelectorKey(jMsg)
	updated = result.(Model)
	if updated.selector.cursor != 2 {
		t.Errorf("expected cursor to stay at 2, got %d", updated.selector.cursor)
	}

	// Move up
	kMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}
	result, _ = updated.handleSelectorKey(kMsg)
	updated = result.(Model)
	if updated.selector.cursor != 1 {
		t.Errorf("expected cursor at 1, got %d", updated.selector.cursor)
	}
}

func TestHandleArtifactSelectKey_EscCancels(t *testing.T) {
	m := Model{
		keys: newKeyMap(),
		selector: selector{
			kind:   selectorArtifact,
			cursor: 1,
			items:  []string{"implement", "review"},
			action: "view",
		},
	}

	msg := tea.KeyMsg{Type: tea.KeyEscape}
	result, _ := m.handleSelectorKey(msg)
	updated := result.(Model)

	if updated.selector.kind == selectorArtifact {
		t.Error("expected selector kind not to be selectorArtifact after esc")
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

func TestViewRendersTaskSelection_HidesUnlisted(t *testing.T) {
	// Since unlisted is removed, all one-off workflows are now visible
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
		selector: selector{
			kind:   selectorTask,
			cursor: 0,
			items:  []string{"Visible", "Hidden"},
			title:  "Run Predefined Task",
		},
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

func TestViewRendersStepContextSelection(t *testing.T) {
	m := Model{
		keys:   newKeyMap(),
		list:   newListView(false, ""),
		detail: newDetailView(),
		view:   viewList,
		selector: selector{
			kind:   selectorArtifact,
			cursor: 0,
			items:  []string{"implement", "review"},
			title:  "Select Step Context",
			action: "view",
		},
	}

	output := m.View()

	if !strings.Contains(output, "Select Step Context") {
		t.Error("expected selection screen to contain 'Select Step Context' title")
	}
	if !strings.Contains(output, "implement") {
		t.Error("expected selection to contain 'implement'")
	}
	if !strings.Contains(output, "review") {
		t.Error("expected selection to contain 'review'")
	}
}

func TestArtifactViewState_View(t *testing.T) {
	v := &artifactViewState{}
	v.SetSize(80, 24)
	v.SetContent("implement", "This is the step context content.\nLine 2.")

	output := v.View()

	if !strings.Contains(output, "Step Context: implement") {
		t.Error("expected view to contain 'Step Context: implement'")
	}
	if !strings.Contains(output, "step context content") {
		t.Error("expected view to contain step context content")
	}
	if !strings.Contains(output, "esc/q") {
		t.Error("expected view help to contain 'esc/q'")
	}
}

func TestTaskStepsLoadedMsg_SwitchesToArtifactView(t *testing.T) {
	m := Model{
		keys: newKeyMap(),
		list: newListView(false, ""),
		view: viewList,
	}
	m.artifactView.SetSize(80, 24)

	now := time.Now()
	msg := taskStepsLoadedMsg{
		taskID: 1,
		steps: []daemon.TaskStepDetail{
			{Name: "implement", Status: stepStatusCompleted, Context: "test content", CompletedAt: &now},
		},
		action: "view",
	}
	result, _ := m.Update(msg)
	updated := result.(Model)

	if updated.view != viewArtifact {
		t.Errorf("expected view to be viewArtifact (%d), got %d", viewArtifact, updated.view)
	}
}

func TestTaskStepsLoadedMsg_SingleStepShowsDirectly(t *testing.T) {
	// When only one actionable step is returned, it should be shown directly without selection
	m := Model{
		keys: newKeyMap(),
		list: newListView(false, ""),
		view: viewList,
	}
	m.artifactView.SetSize(80, 24)

	now := time.Now()
	msg := taskStepsLoadedMsg{
		taskID: 1,
		steps: []daemon.TaskStepDetail{
			{Name: "implement", Status: stepStatusCompleted, Context: "step output", CompletedAt: &now},
			{Name: "review", Status: stepStatusPending},
		},
		action: "view",
	}
	result, _ := m.Update(msg)
	updated := result.(Model)

	if updated.selector.kind == selectorArtifact && len(updated.selector.items) > 0 {
		t.Error("expected selector to be empty for single actionable step")
	}
	if updated.view != viewArtifact {
		t.Errorf("expected view to be viewArtifact, got %d", updated.view)
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
