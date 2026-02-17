package workflow

import (
	"os"
	"path/filepath"
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

func TestSummarizerSkipsWhenNoArtifacts(t *testing.T) {
	// When all steps have artifact: false, stepNames is empty,
	// CollectArtifacts returns empty map, summarizer should skip
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
