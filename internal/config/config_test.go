package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStepConfigArtifactParsing(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".rtk.yaml")

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
