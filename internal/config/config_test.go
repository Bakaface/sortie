package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStepConfigArtifactParsing(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".sortie.yml")

	yaml := `
workflow:
  steps:
    - name: implement
      prompt: "Implement the task"
      artifact: true
    - name: review
      prompt: "Review the implementation"
    - name: test
      prompt: "Write tests"
      artifact: false
`
	if err := os.WriteFile(configPath, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := defaultConfig()
	if err := loadProjectConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}

	steps := cfg.Workflows[0].Steps
	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(steps))
	}

	if !steps[0].Artifact {
		t.Error("expected implement step to have artifact: true")
	}
	if steps[1].Artifact {
		t.Error("expected review step to have artifact: false (default)")
	}
	if steps[2].Artifact {
		t.Error("expected test step to have artifact: false (explicit)")
	}
}

func TestDefaultWorkflowArtifactDefault(t *testing.T) {
	wf := DefaultWorkflow()
	if wf.Steps[0].Artifact {
		t.Error("expected default workflow step to have artifact: false")
	}
}

func TestPredefinedTasksParsing(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".sortie.yml")

	yamlContent := `
tasks:
  - name: "Refactor: Housekeeping"
    description: "Standard codebase maintenance"
    steps:
      - name: audit
        prompt: "Identify code smells"
        artifact: true
      - name: refactor
        prompt: "Apply refactoring based on audit"
        human: true
  - name: "Security Scan"
    description: "Run security audit"
    steps:
      - name: scan
        prompt: "Scan for vulnerabilities"
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := defaultConfig()
	if err := loadProjectConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}

	// Check predefined tasks were parsed into OneOff
	if len(cfg.OneOff) != 2 {
		t.Fatalf("expected 2 one-off workflows, got %d", len(cfg.OneOff))
	}

	if cfg.OneOff[0].Name != "Refactor: Housekeeping" {
		t.Errorf("expected first workflow name 'Refactor: Housekeeping', got %q", cfg.OneOff[0].Name)
	}
	if cfg.OneOff[0].Description != "Standard codebase maintenance" {
		t.Errorf("expected first workflow description 'Standard codebase maintenance', got %q", cfg.OneOff[0].Description)
	}
	if len(cfg.OneOff[0].Steps) != 2 {
		t.Fatalf("expected 2 steps in first workflow, got %d", len(cfg.OneOff[0].Steps))
	}
	if !cfg.OneOff[0].Steps[0].Artifact {
		t.Error("expected audit step to have artifact: true")
	}
	if !cfg.OneOff[0].Steps[1].Human {
		t.Error("expected refactor step to have human: true")
	}

	// Check synthetic workflows were registered with oneoff: prefix
	wf := cfg.GetWorkflow("oneoff:Refactor: Housekeeping")
	if wf.Name != "oneoff:Refactor: Housekeeping" {
		t.Errorf("expected synthetic workflow name 'oneoff:Refactor: Housekeeping', got %q", wf.Name)
	}
	if len(wf.Steps) != 2 {
		t.Fatalf("expected 2 steps in synthetic workflow, got %d", len(wf.Steps))
	}

	wf2 := cfg.GetWorkflow("oneoff:Security Scan")
	if wf2.Name != "oneoff:Security Scan" {
		t.Errorf("expected synthetic workflow name 'oneoff:Security Scan', got %q", wf2.Name)
	}
}

func TestPredefinedTasksAutoNaming(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".sortie.yml")

	yamlContent := `
tasks:
  - description: "Task without name"
    steps:
      - name: do
        prompt: "Do something"
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := defaultConfig()
	if err := loadProjectConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}

	if cfg.OneOff[0].Name != "task-1" {
		t.Errorf("expected auto-named workflow 'task-1', got %q", cfg.OneOff[0].Name)
	}

	// Should be resolvable as workflow with oneoff: prefix
	wf := cfg.GetWorkflow("oneoff:task-1")
	if wf.Name != "oneoff:task-1" {
		t.Errorf("expected synthetic workflow 'oneoff:task-1', got %q", wf.Name)
	}
}

func TestListPredefinedTaskNames(t *testing.T) {
	cfg := &Config{
		OneOff: []WorkflowConfig{
			{Name: "task-a"},
			{Name: "task-b"},
		},
	}

	names := cfg.ListPredefinedTaskNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
	if names[0] != "task-a" {
		t.Errorf("expected first name 'task-a', got %q", names[0])
	}
	if names[1] != "task-b" {
		t.Errorf("expected second name 'task-b', got %q", names[1])
	}
}

func TestListPredefinedTaskNamesEmpty(t *testing.T) {
	cfg := &Config{}
	names := cfg.ListPredefinedTaskNames()
	if len(names) != 0 {
		t.Errorf("expected 0 names for empty config, got %d", len(names))
	}
}

func TestListWorkflowNamesExcludesSyntheticTasks(t *testing.T) {
	cfg := &Config{
		TaskWorkflows: []WorkflowConfig{
			{Name: "default"},
			{Name: "review"},
		},
		OneOff: []WorkflowConfig{
			{Name: "Housekeeping"},
			{Name: "Security"},
		},
		Workflows: []WorkflowConfig{
			{Name: "default"},
			{Name: "review"},
			{Name: "oneoff:Housekeeping"},
			{Name: "oneoff:Security"},
		},
	}

	names := cfg.ListWorkflowNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 workflow names (from TaskWorkflows only), got %d: %v", len(names), names)
	}
	if names[0] != "default" {
		t.Errorf("expected first name 'default', got %q", names[0])
	}
	if names[1] != "review" {
		t.Errorf("expected second name 'review', got %q", names[1])
	}
}

func TestListWorkflowNamesOnlySyntheticTasksReturnsDefault(t *testing.T) {
	cfg := &Config{
		TaskWorkflows: []WorkflowConfig{}, // Empty TaskWorkflows
		Workflows: []WorkflowConfig{
			{Name: "oneoff:Housekeeping"},
			{Name: "oneoff:Security"},
		},
	}

	names := cfg.ListWorkflowNames()
	if len(names) != 1 || names[0] != "default" {
		t.Errorf("expected [\"default\"] when TaskWorkflows is empty, got %v", names)
	}
}

func TestGetPredefinedTask(t *testing.T) {
	cfg := &Config{
		OneOff: []WorkflowConfig{
			{Name: "task-a", Description: "desc-a"},
			{Name: "task-b", Description: "desc-b"},
		},
	}

	task := cfg.GetPredefinedTask("task-b")
	if task == nil {
		t.Fatal("expected task-b to be found")
	}
	if task.Description != "desc-b" {
		t.Errorf("expected description 'desc-b', got %q", task.Description)
	}

	if cfg.GetPredefinedTask("nonexistent") != nil {
		t.Error("expected nil for nonexistent task")
	}
}

func TestListPredefinedTaskNamesExcludesUnlisted(t *testing.T) {
	// Unlisted concept removed - all one-off workflows are returned
	cfg := &Config{
		OneOff: []WorkflowConfig{
			{Name: "task-a"},
			{Name: "task-b"},
			{Name: "task-c"},
		},
	}

	names := cfg.ListPredefinedTaskNames()
	if len(names) != 3 {
		t.Fatalf("expected 3 names (unlisted removed), got %d: %v", len(names), names)
	}
	if names[0] != "task-a" {
		t.Errorf("expected first name 'task-a', got %q", names[0])
	}
	if names[1] != "task-b" {
		t.Errorf("expected second name 'task-b', got %q", names[1])
	}
	if names[2] != "task-c" {
		t.Errorf("expected third name 'task-c', got %q", names[2])
	}
}

func TestListAllPredefinedTaskNamesIncludesUnlisted(t *testing.T) {
	// ListAllPredefinedTaskNames now same as ListPredefinedTaskNames (unlisted removed)
	cfg := &Config{
		OneOff: []WorkflowConfig{
			{Name: "task-a"},
			{Name: "task-b"},
			{Name: "task-c"},
		},
	}

	names := cfg.ListAllPredefinedTaskNames()
	if len(names) != 3 {
		t.Fatalf("expected 3 names, got %d: %v", len(names), names)
	}
	if names[1] != "task-b" {
		t.Errorf("expected second name 'task-b', got %q", names[1])
	}
}

func TestYoloDefaultFalse(t *testing.T) {
	cfg := defaultConfig()
	if cfg.Claude.Yolo {
		t.Error("expected yolo to default to false")
	}
}

func TestClaudeConfigArgsWithoutYolo(t *testing.T) {
	cfg := &ClaudeConfig{
		Command:     "claude",
		DefaultArgs: []string{"--verbose"},
		Yolo:        false,
	}
	args := cfg.Args()
	for _, a := range args {
		if a == "--dangerously-skip-permissions" {
			t.Error("expected --dangerously-skip-permissions to NOT be in args when yolo is false")
		}
	}
	if len(args) != 1 || args[0] != "--verbose" {
		t.Errorf("expected [--verbose], got %v", args)
	}
}

func TestClaudeConfigArgsWithYolo(t *testing.T) {
	cfg := &ClaudeConfig{
		Command: "claude",
		Yolo:    true,
	}
	args := cfg.Args()
	found := false
	for _, a := range args {
		if a == "--dangerously-skip-permissions" {
			found = true
		}
	}
	if !found {
		t.Error("expected --dangerously-skip-permissions in args when yolo is true")
	}
}

func TestYoloProjectConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".sortie.yml")

	yamlContent := `
yolo: true
workflow:
  steps:
    - name: implement
      prompt: "Implement the task"
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := defaultConfig()
	if err := loadProjectConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}

	if !cfg.Claude.Yolo {
		t.Error("expected yolo to be true from project config")
	}
}

func TestYoloGlobalConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	yamlContent := `
yolo: true
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := defaultConfig()
	if err := loadGlobalConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}

	if !cfg.Claude.Yolo {
		t.Error("expected yolo to be true from global config")
	}
}

func TestYoloProjectOverridesGlobal(t *testing.T) {
	// Global sets yolo: true, project sets yolo: false
	globalDir := t.TempDir()
	globalPath := filepath.Join(globalDir, "config.yaml")
	os.WriteFile(globalPath, []byte("yolo: true\n"), 0644)

	projectDir := t.TempDir()
	projectPath := filepath.Join(projectDir, ".sortie.yml")
	os.WriteFile(projectPath, []byte("yolo: false\n"), 0644)

	cfg := defaultConfig()
	if err := loadGlobalConfig(globalPath, cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.Claude.Yolo {
		t.Error("expected yolo to be true after loading global config")
	}

	if err := loadProjectConfig(projectPath, cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Claude.Yolo {
		t.Error("expected yolo to be false after project config overrides global")
	}
}

func TestValidateArtifactDefaultFalse(t *testing.T) {
	cfg := defaultConfig()
	if cfg.ValidateArtifact {
		t.Error("expected validate_artifact to default to false")
	}
}

func TestValidateArtifactProjectConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".sortie.yml")

	yamlContent := `
validate_artifact: true
workflow:
  steps:
    - name: implement
      prompt: "Implement the task"
      artifact: true
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := defaultConfig()
	if err := loadProjectConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}

	if !cfg.ValidateArtifact {
		t.Error("expected validate_artifact to be true from project config")
	}
}

func TestValidateArtifactGlobalConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	yamlContent := `
validate_artifact: true
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := defaultConfig()
	if err := loadGlobalConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}

	if !cfg.ValidateArtifact {
		t.Error("expected validate_artifact to be true from global config")
	}
}

func TestValidateArtifactProjectOverridesGlobal(t *testing.T) {
	// Global sets validate_artifact: true, project sets validate_artifact: false
	globalDir := t.TempDir()
	globalPath := filepath.Join(globalDir, "config.yaml")
	os.WriteFile(globalPath, []byte("validate_artifact: true\n"), 0644)

	projectDir := t.TempDir()
	projectPath := filepath.Join(projectDir, ".sortie.yml")
	os.WriteFile(projectPath, []byte("validate_artifact: false\n"), 0644)

	cfg := defaultConfig()
	if err := loadGlobalConfig(globalPath, cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.ValidateArtifact {
		t.Error("expected validate_artifact to be true after loading global config")
	}

	if err := loadProjectConfig(projectPath, cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.ValidateArtifact {
		t.Error("expected validate_artifact to be false after project config overrides global")
	}
}

func TestLoopConfigParsing(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".sortie.yml")

	yamlContent := `
workflow:
  steps:
    - name: plan
      prompt: "Plan"
      artifact: true
    - name: implement
      prompt: "Implement"
    - name: verify
      prompt: "Verify"
      loop:
        goto: plan
        max_iterations: 5
        exit_condition:
          artifact_empty: plan
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := defaultConfig()
	if err := loadProjectConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}

	steps := cfg.Workflows[0].Steps
	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(steps))
	}

	// Check first two steps have no loop
	if steps[0].Loop != nil {
		t.Error("expected plan step to have no loop")
	}
	if steps[1].Loop != nil {
		t.Error("expected implement step to have no loop")
	}

	// Check third step has loop config
	if steps[2].Loop == nil {
		t.Fatal("expected verify step to have loop config")
	}

	loop := steps[2].Loop
	if loop.Goto != "plan" {
		t.Errorf("expected goto 'plan', got %q", loop.Goto)
	}
	if loop.MaxIterations != 5 {
		t.Errorf("expected max_iterations 5, got %d", loop.MaxIterations)
	}

	// Check exit condition
	if loop.ExitCondition == nil {
		t.Fatal("expected exit_condition to be set")
	}
	if loop.ExitCondition.ArtifactEmpty != "plan" {
		t.Errorf("expected artifact_empty 'plan', got %q", loop.ExitCondition.ArtifactEmpty)
	}
}

func TestValidateLoopsValidConfig(t *testing.T) {
	wf := &WorkflowConfig{
		Name: "test",
		Steps: []StepConfig{
			{Name: "plan", Prompt: "Plan", Artifact: true},
			{Name: "implement", Prompt: "Implement"},
			{
				Name:   "verify",
				Prompt: "Verify",
				Loop: &LoopConfig{
					Goto:          "plan",
					MaxIterations: 3,
					ExitCondition: &LoopExitCondition{
						ArtifactEmpty: "plan",
					},
				},
			},
		},
	}

	if err := wf.ValidateLoops(); err != nil {
		t.Errorf("expected valid config to pass validation, got error: %v", err)
	}
}

func TestValidateLoopsInvalidGoto(t *testing.T) {
	wf := &WorkflowConfig{
		Name: "test",
		Steps: []StepConfig{
			{Name: "plan", Prompt: "Plan"},
			{
				Name:   "implement",
				Prompt: "Implement",
				Loop: &LoopConfig{
					Goto:          "nonexistent",
					MaxIterations: 3,
				},
			},
		},
	}

	err := wf.ValidateLoops()
	if err == nil {
		t.Error("expected error for goto referencing unknown step")
	}
	if err != nil && !containsString(err.Error(), "unknown step") {
		t.Errorf("expected error about unknown step, got: %v", err)
	}
}

func TestValidateLoopsForwardGoto(t *testing.T) {
	wf := &WorkflowConfig{
		Name: "test",
		Steps: []StepConfig{
			{
				Name:   "plan",
				Prompt: "Plan",
				Loop: &LoopConfig{
					Goto:          "implement",
					MaxIterations: 3,
				},
			},
			{Name: "implement", Prompt: "Implement"},
		},
	}

	err := wf.ValidateLoops()
	if err == nil {
		t.Error("expected error for forward goto")
	}
	if err != nil && !containsString(err.Error(), "earlier step") {
		t.Errorf("expected error about earlier step, got: %v", err)
	}
}

func TestValidateLoopsSelfReference(t *testing.T) {
	wf := &WorkflowConfig{
		Name: "test",
		Steps: []StepConfig{
			{
				Name:   "plan",
				Prompt: "Plan",
				Loop: &LoopConfig{
					Goto:          "plan",
					MaxIterations: 3,
				},
			},
		},
	}

	err := wf.ValidateLoops()
	if err == nil {
		t.Error("expected error for self-reference")
	}
	if err != nil && !containsString(err.Error(), "earlier step") {
		t.Errorf("expected error about earlier step, got: %v", err)
	}
}

func TestValidateLoopsMaxIterationsZero(t *testing.T) {
	wf := &WorkflowConfig{
		Name: "test",
		Steps: []StepConfig{
			{Name: "plan", Prompt: "Plan"},
			{
				Name:   "implement",
				Prompt: "Implement",
				Loop: &LoopConfig{
					Goto:          "plan",
					MaxIterations: 0,
				},
			},
		},
	}

	err := wf.ValidateLoops()
	if err == nil {
		t.Error("expected error for max_iterations 0")
	}
	if err != nil && !containsString(err.Error(), "must be >= 1") {
		t.Errorf("expected error about max_iterations >= 1, got: %v", err)
	}
}

func TestValidateLoopsHumanStep(t *testing.T) {
	wf := &WorkflowConfig{
		Name: "test",
		Steps: []StepConfig{
			{Name: "plan", Prompt: "Plan"},
			{
				Name:   "implement",
				Prompt: "Implement",
				Human:  true,
				Loop: &LoopConfig{
					Goto:          "plan",
					MaxIterations: 3,
				},
			},
		},
	}

	err := wf.ValidateLoops()
	if err == nil {
		t.Error("expected error for human step with loop")
	}
	if err != nil && !containsString(err.Error(), "human: true") {
		t.Errorf("expected error about human: true, got: %v", err)
	}
}

func TestValidateLoopsInvalidExitCondition(t *testing.T) {
	wf := &WorkflowConfig{
		Name: "test",
		Steps: []StepConfig{
			{Name: "plan", Prompt: "Plan"},
			{
				Name:   "implement",
				Prompt: "Implement",
				Loop: &LoopConfig{
					Goto:          "plan",
					MaxIterations: 3,
					ExitCondition: &LoopExitCondition{
						ArtifactEmpty: "nonexistent",
					},
				},
			},
		},
	}

	err := wf.ValidateLoops()
	if err == nil {
		t.Error("expected error for exit condition referencing unknown step")
	}
	if err != nil && !containsString(err.Error(), "unknown step") {
		t.Errorf("expected error about unknown step, got: %v", err)
	}
}

func TestValidateLoopsOverlapping(t *testing.T) {
	wf := &WorkflowConfig{
		Name: "test",
		Steps: []StepConfig{
			{Name: "step1", Prompt: "Step 1"},
			{Name: "step2", Prompt: "Step 2"},
			{Name: "step3", Prompt: "Step 3"},
			{
				Name:   "step4",
				Prompt: "Step 4",
				Loop: &LoopConfig{
					Goto:          "step1",
					MaxIterations: 2,
				},
			},
			{
				Name:   "step5",
				Prompt: "Step 5",
				Loop: &LoopConfig{
					Goto:          "step2",
					MaxIterations: 2,
				},
			},
		},
	}

	err := wf.ValidateLoops()
	if err == nil {
		t.Error("expected error for overlapping loops")
	}
	if err != nil && !containsString(err.Error(), "overlaps") {
		t.Errorf("expected error about overlapping loops, got: %v", err)
	}
}

func TestValidateLoopsNoLoop(t *testing.T) {
	wf := &WorkflowConfig{
		Name: "test",
		Steps: []StepConfig{
			{Name: "plan", Prompt: "Plan"},
			{Name: "implement", Prompt: "Implement"},
			{Name: "verify", Prompt: "Verify"},
		},
	}

	if err := wf.ValidateLoops(); err != nil {
		t.Errorf("expected workflow without loops to pass validation, got error: %v", err)
	}
}

func TestResolveBranchTemplate(t *testing.T) {
	tests := []struct {
		name     string
		tmpl     string
		taskID   int64
		title    string
		slug     string
		expected string
	}{
		{
			name:     "config-style placeholders",
			tmpl:     "sortie/{{task_id}}-{{task_slug}}",
			taskID:   42,
			title:    "Add Login Page",
			slug:     "add-login-page",
			expected: "sortie/42-add-login-page",
		},
		{
			name:     "workflow-style placeholders",
			tmpl:     "feature/{{task.id}}-{{task.title}}",
			taskID:   7,
			title:    "Fix Bug",
			slug:     "fix-bug",
			expected: "feature/7-Fix Bug",
		},
		{
			name:     "mixed placeholders",
			tmpl:     "{{task_id}}/{{task.title}}/{{task.slug}}",
			taskID:   99,
			title:    "Refactor Auth",
			slug:     "refactor-auth",
			expected: "99/Refactor Auth/refactor-auth",
		},
		{
			name:     "no placeholders",
			tmpl:     "feature/my-branch",
			taskID:   1,
			title:    "Title",
			slug:     "title",
			expected: "feature/my-branch",
		},
		{
			name:     "task.slug placeholder",
			tmpl:     "feature/TASK-001-{{task.slug}}",
			taskID:   1,
			title:    "Add feature",
			slug:     "add-feature",
			expected: "feature/TASK-001-add-feature",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveBranchTemplate(tt.tmpl, tt.taskID, tt.title, tt.slug)
			if result != tt.expected {
				t.Errorf("ResolveBranchTemplate(%q) = %q, want %q", tt.tmpl, result, tt.expected)
			}
		})
	}
}

// TestHierarchicalConfigMerging tests the 3-layer config loading:
// defaults -> global .sortie.yml -> project .sortie.yml
func TestHierarchicalConfigMerging(t *testing.T) {
	globalDir := t.TempDir()
	globalSortieYml := filepath.Join(globalDir, ".sortie.yml")
	os.WriteFile(globalSortieYml, []byte(`
max_workers: 5
notifications:
  enabled: true
  on_complete: true
  on_failed: false
git:
  base_branch: develop
  on_complete: merge
workflow:
  steps:
    - name: global-step
      prompt: "Global default step"
`), 0644)

	projectDir := t.TempDir()
	projectSortieYml := filepath.Join(projectDir, ".sortie.yml")
	os.WriteFile(projectSortieYml, []byte(`
max_workers: 2
workflow:
  steps:
    - name: implement
      prompt: "Implement {{task.description}}"
`), 0644)

	// Simulate the 3-layer loading: defaults -> global .sortie.yml -> project .sortie.yml
	cfg := defaultConfig()

	// Layer 2: global .sortie.yml
	if err := loadProjectConfig(globalSortieYml, cfg); err != nil {
		t.Fatalf("failed to load global .sortie.yml: %v", err)
	}

	// After global .sortie.yml: max_workers=5, notifications set, git set, workflow set
	if cfg.MaxWorkers != 5 {
		t.Errorf("after global .sortie.yml: expected max_workers=5, got %d", cfg.MaxWorkers)
	}
	if !cfg.Notifications.Enabled {
		t.Error("after global .sortie.yml: expected notifications.enabled=true")
	}
	if !cfg.Notifications.OnComplete {
		t.Error("after global .sortie.yml: expected notifications.on_complete=true")
	}
	if cfg.Notifications.OnFailed {
		t.Error("after global .sortie.yml: expected notifications.on_failed=false")
	}
	if cfg.Git.BaseBranch != "develop" {
		t.Errorf("after global .sortie.yml: expected git.base_branch=develop, got %q", cfg.Git.BaseBranch)
	}
	if cfg.Git.OnComplete != "merge" {
		t.Errorf("after global .sortie.yml: expected git.on_complete=merge, got %q", cfg.Git.OnComplete)
	}
	if len(cfg.Workflows) != 1 || cfg.Workflows[0].Steps[0].Name != "global-step" {
		t.Error("after global .sortie.yml: expected global-step workflow")
	}

	// Layer 3: project .sortie.yml overrides
	if err := loadProjectConfig(projectSortieYml, cfg); err != nil {
		t.Fatalf("failed to load project .sortie.yml: %v", err)
	}

	// max_workers overridden by project
	if cfg.MaxWorkers != 2 {
		t.Errorf("after project override: expected max_workers=2, got %d", cfg.MaxWorkers)
	}
	// notifications should remain from global (project didn't set them)
	if !cfg.Notifications.Enabled {
		t.Error("after project override: expected notifications.enabled=true (from global)")
	}
	if !cfg.Notifications.OnComplete {
		t.Error("after project override: expected notifications.on_complete=true (from global)")
	}
	// git.base_branch should remain from global (project didn't set it)
	if cfg.Git.BaseBranch != "develop" {
		t.Errorf("after project override: expected git.base_branch=develop (from global), got %q", cfg.Git.BaseBranch)
	}
	// workflow should be overridden by project
	if len(cfg.Workflows) != 1 || cfg.Workflows[0].Steps[0].Name != "implement" {
		t.Error("after project override: expected implement workflow from project")
	}
}

func TestHierarchicalConfigProjectOverridesNotifications(t *testing.T) {
	globalDir := t.TempDir()
	globalSortieYml := filepath.Join(globalDir, ".sortie.yml")
	os.WriteFile(globalSortieYml, []byte(`
notifications:
  enabled: true
  on_complete: true
`), 0644)

	projectDir := t.TempDir()
	projectSortieYml := filepath.Join(projectDir, ".sortie.yml")
	os.WriteFile(projectSortieYml, []byte(`
notifications:
  enabled: false
`), 0644)

	cfg := defaultConfig()

	if err := loadProjectConfig(globalSortieYml, cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.Notifications.Enabled {
		t.Error("expected notifications.enabled=true from global")
	}

	if err := loadProjectConfig(projectSortieYml, cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Notifications.Enabled {
		t.Error("expected notifications.enabled=false after project override")
	}
}

func TestHierarchicalConfigGlobalOnlyNoProject(t *testing.T) {
	globalDir := t.TempDir()
	globalSortieYml := filepath.Join(globalDir, ".sortie.yml")
	os.WriteFile(globalSortieYml, []byte(`
max_workers: 7
git:
  base_branch: develop
  branch_template: "feature/{{task_id}}"
workflow:
  steps:
    - name: plan
      prompt: "Plan the task"
`), 0644)

	cfg := defaultConfig()

	if err := loadProjectConfig(globalSortieYml, cfg); err != nil {
		t.Fatal(err)
	}

	// All values from global .sortie.yml
	if cfg.MaxWorkers != 7 {
		t.Errorf("expected max_workers=7, got %d", cfg.MaxWorkers)
	}
	if cfg.Git.BaseBranch != "develop" {
		t.Errorf("expected git.base_branch=develop, got %q", cfg.Git.BaseBranch)
	}
	if cfg.Git.BranchTemplate != "feature/{{task_id}}" {
		t.Errorf("expected branch_template=feature/{{task_id}}, got %q", cfg.Git.BranchTemplate)
	}
	if len(cfg.Workflows) != 1 || cfg.Workflows[0].Steps[0].Name != "plan" {
		t.Error("expected plan workflow")
	}
}

func TestHierarchicalConfigTmuxNestedAttachBehavior(t *testing.T) {
	globalDir := t.TempDir()
	globalSortieYml := filepath.Join(globalDir, ".sortie.yml")
	os.WriteFile(globalSortieYml, []byte(`
tmux_nested_attach_behavior: nest
`), 0644)

	cfg := defaultConfig()

	if err := loadProjectConfig(globalSortieYml, cfg); err != nil {
		t.Fatal(err)
	}

	if cfg.TmuxNestedAttachBehavior != "nest" {
		t.Errorf("expected tmux_nested_attach_behavior=nest, got %q", cfg.TmuxNestedAttachBehavior)
	}
}

func TestHierarchicalConfigAllThreeLayers(t *testing.T) {
	// Layer 1: global config.yaml (sets notifications)
	globalConfigDir := t.TempDir()
	globalConfigPath := filepath.Join(globalConfigDir, "config.yaml")
	os.WriteFile(globalConfigPath, []byte(`
max_workers: 10
notifications:
  enabled: true
  on_complete: true
  on_failed: true
  on_waiting_input: true
tmux_nested_attach_behavior: switch
`), 0644)

	// Layer 2: global .sortie.yml (sets workflow defaults, overrides max_workers)
	globalSortieDir := t.TempDir()
	globalSortieYml := filepath.Join(globalSortieDir, ".sortie.yml")
	os.WriteFile(globalSortieYml, []byte(`
max_workers: 5
git:
  base_branch: develop
notifications:
  enabled: true
  on_complete: false
workflow:
  steps:
    - name: default-step
      prompt: "Default implementation"
`), 0644)

	// Layer 3: project .sortie.yml (overrides max_workers, adds project workflow)
	projectDir := t.TempDir()
	projectSortieYml := filepath.Join(projectDir, ".sortie.yml")
	os.WriteFile(projectSortieYml, []byte(`
max_workers: 2
workflow:
  steps:
    - name: implement
      prompt: "Implement the task"
`), 0644)

	cfg := defaultConfig()

	// Apply all 3 layers in order
	if err := loadGlobalConfig(globalConfigPath, cfg); err != nil {
		t.Fatalf("layer 1: %v", err)
	}
	if err := loadProjectConfig(globalSortieYml, cfg); err != nil {
		t.Fatalf("layer 2: %v", err)
	}
	if err := loadProjectConfig(projectSortieYml, cfg); err != nil {
		t.Fatalf("layer 3: %v", err)
	}

	// max_workers: project (2) overrides global .sortie.yml (5) overrides config.yaml (10)
	if cfg.MaxWorkers != 2 {
		t.Errorf("expected max_workers=2, got %d", cfg.MaxWorkers)
	}
	// notifications: global .sortie.yml overrides config.yaml
	if !cfg.Notifications.Enabled {
		t.Error("expected notifications.enabled=true")
	}
	if cfg.Notifications.OnComplete {
		t.Error("expected notifications.on_complete=false (from global .sortie.yml)")
	}
	// tmux_nested_attach_behavior: from config.yaml (not overridden)
	if cfg.TmuxNestedAttachBehavior != "switch" {
		t.Errorf("expected tmux_nested_attach_behavior=switch, got %q", cfg.TmuxNestedAttachBehavior)
	}
	// git.base_branch: from global .sortie.yml (not overridden by project)
	if cfg.Git.BaseBranch != "develop" {
		t.Errorf("expected git.base_branch=develop, got %q", cfg.Git.BaseBranch)
	}
	// workflow: from project (overrides global .sortie.yml)
	if len(cfg.Workflows) != 1 || cfg.Workflows[0].Steps[0].Name != "implement" {
		t.Error("expected implement workflow from project")
	}
}

func TestVerificationConfigParsing(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".sortie.yml")

	yamlContent := `
verification:
  artifact_retry: true
  max_retries: 3
  verify_summarizer: true
workflow:
  steps:
    - name: implement
      prompt: "Implement"
      artifact: true
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := defaultConfig()
	if err := loadProjectConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}

	if !cfg.Verification.ArtifactRetry {
		t.Error("expected artifact_retry to be true")
	}
	if cfg.Verification.MaxRetries != 3 {
		t.Errorf("expected max_retries 3, got %d", cfg.Verification.MaxRetries)
	}
	if !cfg.Verification.VerifySummarizer {
		t.Error("expected verify_summarizer to be true")
	}
}

func TestVerificationConfigDefaultMaxRetries(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".sortie.yml")

	// artifact_retry: true but no max_retries specified → should default to 1
	yamlContent := `
verification:
  artifact_retry: true
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := defaultConfig()
	if err := loadProjectConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}

	if !cfg.Verification.ArtifactRetry {
		t.Error("expected artifact_retry to be true")
	}
	if cfg.Verification.MaxRetries != 1 {
		t.Errorf("expected max_retries to default to 1 when artifact_retry is true, got %d", cfg.Verification.MaxRetries)
	}
}

func TestVerificationConfigProjectOverridesGlobal(t *testing.T) {
	globalDir := t.TempDir()
	globalPath := filepath.Join(globalDir, "config.yaml")
	os.WriteFile(globalPath, []byte(`
verification:
  artifact_retry: true
  max_retries: 2
  verify_summarizer: true
`), 0644)

	projectDir := t.TempDir()
	projectPath := filepath.Join(projectDir, ".sortie.yml")
	os.WriteFile(projectPath, []byte(`
verification:
  artifact_retry: false
`), 0644)

	cfg := defaultConfig()
	if err := loadGlobalConfig(globalPath, cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.Verification.ArtifactRetry {
		t.Error("expected artifact_retry true from global config")
	}
	if cfg.Verification.MaxRetries != 2 {
		t.Errorf("expected max_retries 2 from global config, got %d", cfg.Verification.MaxRetries)
	}

	if err := loadProjectConfig(projectPath, cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Verification.ArtifactRetry {
		t.Error("expected artifact_retry to be false after project override")
	}
}

func TestVerificationConfigDefaultFalse(t *testing.T) {
	cfg := defaultConfig()
	if cfg.Verification.ArtifactRetry {
		t.Error("expected artifact_retry to default to false")
	}
	if cfg.Verification.MaxRetries != 0 {
		t.Errorf("expected max_retries to default to 0, got %d", cfg.Verification.MaxRetries)
	}
	if cfg.Verification.VerifySummarizer {
		t.Error("expected verify_summarizer to default to false")
	}
}

func TestNewWorkflowsStructureParsing(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".sortie.yml")

	yamlContent := `
workflows:
  one-off:
    - name: refactor
      description: "Refactor pass"
      steps:
        - name: refactoring
          prompt: "Refactor"
  tasks:
    - name: fast
      steps:
        - name: implementing
          prompt: "Implement"
  init:
    - name: spin-up
      description: "Spin up from PRD"
      steps:
        - name: analyzing
          prompt: "Analyze PRD"
          artifact: true
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := defaultConfig()
	if err := loadProjectConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}

	// Verify OneOff
	if len(cfg.OneOff) != 1 {
		t.Fatalf("expected 1 one-off workflow, got %d", len(cfg.OneOff))
	}
	if cfg.OneOff[0].Name != "refactor" {
		t.Errorf("expected one-off name 'refactor', got %q", cfg.OneOff[0].Name)
	}
	if cfg.OneOff[0].Description != "Refactor pass" {
		t.Errorf("expected one-off description 'Refactor pass', got %q", cfg.OneOff[0].Description)
	}

	// Verify TaskWorkflows
	if len(cfg.TaskWorkflows) != 1 {
		t.Fatalf("expected 1 task workflow, got %d", len(cfg.TaskWorkflows))
	}
	if cfg.TaskWorkflows[0].Name != "fast" {
		t.Errorf("expected task workflow name 'fast', got %q", cfg.TaskWorkflows[0].Name)
	}

	// Verify InitWorkflows
	if len(cfg.InitWorkflows) != 1 {
		t.Fatalf("expected 1 init workflow, got %d", len(cfg.InitWorkflows))
	}
	if cfg.InitWorkflows[0].Name != "spin-up" {
		t.Errorf("expected init workflow name 'spin-up', got %q", cfg.InitWorkflows[0].Name)
	}
	if cfg.InitWorkflows[0].Description != "Spin up from PRD" {
		t.Errorf("expected init description 'Spin up from PRD', got %q", cfg.InitWorkflows[0].Description)
	}

	// Verify workflow resolution
	wf := cfg.GetWorkflow("oneoff:refactor")
	if wf == nil || wf.Name != "oneoff:refactor" {
		t.Errorf("expected to resolve 'oneoff:refactor'")
	}

	wf2 := cfg.GetWorkflow("fast")
	if wf2 == nil || wf2.Name != "fast" {
		t.Errorf("expected to resolve 'fast'")
	}

	wf3 := cfg.GetWorkflow("init:spin-up")
	if wf3 == nil || wf3.Name != "init:spin-up" {
		t.Errorf("expected to resolve 'init:spin-up'")
	}

	// Verify listing functions
	workflowNames := cfg.ListWorkflowNames()
	if len(workflowNames) != 1 || workflowNames[0] != "fast" {
		t.Errorf("expected ListWorkflowNames to return ['fast'], got %v", workflowNames)
	}

	predefinedNames := cfg.ListPredefinedTaskNames()
	if len(predefinedNames) != 1 || predefinedNames[0] != "refactor" {
		t.Errorf("expected ListPredefinedTaskNames to return ['refactor'], got %v", predefinedNames)
	}

	initNames := cfg.ListInitWorkflowNames()
	if len(initNames) != 1 || initNames[0] != "spin-up" {
		t.Errorf("expected ListInitWorkflowNames to return ['spin-up'], got %v", initNames)
	}
}

func TestWorkflowCategoryMergingPreservesGlobalInit(t *testing.T) {
	dir := t.TempDir()

	// Global config defines init workflows
	globalPath := filepath.Join(dir, "global.sortie.yml")
	globalYAML := `
workflows:
  tasks:
    - name: global-task
      steps:
        - name: implementing
          prompt: "Implement"
  init:
    - name: spin-up-from-prd
      description: "Spin up from PRD"
      steps:
        - name: analyzing
          prompt: "Analyze PRD"
`
	if err := os.WriteFile(globalPath, []byte(globalYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Project config defines tasks and one-off but NO init
	projectPath := filepath.Join(dir, "project.sortie.yml")
	projectYAML := `
workflows:
  one-off:
    - name: refactor
      description: "Refactor pass"
      steps:
        - name: refactoring
          prompt: "Refactor"
  tasks:
    - name: project-task
      steps:
        - name: implementing
          prompt: "Implement"
`
	if err := os.WriteFile(projectPath, []byte(projectYAML), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := defaultConfig()
	// Load global first, then project (same order as real loading)
	if err := loadProjectConfig(globalPath, cfg); err != nil {
		t.Fatal(err)
	}
	if err := loadProjectConfig(projectPath, cfg); err != nil {
		t.Fatal(err)
	}

	// Project tasks should override global tasks
	if len(cfg.TaskWorkflows) != 1 || cfg.TaskWorkflows[0].Name != "project-task" {
		t.Errorf("expected project task workflow to override global, got %v", cfg.ListWorkflowNames())
	}

	// Project one-off should be present
	if len(cfg.OneOff) != 1 || cfg.OneOff[0].Name != "refactor" {
		t.Errorf("expected project one-off workflow, got %v", cfg.ListPredefinedTaskNames())
	}

	// Global init workflows should be preserved (not wiped)
	initNames := cfg.ListInitWorkflowNames()
	if len(initNames) != 1 || initNames[0] != "spin-up-from-prd" {
		t.Errorf("expected global init workflow to be preserved, got %v", initNames)
	}

	// Init workflow should be resolvable via flat list
	wf := cfg.GetWorkflow("init:spin-up-from-prd")
	if wf == nil {
		t.Error("expected to resolve 'init:spin-up-from-prd' but got nil")
	}
}

func TestLegacyWorkflowsListFormatStillWorks(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".sortie.yml")

	yamlContent := `
workflows:
  - name: default
    steps:
      - name: implementing
        prompt: "Implement"
  - name: review
    steps:
      - name: reviewing
        prompt: "Review"
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := defaultConfig()
	if err := loadProjectConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}

	// Legacy list format should populate TaskWorkflows
	if len(cfg.TaskWorkflows) != 2 {
		t.Fatalf("expected 2 task workflows from legacy format, got %d", len(cfg.TaskWorkflows))
	}
	if cfg.TaskWorkflows[0].Name != "default" {
		t.Errorf("expected first workflow name 'default', got %q", cfg.TaskWorkflows[0].Name)
	}
	if cfg.TaskWorkflows[1].Name != "review" {
		t.Errorf("expected second workflow name 'review', got %q", cfg.TaskWorkflows[1].Name)
	}

	// ListWorkflowNames should return them
	names := cfg.ListWorkflowNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 workflow names, got %d: %v", len(names), names)
	}
	if names[0] != "default" || names[1] != "review" {
		t.Errorf("expected ['default', 'review'], got %v", names)
	}
}

// Helper function to check if string contains substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && contains(s, substr))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestWorktreeSyncPathsParsing(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".sortie.yml")

	yamlContent := `
worktree-sync-paths:
  - .claude
  - .env.example
  - scripts/setup.sh
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := defaultConfig()
	if err := loadProjectConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}

	if len(cfg.WorktreeSyncPaths) != 3 {
		t.Fatalf("expected 3 sync paths, got %d", len(cfg.WorktreeSyncPaths))
	}
	if cfg.WorktreeSyncPaths[0] != ".claude" {
		t.Errorf("expected first path '.claude', got %q", cfg.WorktreeSyncPaths[0])
	}
	if cfg.WorktreeSyncPaths[1] != ".env.example" {
		t.Errorf("expected second path '.env.example', got %q", cfg.WorktreeSyncPaths[1])
	}
	if cfg.WorktreeSyncPaths[2] != "scripts/setup.sh" {
		t.Errorf("expected third path 'scripts/setup.sh', got %q", cfg.WorktreeSyncPaths[2])
	}
}

func TestWorktreeSyncPathsProjectOverridesGlobal(t *testing.T) {
	globalDir := t.TempDir()
	globalPath := filepath.Join(globalDir, ".sortie.yml")
	os.WriteFile(globalPath, []byte(`
worktree-sync-paths:
  - .claude
  - .env
`), 0644)

	projectDir := t.TempDir()
	projectPath := filepath.Join(projectDir, ".sortie.yml")
	os.WriteFile(projectPath, []byte(`
worktree-sync-paths:
  - .vscode
`), 0644)

	cfg := defaultConfig()
	if err := loadProjectConfig(globalPath, cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.WorktreeSyncPaths) != 2 {
		t.Fatalf("expected 2 sync paths from global, got %d", len(cfg.WorktreeSyncPaths))
	}

	if err := loadProjectConfig(projectPath, cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.WorktreeSyncPaths) != 1 {
		t.Fatalf("expected 1 sync path after project override, got %d", len(cfg.WorktreeSyncPaths))
	}
	if cfg.WorktreeSyncPaths[0] != ".vscode" {
		t.Errorf("expected '.vscode', got %q", cfg.WorktreeSyncPaths[0])
	}
}

func TestWorktreeSyncPathsPerWorkflow(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".sortie.yml")

	yamlContent := `
worktree-sync-paths:
  - .claude
workflows:
  tasks:
    - name: custom
      worktree-sync-paths:
        - .claude
        - .vscode
      steps:
        - name: implementing
          prompt: "Implement"
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := defaultConfig()
	if err := loadProjectConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}

	// Global level
	if len(cfg.WorktreeSyncPaths) != 1 {
		t.Fatalf("expected 1 global sync path, got %d", len(cfg.WorktreeSyncPaths))
	}

	// Per-workflow override
	wf := cfg.GetWorkflow("custom")
	paths := cfg.GetWorktreeSyncPaths(wf)
	if len(paths) != 2 {
		t.Fatalf("expected 2 per-workflow sync paths, got %d", len(paths))
	}
	if paths[0] != ".claude" || paths[1] != ".vscode" {
		t.Errorf("expected [.claude, .vscode], got %v", paths)
	}
}

func TestGetWorktreeSyncPathsFallsBackToGlobal(t *testing.T) {
	cfg := &Config{
		WorktreeSyncPaths: []string{".claude", ".env"},
	}

	// Workflow without override should fall back to global
	wf := &WorkflowConfig{Name: "default"}
	paths := cfg.GetWorktreeSyncPaths(wf)
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths from global fallback, got %d", len(paths))
	}

	// Nil workflow should fall back to global
	paths = cfg.GetWorktreeSyncPaths(nil)
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths from nil workflow fallback, got %d", len(paths))
	}
}

func TestWorktreeSyncPathsDefaultEmpty(t *testing.T) {
	cfg := defaultConfig()
	if len(cfg.WorktreeSyncPaths) != 0 {
		t.Errorf("expected empty sync paths by default, got %v", cfg.WorktreeSyncPaths)
	}
}

func TestWorktreeSetupCommandParsing(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".sortie.yml")

	yamlContent := `
worktree-setup-command: "./scripts/bootstrap.sh {{worktree_path}}"
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := defaultConfig()
	if err := loadProjectConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}

	expected := "./scripts/bootstrap.sh {{worktree_path}}"
	if cfg.WorktreeSetupCommand != expected {
		t.Errorf("expected %q, got %q", expected, cfg.WorktreeSetupCommand)
	}
}

func TestWorktreeSetupCommandProjectOverridesGlobal(t *testing.T) {
	globalDir := t.TempDir()
	globalPath := filepath.Join(globalDir, ".sortie.yml")
	os.WriteFile(globalPath, []byte(`
worktree-setup-command: "./global-setup.sh"
`), 0644)

	projectDir := t.TempDir()
	projectPath := filepath.Join(projectDir, ".sortie.yml")
	os.WriteFile(projectPath, []byte(`
worktree-setup-command: "./project-setup.sh"
`), 0644)

	cfg := defaultConfig()
	if err := loadProjectConfig(globalPath, cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.WorktreeSetupCommand != "./global-setup.sh" {
		t.Errorf("expected global command, got %q", cfg.WorktreeSetupCommand)
	}

	if err := loadProjectConfig(projectPath, cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.WorktreeSetupCommand != "./project-setup.sh" {
		t.Errorf("expected project command to override, got %q", cfg.WorktreeSetupCommand)
	}
}

func TestWorktreeSetupCommandPerWorkflow(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".sortie.yml")

	yamlContent := `
worktree-setup-command: "./global-setup.sh"
workflows:
  tasks:
    - name: custom
      worktree-setup-command: "./custom-setup.sh"
      steps:
        - name: implement
          prompt: "do it"
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := defaultConfig()
	if err := loadProjectConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}

	if cfg.WorktreeSetupCommand != "./global-setup.sh" {
		t.Fatalf("expected global command, got %q", cfg.WorktreeSetupCommand)
	}

	wf := cfg.GetWorkflow("custom")
	if wf == nil {
		t.Fatal("expected to find 'custom' workflow")
	}

	cmd := cfg.GetWorktreeSetupCommand(wf)
	if cmd != "./custom-setup.sh" {
		t.Errorf("expected workflow-level command, got %q", cmd)
	}
}

func TestGetWorktreeSetupCommandFallsBackToGlobal(t *testing.T) {
	cfg := &Config{
		WorktreeSetupCommand: "./global-setup.sh",
	}

	// Workflow without override — should fall back
	wf := &WorkflowConfig{Name: "test"}
	cmd := cfg.GetWorktreeSetupCommand(wf)
	if cmd != "./global-setup.sh" {
		t.Errorf("expected global fallback, got %q", cmd)
	}

	// Nil workflow — should fall back
	cmd = cfg.GetWorktreeSetupCommand(nil)
	if cmd != "./global-setup.sh" {
		t.Errorf("expected global fallback for nil workflow, got %q", cmd)
	}
}

func TestWorktreeSetupCommandDefaultEmpty(t *testing.T) {
	cfg := defaultConfig()
	if cfg.WorktreeSetupCommand != "" {
		t.Errorf("expected empty setup command by default, got %q", cfg.WorktreeSetupCommand)
	}
}
