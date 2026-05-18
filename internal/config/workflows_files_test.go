package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupProject writes a .sortie.yml and any .sortie/workflows/* files into a
// temp dir and returns the dir + .sortie.yml path.
func setupProject(t *testing.T, sortieYml string, files map[string]string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	ymlPath := filepath.Join(dir, ".sortie.yml")
	if err := os.WriteFile(ymlPath, []byte(sortieYml), 0644); err != nil {
		t.Fatalf("write .sortie.yml: %v", err)
	}
	for rel, content := range files {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return dir, ymlPath
}

func TestFileWorkflows_StringRefResolves(t *testing.T) {
	dir, _ := setupProject(t, `
workflows:
  tasks:
    - implement
`, map[string]string{
		".sortie/workflows/tasks/implement.yml": `
description: Implement task
steps:
  - name: do
    prompt: "do it"
`,
	})

	cfg, err := LoadForProject(dir)
	if err != nil {
		t.Fatalf("LoadForProject: %v", err)
	}
	if len(cfg.TaskWorkflows) != 1 {
		t.Fatalf("want 1 task workflow, got %d", len(cfg.TaskWorkflows))
	}
	wf := cfg.TaskWorkflows[0]
	if wf.Name != "implement" {
		t.Errorf("want name implement, got %q", wf.Name)
	}
	if wf.Hidden {
		t.Errorf("referenced workflow should not be hidden")
	}
	if wf.Source == "" || !strings.HasSuffix(wf.Source, "implement.yml") {
		t.Errorf("want source path ending with implement.yml, got %q", wf.Source)
	}
	if wf.Description != "Implement task" {
		t.Errorf("want description 'Implement task', got %q", wf.Description)
	}
}

func TestFileWorkflows_MissingRefIsHardError(t *testing.T) {
	dir, _ := setupProject(t, `
workflows:
  tasks:
    - nonexistent
`, nil)

	_, err := LoadForProject(dir)
	if err == nil || !strings.Contains(err.Error(), "nonexistent") {
		t.Fatalf("want missing-ref error, got %v", err)
	}
}

func TestFileWorkflows_InlineFileCollisionIsError(t *testing.T) {
	dir, _ := setupProject(t, `
workflows:
  tasks:
    - name: foo
      steps:
        - name: a
          prompt: "a"
`, map[string]string{
		".sortie/workflows/tasks/foo.yml": `
steps:
  - name: b
    prompt: "b"
`,
	})

	_, err := LoadForProject(dir)
	if err == nil || !strings.Contains(err.Error(), "collides") {
		t.Fatalf("want collision error, got %v", err)
	}
}

func TestFileWorkflows_UnreferencedFileLoadedAsHidden(t *testing.T) {
	dir, _ := setupProject(t, `
workflows:
  tasks:
    - active
`, map[string]string{
		".sortie/workflows/tasks/active.yml": `
steps:
  - name: a
    prompt: "a"
`,
		".sortie/workflows/tasks/hidden.yml": `
steps:
  - name: b
    prompt: "b"
`,
	})

	cfg, err := LoadForProject(dir)
	if err != nil {
		t.Fatalf("LoadForProject: %v", err)
	}
	if len(cfg.TaskWorkflows) != 2 {
		t.Fatalf("want 2 task workflows (1 active + 1 hidden), got %d", len(cfg.TaskWorkflows))
	}
	// Active first, hidden after.
	if cfg.TaskWorkflows[0].Name != "active" || cfg.TaskWorkflows[0].Hidden {
		t.Errorf("want first=active (active), got %s hidden=%v", cfg.TaskWorkflows[0].Name, cfg.TaskWorkflows[0].Hidden)
	}
	if cfg.TaskWorkflows[1].Name != "hidden" || !cfg.TaskWorkflows[1].Hidden {
		t.Errorf("want second=hidden (hidden), got %s hidden=%v", cfg.TaskWorkflows[1].Name, cfg.TaskWorkflows[1].Hidden)
	}
}

func TestFileWorkflows_OrderPreservedFromListing(t *testing.T) {
	dir, _ := setupProject(t, `
workflows:
  tasks:
    - second
    - first
`, map[string]string{
		".sortie/workflows/tasks/first.yml":  "steps:\n  - name: a\n    prompt: a\n",
		".sortie/workflows/tasks/second.yml": "steps:\n  - name: b\n    prompt: b\n",
	})

	cfg, err := LoadForProject(dir)
	if err != nil {
		t.Fatalf("LoadForProject: %v", err)
	}
	if len(cfg.TaskWorkflows) != 2 {
		t.Fatalf("want 2 workflows, got %d", len(cfg.TaskWorkflows))
	}
	if cfg.TaskWorkflows[0].Name != "second" {
		t.Errorf("want first listed=second, got %s", cfg.TaskWorkflows[0].Name)
	}
	if cfg.TaskWorkflows[1].Name != "first" {
		t.Errorf("want second listed=first, got %s", cfg.TaskWorkflows[1].Name)
	}
}

func TestFileWorkflows_InvalidFilenameRejected(t *testing.T) {
	dir, _ := setupProject(t, `workflows:
  tasks: []
`, map[string]string{
		".sortie/workflows/tasks/Bad_Name.yml": "steps: []\n",
	})

	_, err := LoadForProject(dir)
	if err == nil || !strings.Contains(err.Error(), "invalid filename") {
		t.Fatalf("want invalid filename error, got %v", err)
	}
}

func TestFileWorkflows_NameFieldRejected(t *testing.T) {
	dir, _ := setupProject(t, `workflows:
  tasks:
    - foo
`, map[string]string{
		".sortie/workflows/tasks/foo.yml": `
name: explicit-name
steps:
  - name: a
    prompt: a
`,
	})

	_, err := LoadForProject(dir)
	if err == nil || !strings.Contains(err.Error(), "must not set `name:`") {
		t.Fatalf("want name-rejection error, got %v", err)
	}
}

func TestFileWorkflows_OneOffEnginePrefix(t *testing.T) {
	dir, _ := setupProject(t, `workflows:
  one-off:
    - cleanup
`, map[string]string{
		".sortie/workflows/one-off/cleanup.yml": `
description: Clean up artifacts
steps:
  - name: clean
    prompt: "clean"
`,
	})

	cfg, err := LoadForProject(dir)
	if err != nil {
		t.Fatalf("LoadForProject: %v", err)
	}
	if got := cfg.GetWorkflow("oneoff:cleanup"); got == nil {
		t.Fatalf("want one-off workflow under oneoff:cleanup, got nil")
	}
	// Hidden defaults to false for active workflows.
	if got := cfg.GetPredefinedTask("cleanup"); got == nil || got.Hidden {
		t.Errorf("want cleanup active, got %+v", got)
	}
}

func TestFileWorkflows_HiddenOneOffReachableViaListAll(t *testing.T) {
	dir, _ := setupProject(t, `workflows:
  one-off: []
`, map[string]string{
		".sortie/workflows/one-off/hidden-job.yml": `
steps:
  - name: a
    prompt: "a"
`,
	})

	cfg, err := LoadForProject(dir)
	if err != nil {
		t.Fatalf("LoadForProject: %v", err)
	}
	// Active list should be empty.
	if got := cfg.ListPredefinedTaskNames(); len(got) != 0 {
		t.Errorf("want no active one-offs, got %v", got)
	}
	// ListAll should include hidden one.
	all := cfg.ListAllPredefinedTaskNames()
	if len(all) != 1 || all[0] != "hidden-job" {
		t.Errorf("want [hidden-job] in ListAll, got %v", all)
	}
}

func TestFileWorkflows_YamlExtension(t *testing.T) {
	dir, _ := setupProject(t, `workflows:
  tasks:
    - alpha
`, map[string]string{
		".sortie/workflows/tasks/alpha.yaml": `
steps:
  - name: a
    prompt: a
`,
	})

	cfg, err := LoadForProject(dir)
	if err != nil {
		t.Fatalf("LoadForProject: %v", err)
	}
	if len(cfg.TaskWorkflows) != 1 || cfg.TaskWorkflows[0].Name != "alpha" {
		t.Errorf("want [alpha] from .yaml file, got %+v", cfg.TaskWorkflows)
	}
}

func TestFileWorkflows_SubdirectoryRejected(t *testing.T) {
	dir, _ := setupProject(t, `workflows:
  tasks: []
`, map[string]string{
		".sortie/workflows/tasks/sub/nested.yml": "steps: []\n",
	})

	_, err := LoadForProject(dir)
	if err == nil || !strings.Contains(err.Error(), "subdirector") {
		t.Fatalf("want subdirectory error, got %v", err)
	}
}

func TestFileWorkflows_MixedInlineAndRef(t *testing.T) {
	dir, _ := setupProject(t, `workflows:
  tasks:
    - file-based
    - name: inline-based
      steps:
        - name: a
          prompt: "a"
`, map[string]string{
		".sortie/workflows/tasks/file-based.yml": `
steps:
  - name: b
    prompt: "b"
`,
	})

	cfg, err := LoadForProject(dir)
	if err != nil {
		t.Fatalf("LoadForProject: %v", err)
	}
	if len(cfg.TaskWorkflows) != 2 {
		t.Fatalf("want 2 task workflows, got %d", len(cfg.TaskWorkflows))
	}
	if cfg.TaskWorkflows[0].Name != "file-based" {
		t.Errorf("want first=file-based, got %s", cfg.TaskWorkflows[0].Name)
	}
	if cfg.TaskWorkflows[1].Name != "inline-based" {
		t.Errorf("want second=inline-based, got %s", cfg.TaskWorkflows[1].Name)
	}
	if cfg.TaskWorkflows[0].Source == "inline" {
		t.Errorf("file-based should have file path source, got 'inline'")
	}
	if cfg.TaskWorkflows[1].Source != "inline" {
		t.Errorf("inline workflow should have source='inline', got %q", cfg.TaskWorkflows[1].Source)
	}
}

func TestDiagnose_WarnsForUnreferencedFile(t *testing.T) {
	_, ymlPath := setupProject(t, `workflows:
  tasks:
    - active
`, map[string]string{
		".sortie/workflows/tasks/active.yml": "steps:\n  - name: a\n    prompt: a\n",
		".sortie/workflows/tasks/hidden.yml": "steps:\n  - name: b\n    prompt: b\n",
	})

	diags, err := Diagnose(ymlPath)
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	found := false
	for _, d := range diags {
		if strings.Contains(d.Message, "hidden") && strings.Contains(d.Message, "hidden.yml") {
			found = true
		}
	}
	if !found {
		t.Errorf("want a 'hidden' diagnostic about hidden.yml, got %+v", diags)
	}
}

func TestDiagnose_WarnsForCategoryWithNoListing(t *testing.T) {
	// Files in init/ but no init: key in .sortie.yml — all init workflows hidden.
	_, ymlPath := setupProject(t, `workflows:
  tasks: []
`, map[string]string{
		".sortie/workflows/init/bootstrap.yml": "steps:\n  - name: a\n    prompt: a\n",
	})

	diags, err := Diagnose(ymlPath)
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	var got []string
	for _, d := range diags {
		got = append(got, d.Message)
	}
	found := false
	for _, msg := range got {
		if strings.Contains(msg, "workflows.init") && strings.Contains(msg, "no listing") {
			found = true
		}
	}
	if !found {
		t.Errorf("want diagnostic about workflows.init having no listing, got %v", got)
	}
}
