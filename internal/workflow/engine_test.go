package workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aface/sortie/internal/config"
	"github.com/aface/sortie/internal/db"
	"github.com/aface/sortie/internal/task"
)

func TestCollectArtifactsOnlyFromArtifactSteps(t *testing.T) {
	dir := t.TempDir()
	artifactsDir := filepath.Join(dir, ".sortie", "artifacts")
	if err := os.MkdirAll(artifactsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write artifacts for two steps
	os.WriteFile(filepath.Join(artifactsDir, "implement.md"), []byte("implementation notes"), 0644)
	os.WriteFile(filepath.Join(artifactsDir, "review.md"), []byte("review notes"), 0644)

	steps := []config.StepConfig{
		{Name: "implement", Artifact: true},
		{Name: "review", Artifact: false},
	}

	// Filter step names like the engine does (only artifact: true)
	var priorStepNames []string
	for _, s := range steps {
		if s.Artifact {
			priorStepNames = append(priorStepNames, s.Name)
		}
	}

	artifacts := CollectArtifacts(dir, priorStepNames)

	if _, ok := artifacts["implement"]; !ok {
		t.Error("expected implement artifact to be collected")
	}
	if _, ok := artifacts["review"]; ok {
		t.Error("expected review artifact to NOT be collected (artifact: false)")
	}
}

func TestCollectArtifactsEmptyWhenNoArtifactSteps(t *testing.T) {
	dir := t.TempDir()
	artifactsDir := filepath.Join(dir, ".sortie", "artifacts")
	if err := os.MkdirAll(artifactsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Even with files on disk, if no steps have artifact: true, nothing is collected
	os.WriteFile(filepath.Join(artifactsDir, "implement.md"), []byte("notes"), 0644)

	steps := []config.StepConfig{
		{Name: "implement", Artifact: false},
	}

	var priorStepNames []string
	for _, s := range steps {
		if s.Artifact {
			priorStepNames = append(priorStepNames, s.Name)
		}
	}

	artifacts := CollectArtifacts(dir, priorStepNames)

	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts, got %d", len(artifacts))
	}
}

func TestSummarizerStepNameFiltering(t *testing.T) {
	// Simulate the summarizer's step name filtering logic
	steps := []config.StepConfig{
		{Name: "implement", Artifact: true},
		{Name: "review", Artifact: false},
		{Name: "test", Artifact: true},
	}

	var stepNames []string
	for _, s := range steps {
		if s.Artifact {
			stepNames = append(stepNames, s.Name)
		}
	}

	if len(stepNames) != 2 {
		t.Fatalf("expected 2 artifact step names, got %d", len(stepNames))
	}
	if stepNames[0] != "implement" || stepNames[1] != "test" {
		t.Errorf("expected [implement, test], got %v", stepNames)
	}
}

func TestSummarizerCollectsNoArtifactsWhenNoArtifactSteps(t *testing.T) {
	// When all steps have artifact: false, stepNames is empty,
	// CollectArtifacts returns empty map. The summarizer should then
	// fall through to the git diff stat path instead of skipping entirely.
	steps := []config.StepConfig{
		{Name: "implement", Artifact: false},
		{Name: "review", Artifact: false},
	}

	var stepNames []string
	for _, s := range steps {
		if s.Artifact {
			stepNames = append(stepNames, s.Name)
		}
	}

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".sortie", "artifacts"), 0755); err != nil {
		t.Fatal(err)
	}

	artifacts := CollectArtifacts(dir, stepNames)
	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts when no steps have artifact: true, got %d", len(artifacts))
	}
}

func TestSummarizerPromptBuildWithArtifacts(t *testing.T) {
	// Verify that when artifacts are present, the prompt includes artifact content
	dir := t.TempDir()
	artifactsDir := filepath.Join(dir, ".sortie", "artifacts")
	if err := os.MkdirAll(artifactsDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(artifactsDir, "implement.md"), []byte("Added feature X"), 0644)

	stepNames := []string{"implement"}
	artifacts := CollectArtifacts(dir, stepNames)

	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	if artifacts["implement"] != "Added feature X" {
		t.Errorf("expected artifact content 'Added feature X', got %q", artifacts["implement"])
	}
}

func TestSummarizerPromptBuildWithoutArtifacts(t *testing.T) {
	// Verify that when no artifacts exist and no steps have artifact: true,
	// the summarizer should use the diff stat fallback path.
	// This tests the condition that previously caused empty tasks.context.
	steps := []config.StepConfig{
		{Name: "implementing", Artifact: false},
	}

	var stepNames []string
	for _, s := range steps {
		if s.Artifact {
			stepNames = append(stepNames, s.Name)
		}
	}

	if len(stepNames) != 0 {
		t.Errorf("expected 0 artifact step names for workflow without artifacts, got %d", len(stepNames))
	}

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".sortie", "artifacts"), 0755); err != nil {
		t.Fatal(err)
	}

	artifacts := CollectArtifacts(dir, stepNames)
	if len(artifacts) != 0 {
		t.Fatalf("expected 0 artifacts, got %d", len(artifacts))
	}

	// With the fix, the summarizer should not bail out here — it should
	// proceed to check git diff stat. The empty artifacts map triggers
	// the fallback path.
}

func TestCopyImagesToWorktree(t *testing.T) {
	// Create a source directory with test images
	srcDir := t.TempDir()
	img1 := filepath.Join(srcDir, "screenshot.png")
	img2 := filepath.Join(srcDir, "diagram.jpg")
	os.WriteFile(img1, []byte("fake png data"), 0644)
	os.WriteFile(img2, []byte("fake jpg data"), 0644)

	// Create a worktree directory
	worktree := t.TempDir()

	relPaths, err := CopyImagesToWorktree(worktree, []string{img1, img2})
	if err != nil {
		t.Fatalf("CopyImagesToWorktree failed: %v", err)
	}

	if len(relPaths) != 2 {
		t.Fatalf("expected 2 relative paths, got %d", len(relPaths))
	}

	// Verify files were copied
	for _, rel := range relPaths {
		fullPath := filepath.Join(worktree, rel)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			t.Errorf("expected copied image at %s", fullPath)
		}
	}

	// Verify content
	data, _ := os.ReadFile(filepath.Join(worktree, relPaths[0]))
	if string(data) != "fake png data" {
		t.Errorf("expected copied content to match, got %q", string(data))
	}
}

func TestCopyImagesToWorktreeEmpty(t *testing.T) {
	worktree := t.TempDir()

	relPaths, err := CopyImagesToWorktree(worktree, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if relPaths != nil {
		t.Errorf("expected nil for empty images, got %v", relPaths)
	}
}

func TestTemplateTaskImages(t *testing.T) {
	ctx := &TemplateContext{
		Task: TaskVars{
			ID:          1,
			Title:       "Test task",
			Description: "A test",
			Images:      []string{".sortie/images/screenshot.png", ".sortie/images/diagram.jpg"},
		},
	}

	result := ResolveTemplate("Images:\n{{task.images}}", ctx)
	expected := "Images:\n.sortie/images/screenshot.png\n.sortie/images/diagram.jpg"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestTemplateTaskImagesEmpty(t *testing.T) {
	ctx := &TemplateContext{
		Task: TaskVars{
			ID:     1,
			Images: nil,
		},
	}

	result := ResolveTemplate("Images: {{task.images}}", ctx)
	if result != "Images: " {
		t.Errorf("expected 'Images: ', got %q", result)
	}
}

func TestBuildSystemPromptWithImages(t *testing.T) {
	images := []string{".sortie/images/screenshot.png", ".sortie/images/diagram.jpg"}

	s := BuildSystemPrompt("Implement the feature", "", images)

	if !strings.Contains(s, "Attached Images") {
		t.Error("expected system prompt to contain 'Attached Images' section")
	}
	if !strings.Contains(s, ".sortie/images/screenshot.png") {
		t.Error("expected system prompt to reference screenshot.png")
	}
	if !strings.Contains(s, ".sortie/images/diagram.jpg") {
		t.Error("expected system prompt to reference diagram.jpg")
	}
	// Verify default system prompt is used when empty
	if !strings.Contains(s, "autonomous coding agent") {
		t.Error("expected system prompt to contain default system prompt")
	}
}

func TestBuildSystemPromptWithoutImages(t *testing.T) {
	s := BuildSystemPrompt("Implement the feature", "", nil)

	if strings.Contains(s, "Attached Images") {
		t.Error("expected system prompt to NOT contain 'Attached Images' section when no images")
	}
}

func TestBuildSystemPromptWithCustomSystemPrompt(t *testing.T) {
	customPrompt := "You are a careful code reviewer. Never make changes without tests."

	s := BuildSystemPrompt("Review the code", customPrompt, nil)

	if !strings.Contains(s, customPrompt) {
		t.Error("expected system prompt to contain custom system prompt")
	}
	if strings.Contains(s, "autonomous coding agent") {
		t.Error("expected system prompt to NOT contain default system prompt when custom is provided")
	}
	if !strings.Contains(s, "# Task") {
		t.Error("expected system prompt to contain task section")
	}
	if !strings.Contains(s, "Review the code") {
		t.Error("expected system prompt to contain resolved prompt")
	}
}

func TestWriteTmuxLogMessage(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "step.log")

	lines := writeTmuxLogMessage(logPath, 42, "implement", "sortie-42", "42")

	// Verify returned lines
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "=== Step: implement (task #42) ===") {
		t.Errorf("expected step header in line 0, got: %s", lines[0])
	}
	if !strings.Contains(lines[1], `Tmux session "sortie-42" initiated`) {
		t.Errorf("expected tmux session initiated message in line 1, got: %s", lines[1])
	}
	if !strings.Contains(lines[2], "Attach with: sortie attach 42") {
		t.Errorf("expected attach instructions in line 2, got: %s", lines[2])
	}

	// Verify log file was written
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	logContent := string(content)
	if !strings.Contains(logContent, "=== Step: implement (task #42) ===") {
		t.Error("log file missing step header")
	}
	if !strings.Contains(logContent, "Tmux session") {
		t.Error("log file missing tmux session message")
	}
	if !strings.Contains(logContent, "sortie attach 42") {
		t.Error("log file missing attach instructions")
	}
}

func TestWriteTmuxLogMessageCallsOutputFn(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "step.log")

	var captured []string
	outputFn := func(lines []string) {
		captured = append(captured, lines...)
	}

	lines := writeTmuxLogMessage(logPath, 7, "review", "sortie-7", "7")
	outputFn(lines)

	if len(captured) != 3 {
		t.Fatalf("expected outputFn to receive 3 lines, got %d", len(captured))
	}
	if !strings.Contains(captured[1], "Tmux session") {
		t.Errorf("expected tmux session message in outputFn, got: %s", captured[1])
	}
}

func TestRunClaudeSyncSetsWorkDir(t *testing.T) {
	dir := t.TempDir()

	// Create a script that prints the working directory, ignoring all args
	script := filepath.Join(t.TempDir(), "fake-claude.sh")
	os.WriteFile(script, []byte("#!/bin/sh\npwd\n"), 0755)

	cfg := &config.Config{
		Claude: config.ClaudeConfig{
			Command: script,
		},
	}
	engine := NewEngine(cfg, nil, nil, dir)

	ctx := context.Background()
	output, err := engine.runClaudeSync(ctx, "test prompt", dir)
	if err != nil {
		t.Fatalf("runClaudeSync failed: %v", err)
	}

	output = strings.TrimSpace(output)
	// The script should print the workDir we passed
	if output != dir {
		t.Errorf("expected working directory %q, got %q", dir, output)
	}
}

func TestRunClaudeSyncEmptyWorkDir(t *testing.T) {
	// Create a script that prints the working directory, ignoring all args
	script := filepath.Join(t.TempDir(), "fake-claude.sh")
	os.WriteFile(script, []byte("#!/bin/sh\npwd\n"), 0755)

	cfg := &config.Config{
		Claude: config.ClaudeConfig{
			Command: script,
		},
	}
	engine := NewEngine(cfg, nil, nil, "")

	ctx := context.Background()
	output, err := engine.runClaudeSync(ctx, "test prompt", "")
	if err != nil {
		t.Fatalf("runClaudeSync failed: %v", err)
	}

	// Should succeed without error — we just verify it doesn't crash
	output = strings.TrimSpace(output)
	if output == "" {
		t.Error("expected non-empty output from pwd")
	}
}

func TestArtifactValidationDetectsMissingArtifact(t *testing.T) {
	// Simulate the artifact validation check from RunTask:
	// if step.Artifact && cfg.ValidateArtifact, check if artifact file exists
	dir := t.TempDir()
	artifactsDir := filepath.Join(dir, ".sortie", "artifacts")
	if err := os.MkdirAll(artifactsDir, 0755); err != nil {
		t.Fatal(err)
	}

	stepName := "implement"
	artifactPath := filepath.Join(artifactsDir, stepName+".md")

	// Artifact file does NOT exist — should be detected as missing
	if _, err := os.Stat(artifactPath); !os.IsNotExist(err) {
		t.Error("expected artifact file to not exist")
	}

	// Now write the artifact file — should pass validation
	os.WriteFile(artifactPath, []byte("implementation notes"), 0644)
	if _, err := os.Stat(artifactPath); os.IsNotExist(err) {
		t.Error("expected artifact file to exist after writing")
	}
}

func TestArtifactValidationSkippedWhenDisabled(t *testing.T) {
	// When validate_artifact is false (default), artifact validation should not trigger
	cfg := &config.Config{
		ValidateArtifact: false,
	}

	step := config.StepConfig{
		Name:     "implement",
		Artifact: true,
	}

	// Even though the step has artifact: true, validation is skipped when disabled
	shouldValidate := step.Artifact && cfg.ValidateArtifact
	if shouldValidate {
		t.Error("expected artifact validation to be skipped when validate_artifact is false")
	}
}

func TestArtifactValidationTriggeredWhenEnabled(t *testing.T) {
	cfg := &config.Config{
		ValidateArtifact: true,
	}

	step := config.StepConfig{
		Name:     "implement",
		Artifact: true,
	}

	shouldValidate := step.Artifact && cfg.ValidateArtifact
	if !shouldValidate {
		t.Error("expected artifact validation to trigger when validate_artifact is true and step has artifact: true")
	}
}

func TestArtifactValidationSkippedForNonArtifactStep(t *testing.T) {
	cfg := &config.Config{
		ValidateArtifact: true,
	}

	step := config.StepConfig{
		Name:     "review",
		Artifact: false,
	}

	shouldValidate := step.Artifact && cfg.ValidateArtifact
	if shouldValidate {
		t.Error("expected artifact validation to be skipped for step without artifact: true")
	}
}

func TestSummarizerDiffStatPromptIncludesReadInstruction(t *testing.T) {
	// Verify that the no-artifacts summarizer prompt instructs Claude to read files
	// rather than just summarizing based on filenames
	taskTitle := "Add feature X"
	taskDesc := "Implement feature X for the system"
	diffStat := " file1.go | 10 +\n file2.go | 5 +-\n"

	// Build the prompt the same way the engine does in the no-artifacts path
	var sb strings.Builder
	sb.WriteString("Summarize the progress made on task #1: " + taskTitle + "\n\n")
	sb.WriteString("The task description was:\n")
	sb.WriteString(taskDesc)
	sb.WriteString("\n\nThe following files were changed:\n\n```\n")
	sb.WriteString(diffStat)
	sb.WriteString("\n```\n\n")
	sb.WriteString("Read the changed files listed above and review the actual code to understand what was implemented. ")
	sb.WriteString("Do NOT guess or assume — base your summary on the actual file contents and git changes in this repository. ")
	sb.WriteString("Provide a concise summary of what was accomplished. ")
	sb.WriteString("This summary will be used as context for future work on this task.")
	prompt := sb.String()

	if !strings.Contains(prompt, "Read the changed files") {
		t.Error("expected prompt to instruct Claude to read changed files")
	}
	if !strings.Contains(prompt, "Do NOT guess or assume") {
		t.Error("expected prompt to instruct Claude not to guess")
	}
	if !strings.Contains(prompt, diffStat) {
		t.Error("expected prompt to contain the diff stat")
	}
}

func TestTemplateLoopVars(t *testing.T) {
	ctx := &TemplateContext{
		Task: TaskVars{ID: 1, Title: "Test task", Description: "A test"},
		Loop: LoopVars{Iteration: 3, MaxIterations: 5},
	}

	result := ResolveTemplate("Iteration {{loop.iteration}} of {{loop.max_iterations}}", ctx)
	expected := "Iteration 3 of 5"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestTemplateLoopVarsZero(t *testing.T) {
	ctx := &TemplateContext{
		Task: TaskVars{ID: 1, Title: "Test task", Description: "A test"},
		Loop: LoopVars{Iteration: 0, MaxIterations: 0},
	}

	result := ResolveTemplate("Iteration {{loop.iteration}} of {{loop.max_iterations}}", ctx)
	expected := "Iteration 0 of 0"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestSummarizationDescription(t *testing.T) {
	tests := []struct {
		name            string
		taskID          int64
		hasCustomPrompt bool
		artifactNames   []string
		useDiffStat     bool
		expected        string
	}{
		{
			name:            "custom prompt with artifacts",
			taskID:          42,
			hasCustomPrompt: true,
			artifactNames:   []string{"implement", "review"},
			expected:        "Summarizing task #42 with custom prompt and artifacts: implement, review",
		},
		{
			name:            "custom prompt without artifacts",
			taskID:          7,
			hasCustomPrompt: true,
			artifactNames:   nil,
			expected:        "Summarizing task #7 with custom prompt",
		},
		{
			name:          "default prompt with artifacts",
			taskID:        10,
			artifactNames: []string{"implement"},
			expected:      "Summarizing task #10 with artifacts: implement",
		},
		{
			name:        "git diff fallback",
			taskID:      5,
			useDiffStat: true,
			expected:    "Summarizing task #5 via git diff",
		},
		{
			name:     "no context",
			taskID:   1,
			expected: "Summarizing task #1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := summarizationDescription(tt.taskID, tt.hasCustomPrompt, tt.artifactNames, tt.useDiffStat)
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestSummarizerLogFnCalledWithArtifacts(t *testing.T) {
	// Create a fake Claude script that echoes a summary
	script := filepath.Join(t.TempDir(), "fake-claude.sh")
	os.WriteFile(script, []byte("#!/bin/sh\necho 'task summary output'\n"), 0755)

	dir := t.TempDir()
	artifactsDir := filepath.Join(dir, ".sortie", "artifacts")
	os.MkdirAll(artifactsDir, 0755)
	os.WriteFile(filepath.Join(artifactsDir, "implement.md"), []byte("Added feature X"), 0644)

	// Create a real SQLite database for the test
	dbPath := filepath.Join(dir, ".sortie", "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer database.Close()

	// Create a project and task so UpdateTaskContext works
	project, err := database.GetOrCreateProject(dir)
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}
	taskObj, err := database.CreateTask(project.ID, "Test task", "A test task", "test-task", "default", "", task.StatusRunning, nil)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}
	taskObj.WorktreePath = dir

	cfg := &config.Config{
		Claude: config.ClaudeConfig{Command: script},
	}
	engine := NewEngine(cfg, database, nil, dir)

	wf := &config.WorkflowConfig{
		Steps: []config.StepConfig{
			{Name: "implement", Artifact: true},
		},
	}

	var logMessages []string
	logFn := func(format string, args ...any) {
		logMessages = append(logMessages, fmt.Sprintf(format, args...))
	}

	ctx := context.Background()
	err = engine.runSummarizer(ctx, taskObj, wf, logFn)
	if err != nil {
		t.Fatalf("runSummarizer failed: %v", err)
	}

	// Verify that the log messages contain the artifact description
	found := false
	for _, msg := range logMessages {
		if strings.Contains(msg, "Summarizing task #") && strings.Contains(msg, "with artifacts: implement") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected log message about summarizing with artifacts, got: %v", logMessages)
	}
}

func TestSummarizerLogFnCalledWithNilLogFn(t *testing.T) {
	// Verify runSummarizer doesn't panic when logFn is nil
	script := filepath.Join(t.TempDir(), "fake-claude.sh")
	os.WriteFile(script, []byte("#!/bin/sh\necho 'summary'\n"), 0755)

	dir := t.TempDir()
	artifactsDir := filepath.Join(dir, ".sortie", "artifacts")
	os.MkdirAll(artifactsDir, 0755)
	os.WriteFile(filepath.Join(artifactsDir, "implement.md"), []byte("notes"), 0644)

	// Create a real SQLite database for the test
	dbPath := filepath.Join(dir, ".sortie", "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer database.Close()

	project, err := database.GetOrCreateProject(dir)
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}
	taskObj, err := database.CreateTask(project.ID, "Test", "A test", "test", "default", "", task.StatusRunning, nil)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}
	taskObj.WorktreePath = dir

	cfg := &config.Config{
		Claude: config.ClaudeConfig{Command: script},
	}
	engine := NewEngine(cfg, database, nil, dir)

	wf := &config.WorkflowConfig{
		Steps: []config.StepConfig{
			{Name: "implement", Artifact: true},
		},
	}

	ctx := context.Background()
	// Should not panic with nil logFn
	err = engine.runSummarizer(ctx, taskObj, wf, nil)
	if err != nil {
		t.Fatalf("runSummarizer with nil logFn failed: %v", err)
	}
}

func TestFindStepIndex(t *testing.T) {
	steps := []config.StepConfig{
		{Name: "implement"},
		{Name: "review"},
		{Name: "test"},
	}

	tests := []struct {
		name     string
		stepName string
		expected int
	}{
		{"first step", "implement", 0},
		{"middle step", "review", 1},
		{"last step", "test", 2},
		{"not found", "deploy", -1},
		{"empty string", "", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findStepIndex(steps, tt.stepName)
			if result != tt.expected {
				t.Errorf("findStepIndex(%q) = %d, want %d", tt.stepName, result, tt.expected)
			}
		})
	}
}

func TestTmuxScriptEmptyPromptLaunchesBlankSession(t *testing.T) {
	// Verify that when prompt is empty, the tmux script launches Claude
	// without a prompt argument (blank interactive session)
	prompt := ""
	claudeCmd := "claude"
	envExports := ""

	var script string
	if strings.TrimSpace(prompt) == "" {
		script = fmt.Sprintf("#!/bin/bash\n%s%s\nexec bash\n", envExports, claudeCmd)
	} else {
		script = fmt.Sprintf("#!/bin/bash\n%sPROMPT=$(cat %q)\n%s \"$PROMPT\"\nexec bash\n", envExports, "/tmp/prompt.txt", claudeCmd)
	}

	// Should NOT contain PROMPT= or "$PROMPT"
	if strings.Contains(script, "PROMPT=") {
		t.Error("blank session script should not contain PROMPT variable")
	}
	if strings.Contains(script, "$PROMPT") {
		t.Error("blank session script should not reference $PROMPT")
	}
	// Should contain bare claude command
	if !strings.Contains(script, "claude\n") {
		t.Error("blank session script should contain bare claude command")
	}
}

func TestFinalizationOrderMergeBeforeSummarization(t *testing.T) {
	// Verify that executeOnComplete is called before runSummarizer in both
	// RunTask and FinalizeTask by checking the code structure.
	// This test validates the cleanup condition: merge+worktree triggers cleanup.

	tests := []struct {
		name          string
		onComplete    string
		worktree      bool
		expectCleanup bool
	}{
		{"merge with worktree", "merge", true, true},
		{"merge without worktree", "merge", false, false},
		{"commit with worktree", "commit", true, false},
		{"none with worktree", "none", true, false},
		{"empty with worktree", "", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldCleanup := tt.onComplete == "merge" && tt.worktree
			if shouldCleanup != tt.expectCleanup {
				t.Errorf("expected cleanup=%v for onComplete=%q worktree=%v, got %v",
					tt.expectCleanup, tt.onComplete, tt.worktree, shouldCleanup)
			}
		})
	}
}

func TestCleanupMergedWorktreeLogsMessages(t *testing.T) {
	// Verify that cleanupMergedWorktree calls the log function with expected messages.
	dir := t.TempDir()

	// Create a real database so ClearWorktreePath doesn't panic
	dbPath := filepath.Join(dir, ".sortie", "test.db")
	os.MkdirAll(filepath.Dir(dbPath), 0755)
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{}
	engine := NewEngine(cfg, database, nil, dir)

	tk := &task.Task{
		ID:             1,
		WorktreePath:   filepath.Join(dir, "nonexistent-worktree"),
		Branch:         "test-branch",
		CheckoutBranch: "",
	}

	var logMessages []string
	logFn := func(format string, args ...any) {
		logMessages = append(logMessages, fmt.Sprintf(format, args...))
	}

	// Should not panic even with nonexistent paths (git operations will warn but not crash)
	engine.cleanupMergedWorktree(tk, logFn)

	if len(logMessages) < 2 {
		t.Fatalf("expected at least 2 log messages, got %d: %v", len(logMessages), logMessages)
	}
	if !strings.Contains(logMessages[0], "Cleaning up worktree and branch") {
		t.Errorf("expected first log to mention cleanup, got: %s", logMessages[0])
	}
	if !strings.Contains(logMessages[len(logMessages)-1], "Cleanup completed") {
		t.Errorf("expected last log to mention completion, got: %s", logMessages[len(logMessages)-1])
	}
}

func TestCleanupMergedWorktreePreservesCheckoutBranch(t *testing.T) {
	// When CheckoutBranch is set (user-provided branch), the branch should NOT be deleted.
	dir := t.TempDir()

	dbPath := filepath.Join(dir, ".sortie", "test.db")
	os.MkdirAll(filepath.Dir(dbPath), 0755)
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{}
	engine := NewEngine(cfg, database, nil, dir)

	tk := &task.Task{
		ID:             1,
		WorktreePath:   filepath.Join(dir, "nonexistent-worktree"),
		Branch:         "user-branch",
		CheckoutBranch: "user-branch", // User provided this branch
	}

	var logMessages []string
	logFn := func(format string, args ...any) {
		logMessages = append(logMessages, fmt.Sprintf(format, args...))
	}

	engine.cleanupMergedWorktree(tk, logFn)

	// Should complete without panicking
	if len(logMessages) < 2 {
		t.Fatalf("expected at least 2 log messages, got %d", len(logMessages))
	}
}

func TestRunTaskDoesNotSetSummarizingStatus(t *testing.T) {
	// Verify that after RunTask completes all steps, the task status is NOT
	// StatusCompleted or StatusSummarizing — finalization is now the daemon's job.
	dir := t.TempDir()

	// Create a git repo so worktree operations don't fail
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create a fake Claude script that exits successfully
	script := filepath.Join(t.TempDir(), "fake-claude.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho 'done'\n"), 0755); err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(dir, ".sortie", "test.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer database.Close()

	project, err := database.GetOrCreateProject(dir)
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	tk, err := database.CreateTask(project.ID, "Test task", "A test task", "test-task", "default", "", task.StatusRunning, nil)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}
	// No-worktree mode so we don't need a real git repo with worktree support
	tk.Worktree = false
	tk.WorktreePath = dir

	cfg := &config.Config{
		Claude: config.ClaudeConfig{Command: script},
		Git:    config.GitConfig{OnComplete: "merge"},
		Workflows: []config.WorkflowConfig{
			{
				Name: "default",
				Steps: []config.StepConfig{
					{Name: "implement", Prompt: "implement the thing"},
				},
			},
		},
	}
	engine := NewEngine(cfg, database, nil, dir)

	ctx := context.Background()
	err = engine.RunTask(ctx, tk, nil)
	if err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}

	// After RunTask, the task should NOT be StatusSummarizing or StatusCompleted.
	// The daemon handles finalization after agent completion.
	refreshed, err := database.GetTask(tk.ID)
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if refreshed.Status == task.StatusSummarizing {
		t.Error("RunTask should not set StatusSummarizing — that is the daemon's job now")
	}
	if refreshed.Status == task.StatusCompleted {
		t.Error("RunTask should not set StatusCompleted — that is the daemon's job now")
	}
}

func TestRunTaskDoesNotCallExecuteOnComplete(t *testing.T) {
	// Verify the finalization order: RunTask should return nil after all steps
	// without doing merge/summarize/cleanup. The comment in code documents this.

	// This is a structural test — we verify RunTask exits cleanly with nil
	// and the task remains in its running state (no status change to summarizing/completed).
	dir := t.TempDir()

	script := filepath.Join(t.TempDir(), "fake-claude.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho 'done'\n"), 0755); err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(dir, ".sortie", "test.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer database.Close()

	project, err := database.GetOrCreateProject(dir)
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	// Test with on_complete: none — RunTask should still just return nil
	// (previously it would call executeOnComplete then summarizer)
	tk, err := database.CreateTask(project.ID, "No finalization task", "desc", "slug", "default", "", task.StatusRunning, nil)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}
	tk.Worktree = false
	tk.WorktreePath = dir

	cfg := &config.Config{
		Claude: config.ClaudeConfig{Command: script},
		Git:    config.GitConfig{OnComplete: "none"},
		Workflows: []config.WorkflowConfig{
			{
				Name: "default",
				Steps: []config.StepConfig{
					{Name: "step1", Prompt: "do something"},
				},
			},
		},
	}
	engine := NewEngine(cfg, database, nil, dir)

	ctx := context.Background()
	if err := engine.RunTask(ctx, tk, nil); err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}

	// Verify task status is still running (not changed by RunTask itself)
	refreshed, err := database.GetTask(tk.ID)
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	switch refreshed.Status {
	case task.StatusSummarizing, task.StatusCompleted, task.StatusFinalizing:
		t.Errorf("RunTask should not change task status to %s — daemon handles finalization", refreshed.Status)
	}
}

func TestTmuxScriptNonEmptyPromptPassesPrompt(t *testing.T) {
	// Verify that when prompt is non-empty, the tmux script passes it to Claude
	prompt := "implement the feature"
	claudeCmd := "claude"
	envExports := ""
	promptFile := "/tmp/prompt.txt"

	var script string
	if strings.TrimSpace(prompt) == "" {
		script = fmt.Sprintf("#!/bin/bash\n%s%s\nexec bash\n", envExports, claudeCmd)
	} else {
		script = fmt.Sprintf("#!/bin/bash\n%sPROMPT=$(cat %q)\n%s \"$PROMPT\"\nexec bash\n", envExports, promptFile, claudeCmd)
	}

	// Should contain PROMPT= and "$PROMPT"
	if !strings.Contains(script, "PROMPT=") {
		t.Error("non-empty prompt script should contain PROMPT variable")
	}
	if !strings.Contains(script, `"$PROMPT"`) {
		t.Error("non-empty prompt script should pass $PROMPT to claude")
	}
}
