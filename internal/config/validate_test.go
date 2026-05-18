package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempConfig(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".sortie.yml")
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestValidateFile_Valid(t *testing.T) {
	path := writeTempConfig(t, `
max_workers: 2
default_priority: high
git:
  branch_template: "sortie/{{task_id}}-{{task_slug}}"
  on_complete: commit
workflows:
  tasks:
    - name: default
      steps:
        - name: implementing
          prompt: "do the thing"
`)
	if err := ValidateFile(path); err != nil {
		t.Fatalf("expected valid config, got: %v", err)
	}
}

func TestValidateFile_MissingFile(t *testing.T) {
	err := ValidateFile(filepath.Join(t.TempDir(), "does-not-exist.yml"))
	if err == nil || !strings.Contains(err.Error(), "read config") {
		t.Fatalf("expected read config error, got: %v", err)
	}
}

func TestValidateFile_BadYAML(t *testing.T) {
	path := writeTempConfig(t, "max_workers: : :\n")
	err := ValidateFile(path)
	if err == nil || !strings.Contains(err.Error(), "parse yaml") {
		t.Fatalf("expected parse yaml error, got: %v", err)
	}
}

func TestValidateFile_UnknownTopLevelField(t *testing.T) {
	// The skill notes that `worktree_sync_paths` (snake_case) is a common typo
	// for the canonical `worktree-sync-paths` — strict parsing should flag it.
	path := writeTempConfig(t, "worktree_sync_paths:\n  copy: [.env]\n")
	err := ValidateFile(path)
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestValidateFile_InvalidOnComplete(t *testing.T) {
	path := writeTempConfig(t, "git:\n  on_complete: yeet\n")
	err := ValidateFile(path)
	if err == nil || !strings.Contains(err.Error(), "on_complete") {
		t.Fatalf("expected on_complete error, got: %v", err)
	}
}

func TestValidateFile_InvalidPriority(t *testing.T) {
	path := writeTempConfig(t, "default_priority: chill\n")
	err := ValidateFile(path)
	if err == nil || !strings.Contains(err.Error(), "default_priority") {
		t.Fatalf("expected default_priority error, got: %v", err)
	}
}

func TestValidateFile_InvalidTmuxNestedBehavior(t *testing.T) {
	path := writeTempConfig(t, "tmux_nested_attach_behavior: explode\n")
	err := ValidateFile(path)
	if err == nil || !strings.Contains(err.Error(), "tmux_nested_attach_behavior") {
		t.Fatalf("expected tmux_nested_attach_behavior error, got: %v", err)
	}
}

func TestValidateFile_ForwardLoopGoto(t *testing.T) {
	path := writeTempConfig(t, `
workflows:
  tasks:
    - name: bad
      steps:
        - name: a
          prompt: "a"
          loop:
            goto: b
            max_iterations: 2
        - name: b
          prompt: "b"
`)
	err := ValidateFile(path)
	if err == nil || !strings.Contains(err.Error(), "earlier step") {
		t.Fatalf("expected forward goto error, got: %v", err)
	}
}

func TestValidateFile_InvalidSummarizationStrategy(t *testing.T) {
	path := writeTempConfig(t, `
workflows:
  tasks:
    - name: bad
      steps:
        - name: a
          prompt: "a"
          summarization_strategy: telepathy
`)
	err := ValidateFile(path)
	if err == nil || !strings.Contains(err.Error(), "summarization_strategy") {
		t.Fatalf("expected summarization_strategy error, got: %v", err)
	}
}

func TestValidateFile_DuplicateWorkflowName(t *testing.T) {
	path := writeTempConfig(t, `
workflows:
  tasks:
    - name: dup
      steps:
        - name: a
          prompt: "a"
    - name: dup
      steps:
        - name: a
          prompt: "a"
`)
	err := ValidateFile(path)
	if err == nil || !strings.Contains(err.Error(), "duplicate workflow") {
		t.Fatalf("expected duplicate workflow error, got: %v", err)
	}
}

func TestValidateFile_LegacyWorkflowTmuxFieldFails(t *testing.T) {
	// The legacy `tmux:` field at the workflow level was removed in favour of
	// the inverted `print:` field; loading an old config should surface a clear
	// migration error rather than silently dropping the setting.
	path := writeTempConfig(t, `
workflows:
  tasks:
    - name: w
      tmux: true
      steps:
        - name: s
          prompt: "do"
`)
	err := ValidateFile(path)
	if err == nil || !strings.Contains(err.Error(), "tmux") || !strings.Contains(err.Error(), "print") {
		t.Fatalf("expected migration error mentioning tmux and print, got: %v", err)
	}
}

func TestValidateFile_LegacyStepTmuxFieldFails(t *testing.T) {
	// The legacy step-level `tmux:` field is also removed; surface the same
	// migration guidance from the step decoder.
	path := writeTempConfig(t, `
workflows:
  tasks:
    - name: w
      steps:
        - name: s
          prompt: "do"
          tmux: false
`)
	err := ValidateFile(path)
	if err == nil || !strings.Contains(err.Error(), "tmux") || !strings.Contains(err.Error(), "print") {
		t.Fatalf("expected migration error mentioning tmux and print, got: %v", err)
	}
}

func TestValidateFile_MissingFileRefIsError(t *testing.T) {
	// String ref to a non-existent file should be a hard error.
	dir := t.TempDir()
	path := filepath.Join(dir, ".sortie.yml")
	if err := os.WriteFile(path, []byte(`
workflows:
  tasks:
    - phantom
`), 0644); err != nil {
		t.Fatal(err)
	}
	err := ValidateFile(path)
	if err == nil || !strings.Contains(err.Error(), "phantom") {
		t.Fatalf("expected missing-ref error, got %v", err)
	}
}

func TestValidateFile_InlineFileCollisionIsError(t *testing.T) {
	dir := t.TempDir()
	wf := filepath.Join(dir, ".sortie", "workflows", "tasks")
	if err := os.MkdirAll(wf, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wf, "foo.yml"), []byte("steps:\n  - name: a\n    prompt: a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ymlPath := filepath.Join(dir, ".sortie.yml")
	if err := os.WriteFile(ymlPath, []byte(`
workflows:
  tasks:
    - name: foo
      steps:
        - name: a
          prompt: "a"
`), 0644); err != nil {
		t.Fatal(err)
	}
	err := ValidateFile(ymlPath)
	if err == nil || !strings.Contains(err.Error(), "collides") {
		t.Fatalf("expected collision error, got %v", err)
	}
}

func TestValidateFile_DuplicateStepName(t *testing.T) {
	path := writeTempConfig(t, `
workflows:
  tasks:
    - name: w
      steps:
        - name: a
          prompt: "a"
        - name: a
          prompt: "a"
`)
	err := ValidateFile(path)
	if err == nil || !strings.Contains(err.Error(), "duplicate step") {
		t.Fatalf("expected duplicate step error, got: %v", err)
	}
}
