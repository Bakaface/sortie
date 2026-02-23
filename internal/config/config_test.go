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
