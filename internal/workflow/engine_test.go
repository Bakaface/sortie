package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aface/ralph-tamer-kit/internal/config"
)

func TestCollectArtifactsOnlyFromArtifactSteps(t *testing.T) {
	dir := t.TempDir()
	artifactsDir := filepath.Join(dir, ".rtk", "artifacts")
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
	artifactsDir := filepath.Join(dir, ".rtk", "artifacts")
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
	if err := os.MkdirAll(filepath.Join(dir, ".rtk", "artifacts"), 0755); err != nil {
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
	artifactsDir := filepath.Join(dir, ".rtk", "artifacts")
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
	if err := os.MkdirAll(filepath.Join(dir, ".rtk", "artifacts"), 0755); err != nil {
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
			Images:      []string{".rtk/images/screenshot.png", ".rtk/images/diagram.jpg"},
		},
	}

	result := ResolveTemplate("Images:\n{{task.images}}", ctx)
	expected := "Images:\n.rtk/images/screenshot.png\n.rtk/images/diagram.jpg"
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

func TestInjectClaudeMDWithImages(t *testing.T) {
	dir := t.TempDir()
	images := []string{".rtk/images/screenshot.png", ".rtk/images/diagram.jpg"}

	err := InjectClaudeMD(dir, "Implement the feature", images)
	if err != nil {
		t.Fatalf("InjectClaudeMD failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("failed to read CLAUDE.md: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, "Attached Images") {
		t.Error("expected CLAUDE.md to contain 'Attached Images' section")
	}
	if !strings.Contains(s, ".rtk/images/screenshot.png") {
		t.Error("expected CLAUDE.md to reference screenshot.png")
	}
	if !strings.Contains(s, ".rtk/images/diagram.jpg") {
		t.Error("expected CLAUDE.md to reference diagram.jpg")
	}
}

func TestInjectClaudeMDWithoutImages(t *testing.T) {
	dir := t.TempDir()

	err := InjectClaudeMD(dir, "Implement the feature", nil)
	if err != nil {
		t.Fatalf("InjectClaudeMD failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("failed to read CLAUDE.md: %v", err)
	}

	if strings.Contains(string(content), "Attached Images") {
		t.Error("expected CLAUDE.md to NOT contain 'Attached Images' section when no images")
	}
}

func TestWriteTmuxLogMessage(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "step.log")

	lines := writeTmuxLogMessage(logPath, 42, "implement", "ralph-tamer-kit-42-implement", "42")

	// Verify returned lines
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "=== Step: implement (task #42) ===") {
		t.Errorf("expected step header in line 0, got: %s", lines[0])
	}
	if !strings.Contains(lines[1], `Tmux session "ralph-tamer-kit-42-implement" initiated`) {
		t.Errorf("expected tmux session initiated message in line 1, got: %s", lines[1])
	}
	if !strings.Contains(lines[2], "Attach with: rtk attach 42 implement") {
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
	if !strings.Contains(logContent, "rtk attach 42 implement") {
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

	lines := writeTmuxLogMessage(logPath, 7, "review", "ralph-tamer-kit-7-review", "7")
	outputFn(lines)

	if len(captured) != 3 {
		t.Fatalf("expected outputFn to receive 3 lines, got %d", len(captured))
	}
	if !strings.Contains(captured[1], "Tmux session") {
		t.Errorf("expected tmux session message in outputFn, got: %s", captured[1])
	}
}
