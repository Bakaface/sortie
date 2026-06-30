package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupGlobalAndProject writes a global ~/.sortie.yml and an optional
// ~/.sortie/workflows/<name>.yml tree under an isolated HOME, plus a
// project .sortie.yml (with optional .sortie/workflows files) under a separate
// project directory. Returns the project directory.
//
// The test's HOME and XDG_CONFIG_HOME are pointed at a temp dir so that
// LoadForProject picks up only the synthesized global config.
func setupGlobalAndProject(
	t *testing.T,
	globalYml string,
	globalFiles map[string]string,
	projectYml string,
	projectFiles map[string]string,
) string {
	t.Helper()

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	// Force XDG_CONFIG_HOME to a fresh empty dir to silence config.yaml lookup
	// and avoid the user's real ~/.config/sortie/ leaking in.
	xdgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgDir)

	if globalYml != "" {
		globalPath := filepath.Join(homeDir, ".sortie.yml")
		if err := os.WriteFile(globalPath, []byte(globalYml), 0644); err != nil {
			t.Fatalf("write global .sortie.yml: %v", err)
		}
	}
	for rel, content := range globalFiles {
		full := filepath.Join(homeDir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	projectDir := t.TempDir()
	projectPath := filepath.Join(projectDir, ".sortie.yml")
	if err := os.WriteFile(projectPath, []byte(projectYml), 0644); err != nil {
		t.Fatalf("write project .sortie.yml: %v", err)
	}
	for rel, content := range projectFiles {
		full := filepath.Join(projectDir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	return projectDir
}

// Example 1: global defines an *inline* workflow, project references it via
// string ref just as it would reference a workflow file under .sortie/workflows/.
func TestGlobalWorkflows_ProjectReferencesGlobalInline(t *testing.T) {
	projectDir := setupGlobalAndProject(t,
		`workflows:
  - name: shared-impl
    description: Globally-defined inline workflow
    steps:
      - name: do
        prompt: "global inline implementation"
`,
		nil,
		`workflows:
  - shared-impl
`,
		nil,
	)

	cfg, err := LoadForProject(projectDir)
	if err != nil {
		t.Fatalf("LoadForProject: %v", err)
	}

	if len(cfg.Workflows) != 1 {
		t.Fatalf("want 1 workflow, got %d: %+v", len(cfg.Workflows), cfg.Workflows)
	}
	wf := cfg.Workflows[0]
	if wf.Name != "shared-impl" {
		t.Errorf("want name shared-impl, got %q", wf.Name)
	}
	if wf.Description != "Globally-defined inline workflow" {
		t.Errorf("want global description, got %q", wf.Description)
	}
	if len(wf.Steps) != 1 || wf.Steps[0].Prompt != "global inline implementation" {
		t.Errorf("want global step prompt, got %+v", wf.Steps)
	}
	if wf.Hidden {
		t.Errorf("referenced workflow should not be hidden")
	}
}

// Example 2: global defines a file workflow under ~/.sortie/workflows/, project
// references it via string ref just as it would reference a project-local file.
func TestGlobalWorkflows_ProjectReferencesGlobalFile(t *testing.T) {
	projectDir := setupGlobalAndProject(t,
		// Global .sortie.yml is empty so its file-pool workflow becomes
		// "hidden" globally — but project can still reference it by name.
		`# global file pool workflow exists but isn't listed; project can still reference it.
`,
		map[string]string{
			".sortie/workflows/global-file-impl.yml": `
description: Globally-defined file workflow
steps:
  - name: do
    prompt: "global file implementation"
`,
		},
		`workflows:
  - global-file-impl
`,
		nil,
	)

	cfg, err := LoadForProject(projectDir)
	if err != nil {
		t.Fatalf("LoadForProject: %v", err)
	}

	if len(cfg.Workflows) != 1 {
		t.Fatalf("want 1 workflow, got %d: %+v", len(cfg.Workflows), cfg.Workflows)
	}
	wf := cfg.Workflows[0]
	if wf.Name != "global-file-impl" {
		t.Errorf("want name global-file-impl, got %q", wf.Name)
	}
	if wf.Description != "Globally-defined file workflow" {
		t.Errorf("want global description, got %q", wf.Description)
	}
	if len(wf.Steps) != 1 || wf.Steps[0].Prompt != "global file implementation" {
		t.Errorf("want global file step prompt, got %+v", wf.Steps)
	}
	if wf.Hidden {
		t.Errorf("project-referenced workflow should be active, not hidden")
	}
	// Source should point at the global file path so users can trace where the
	// definition came from.
	if !strings.HasSuffix(wf.Source, "global-file-impl.yml") {
		t.Errorf("want source ending in global-file-impl.yml, got %q", wf.Source)
	}
}

// Example 3a: global defines a workflow (inline), project overrides it inline.
func TestGlobalWorkflows_ProjectOverridesGlobalInlineWithInline(t *testing.T) {
	projectDir := setupGlobalAndProject(t,
		`workflows:
  - name: shared-impl
    steps:
      - name: do
        prompt: "global version"
`,
		nil,
		`workflows:
  - name: shared-impl
    steps:
      - name: do
        prompt: "project override"
`,
		nil,
	)

	cfg, err := LoadForProject(projectDir)
	if err != nil {
		t.Fatalf("LoadForProject: %v", err)
	}
	if len(cfg.Workflows) != 1 {
		t.Fatalf("want 1 workflow, got %d", len(cfg.Workflows))
	}
	got := cfg.Workflows[0]
	if got.Steps[0].Prompt != "project override" {
		t.Errorf("want project override prompt, got %q", got.Steps[0].Prompt)
	}
	if got.Source != "inline" {
		t.Errorf("want source=inline (project's), got %q", got.Source)
	}
}

// Example 3b: global defines a workflow (inline), project overrides it via a
// project-local file under .sortie/workflows/ + a string ref to that name.
func TestGlobalWorkflows_ProjectOverridesGlobalInlineWithLocalFile(t *testing.T) {
	projectDir := setupGlobalAndProject(t,
		`workflows:
  - name: shared-impl
    steps:
      - name: do
        prompt: "global version"
`,
		nil,
		`workflows:
  - shared-impl
`,
		map[string]string{
			".sortie/workflows/shared-impl.yml": `
steps:
  - name: do
    prompt: "project file override"
`,
		},
	)

	cfg, err := LoadForProject(projectDir)
	if err != nil {
		t.Fatalf("LoadForProject: %v", err)
	}
	if len(cfg.Workflows) != 1 {
		t.Fatalf("want 1 workflow, got %d", len(cfg.Workflows))
	}
	got := cfg.Workflows[0]
	if got.Steps[0].Prompt != "project file override" {
		t.Errorf("want project file override prompt, got %q", got.Steps[0].Prompt)
	}
	if !strings.HasSuffix(got.Source, "shared-impl.yml") || strings.Contains(got.Source, "/.sortie.yml") {
		t.Errorf("want project file source, got %q", got.Source)
	}
	if strings.HasPrefix(got.Source, os.Getenv("HOME")+"/.sortie/") {
		t.Errorf("source should be the project-local file, got global path %q", got.Source)
	}
}

// Example 3c: global defines a workflow in its file pool (under
// ~/.sortie/workflows/), project overrides it inline.
func TestGlobalWorkflows_ProjectOverridesGlobalFileWithInline(t *testing.T) {
	projectDir := setupGlobalAndProject(t,
		`workflows:
  - shared-impl
`,
		map[string]string{
			".sortie/workflows/shared-impl.yml": `
steps:
  - name: do
    prompt: "global file version"
`,
		},
		`workflows:
  - name: shared-impl
    steps:
      - name: do
        prompt: "project inline override"
`,
		nil,
	)

	cfg, err := LoadForProject(projectDir)
	if err != nil {
		t.Fatalf("LoadForProject: %v", err)
	}
	if len(cfg.Workflows) != 1 {
		t.Fatalf("want 1 workflow, got %d", len(cfg.Workflows))
	}
	got := cfg.Workflows[0]
	if got.Steps[0].Prompt != "project inline override" {
		t.Errorf("want project inline override prompt, got %q", got.Steps[0].Prompt)
	}
	if got.Source != "inline" {
		t.Errorf("want source=inline, got %q", got.Source)
	}
}

// Mixing: project references a global workflow alongside a project-local file
// in the same listing. Both should resolve.
func TestGlobalWorkflows_MixedGlobalAndLocalRefs(t *testing.T) {
	projectDir := setupGlobalAndProject(t,
		`workflows:
  - name: from-global
    steps:
      - name: do
        prompt: "from global"
`,
		nil,
		`workflows:
  - from-global
  - from-local
`,
		map[string]string{
			".sortie/workflows/from-local.yml": `
steps:
  - name: do
    prompt: "from local"
`,
		},
	)

	cfg, err := LoadForProject(projectDir)
	if err != nil {
		t.Fatalf("LoadForProject: %v", err)
	}
	if len(cfg.Workflows) != 2 {
		t.Fatalf("want 2 workflows, got %d", len(cfg.Workflows))
	}
	if cfg.Workflows[0].Name != "from-global" {
		t.Errorf("want first=from-global, got %q", cfg.Workflows[0].Name)
	}
	if cfg.Workflows[1].Name != "from-local" {
		t.Errorf("want second=from-local, got %q", cfg.Workflows[1].Name)
	}
}

// Unreferenced global workflows should not appear in the project's listing
// when the project defines its own workflows. They only flow in when explicitly
// referenced.
func TestGlobalWorkflows_UnreferencedGlobalNotIncluded(t *testing.T) {
	projectDir := setupGlobalAndProject(t,
		`workflows:
  - name: g1
    steps: [{ name: a, prompt: a }]
  - name: g2
    steps: [{ name: a, prompt: a }]
`,
		nil,
		`workflows:
  - g1
`,
		nil,
	)

	cfg, err := LoadForProject(projectDir)
	if err != nil {
		t.Fatalf("LoadForProject: %v", err)
	}
	// Only g1 explicitly referenced; g2 not included.
	nonHidden := 0
	for _, wf := range cfg.Workflows {
		if !wf.Hidden {
			nonHidden++
		}
	}
	if nonHidden != 1 {
		t.Fatalf("want 1 active workflow (g1 only), got %d active", nonHidden)
	}
	if cfg.Workflows[0].Name != "g1" {
		t.Errorf("want g1, got %q", cfg.Workflows[0].Name)
	}
}

// Missing string ref that exists neither in project nor global is a hard error.
func TestGlobalWorkflows_MissingRefAcrossBoth(t *testing.T) {
	projectDir := setupGlobalAndProject(t,
		`workflows:
  - name: g1
    steps: [{ name: a, prompt: a }]
`,
		nil,
		`workflows:
  - g1
  - missing
`,
		nil,
	)

	_, err := LoadForProject(projectDir)
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("want error about missing workflow, got %v", err)
	}
}

// Flat global workflows should support the same global pool references.
func TestGlobalWorkflows_FlatGlobalAndProjectRefs(t *testing.T) {
	projectDir := setupGlobalAndProject(t,
		`workflows:
  - name: cleanup
    steps: [{ name: a, prompt: "global cleanup" }]
  - name: bootstrap
    steps: [{ name: a, prompt: "global bootstrap" }]
`,
		nil,
		`workflows:
  - cleanup
  - bootstrap
`,
		nil,
	)

	cfg, err := LoadForProject(projectDir)
	if err != nil {
		t.Fatalf("LoadForProject: %v", err)
	}
	if len(cfg.Workflows) != 2 {
		t.Fatalf("want 2 workflows, got %d", len(cfg.Workflows))
	}

	// Both should be resolvable via GetWorkflow
	cleanup := cfg.GetWorkflow("cleanup")
	if cleanup == nil || cleanup.Steps[0].Prompt != "global cleanup" {
		t.Errorf("want cleanup resolved from global, got %+v", cleanup)
	}
	bootstrap := cfg.GetWorkflow("bootstrap")
	if bootstrap == nil || bootstrap.Steps[0].Prompt != "global bootstrap" {
		t.Errorf("want bootstrap resolved from global, got %+v", bootstrap)
	}
}

// Diagnose should pick up the global pool so that project configs referencing
// global workflows validate cleanly (no false-positive "missing file" error).
func TestDiagnose_ResolvesGlobalRefs(t *testing.T) {
	projectDir := setupGlobalAndProject(t,
		`workflows:
  - name: shared-impl
    steps:
      - name: do
        prompt: "global"
`,
		nil,
		`workflows:
  - shared-impl
`,
		nil,
	)

	diags, err := Diagnose(filepath.Join(projectDir, ".sortie.yml"))
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	for _, d := range diags {
		if strings.Contains(d.Message, "shared-impl") {
			t.Errorf("did not expect diagnostic about shared-impl, got %q", d.Message)
		}
	}
}
