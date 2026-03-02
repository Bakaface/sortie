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

	// Check predefined tasks were parsed
	if len(cfg.Tasks) != 2 {
		t.Fatalf("expected 2 predefined tasks, got %d", len(cfg.Tasks))
	}

	if cfg.Tasks[0].Name != "Refactor: Housekeeping" {
		t.Errorf("expected first task name 'Refactor: Housekeeping', got %q", cfg.Tasks[0].Name)
	}
	if cfg.Tasks[0].Description != "Standard codebase maintenance" {
		t.Errorf("expected first task description 'Standard codebase maintenance', got %q", cfg.Tasks[0].Description)
	}
	if len(cfg.Tasks[0].Steps) != 2 {
		t.Fatalf("expected 2 steps in first task, got %d", len(cfg.Tasks[0].Steps))
	}
	if !cfg.Tasks[0].Steps[0].Artifact {
		t.Error("expected audit step to have artifact: true")
	}
	if !cfg.Tasks[0].Steps[1].Human {
		t.Error("expected refactor step to have human: true")
	}

	// Check synthetic workflows were registered
	wf := cfg.GetWorkflow("task:Refactor: Housekeeping")
	if wf.Name != "task:Refactor: Housekeeping" {
		t.Errorf("expected synthetic workflow name 'task:Refactor: Housekeeping', got %q", wf.Name)
	}
	if len(wf.Steps) != 2 {
		t.Fatalf("expected 2 steps in synthetic workflow, got %d", len(wf.Steps))
	}

	wf2 := cfg.GetWorkflow("task:Security Scan")
	if wf2.Name != "task:Security Scan" {
		t.Errorf("expected synthetic workflow name 'task:Security Scan', got %q", wf2.Name)
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

	if cfg.Tasks[0].Name != "task-1" {
		t.Errorf("expected auto-named task 'task-1', got %q", cfg.Tasks[0].Name)
	}

	// Should be resolvable as workflow
	wf := cfg.GetWorkflow("task:task-1")
	if wf.Name != "task:task-1" {
		t.Errorf("expected synthetic workflow 'task:task-1', got %q", wf.Name)
	}
}

func TestListPredefinedTaskNames(t *testing.T) {
	cfg := &Config{
		Tasks: []TaskConfig{
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
		Workflows: []WorkflowConfig{
			{Name: "default"},
			{Name: "task:Housekeeping"},
			{Name: "review"},
			{Name: "task:Security"},
		},
	}

	names := cfg.ListWorkflowNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 workflow names (excluding task: prefixed), got %d: %v", len(names), names)
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
		Workflows: []WorkflowConfig{
			{Name: "task:Housekeeping"},
			{Name: "task:Security"},
		},
	}

	names := cfg.ListWorkflowNames()
	if len(names) != 1 || names[0] != "default" {
		t.Errorf("expected [\"default\"] when only synthetic task workflows exist, got %v", names)
	}
}

func TestGetPredefinedTask(t *testing.T) {
	cfg := &Config{
		Tasks: []TaskConfig{
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
	cfg := &Config{
		Tasks: []TaskConfig{
			{Name: "task-a"},
			{Name: "task-b", Unlisted: true},
			{Name: "task-c"},
		},
	}

	names := cfg.ListPredefinedTaskNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 names (excluding unlisted), got %d: %v", len(names), names)
	}
	if names[0] != "task-a" {
		t.Errorf("expected first name 'task-a', got %q", names[0])
	}
	if names[1] != "task-c" {
		t.Errorf("expected second name 'task-c', got %q", names[1])
	}
}

func TestListAllPredefinedTaskNamesIncludesUnlisted(t *testing.T) {
	cfg := &Config{
		Tasks: []TaskConfig{
			{Name: "task-a"},
			{Name: "task-b", Unlisted: true},
			{Name: "task-c"},
		},
	}

	names := cfg.ListAllPredefinedTaskNames()
	if len(names) != 3 {
		t.Fatalf("expected 3 names (including unlisted), got %d: %v", len(names), names)
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
