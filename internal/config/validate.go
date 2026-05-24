package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Diagnostic carries a non-fatal validation warning surfaced by ValidateFile
// (e.g. file-based workflow loaded but not referenced from .sortie.yml, which
// is legal but hides the workflow from menus).
type Diagnostic struct {
	Severity string // "warning"
	Message  string
}

// validOnCompleteValues lists the accepted git.on_complete values.
var validOnCompleteValues = map[string]bool{
	"":       true, // empty falls back to default
	"commit": true,
	"merge":  true,
	"none":   true,
}

// validTmuxNestedAttachBehaviors lists the accepted tmux_nested_attach_behavior values.
var validTmuxNestedAttachBehaviors = map[string]bool{
	"":       true, // empty falls back to default
	"switch": true,
	"nest":   true,
}

// validPriorities lists the accepted default_priority values.
var validPriorities = map[string]bool{
	"":       true, // empty falls back to default
	"low":    true,
	"medium": true,
	"high":   true,
	"urgent": true,
}

// ValidateFile validates a Sortie project config file (.sortie.yml) without
// touching the global merge hierarchy. It catches:
//   - YAML syntax errors
//   - Unknown top-level / nested fields (typos like `worktree_sync_paths`)
//   - Workflow loop misconfiguration (forward gotos, bad max_iterations, etc.)
//   - Invalid summarization strategies
//   - Invalid enum values (git.on_complete, default_priority, tmux_nested_attach_behavior)
//   - Duplicate workflow / step names
//   - File-based workflow errors (missing refs, inline/file collisions, bad filenames)
//
// ValidateFile also surfaces non-fatal warnings (e.g. unreferenced file-based
// workflows) — those are returned via Diagnose for callers that want to display
// them. ValidateFile's bool error return path only fires for fatal issues.
func ValidateFile(path string) error {
	_, err := Diagnose(path)
	return err
}

// Diagnose validates a Sortie project config file and returns any non-fatal
// warnings collected during validation. Fatal errors are returned via the
// error result.
func Diagnose(path string) ([]Diagnostic, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var proj ProjectConfig
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&proj); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	baseDir := filepath.Dir(path)
	filePool, err := loadWorkflowFilePool(baseDir)
	if err != nil {
		return nil, err
	}

	// Build the global workflow pool (resolved workflows from ~/.sortie.yml
	// and ~/.sortie/workflows/) so project configs that reference global
	// workflows by string ref validate cleanly. Skipped when the file under
	// validation IS the global config itself.
	globalPool, err := loadGlobalPoolForValidation(path)
	if err != nil {
		return nil, fmt.Errorf("load global config: %w", err)
	}

	return validateProject(&proj, filePool, globalPool)
}

// loadGlobalPoolForValidation resolves the global ~/.sortie.yml into a
// globalWorkflowPool. Returns nil (no global pool) when no global config
// exists or when skipPath matches the global path (avoiding self-recursion
// when the validation target IS the global config).
func loadGlobalPoolForValidation(skipPath string) (*globalWorkflowPool, error) {
	globalYml := getGlobalSortieYmlPath()
	if globalYml == "" {
		return nil, nil
	}
	// Resolve to absolute paths to avoid false negatives when skipPath was
	// passed as a relative path.
	absGlobal, err := filepath.Abs(globalYml)
	if err == nil {
		globalYml = absGlobal
	}
	absSkip, err := filepath.Abs(skipPath)
	if err == nil {
		skipPath = absSkip
	}
	if globalYml == skipPath {
		return nil, nil
	}

	tmpCfg := defaultConfig()
	if err := loadProjectConfig(globalYml, tmpCfg); err != nil {
		return nil, err
	}
	return snapshotGlobalPool(tmpCfg), nil
}

// validateProject runs structural validation on a parsed ProjectConfig.
// Returns any non-fatal warnings (e.g. unreferenced file-based workflows).
//
// globalPool, when non-nil, supplies workflows defined in the global
// ~/.sortie.yml so that project-level string refs to global workflows
// validate without surfacing "missing file" errors.
func validateProject(proj *ProjectConfig, filePool *workflowFilePool, globalPool *globalWorkflowPool) ([]Diagnostic, error) {
	// Enum sanity
	if !validOnCompleteValues[proj.Git.OnComplete] {
		return nil, fmt.Errorf(`git.on_complete: invalid value %q (must be "commit", "merge", or "none")`, proj.Git.OnComplete)
	}
	if !validTmuxNestedAttachBehaviors[proj.TmuxNestedAttachBehavior] {
		return nil, fmt.Errorf(`tmux_nested_attach_behavior: invalid value %q (must be "switch" or "nest")`, proj.TmuxNestedAttachBehavior)
	}
	if !validPriorities[proj.DefaultPriority] {
		return nil, fmt.Errorf(`default_priority: invalid value %q (must be "low", "medium", "high", or "urgent")`, proj.DefaultPriority)
	}

	// Capture pre-resolution file-pool state so we can detect categories with
	// files but no listing in .sortie.yml after resolution mutates the pool.
	preFiles := map[string][]string{}
	if filePool != nil {
		for _, cat := range workflowCategories {
			for _, name := range filePool.order[cat] {
				if _, ok := filePool.byCategory[cat][name]; ok {
					preFiles[cat] = append(preFiles[cat], name)
				}
			}
		}
	}

	// Reuse the production assembly path to apply identical validation rules
	// (loops, steps, summarization strategies) the daemon enforces at load time.
	cfg := defaultConfig()
	cfg.globalPool = globalPool
	if err := resolveWorkflows(cfg, proj, filePool); err != nil {
		return nil, err
	}

	if err := validateUniqueNames(cfg); err != nil {
		return nil, err
	}

	var diagnostics []Diagnostic

	// Surface a warning when a category has on-disk files but no listing in
	// .sortie.yml (all files become hidden — likely a user oversight).
	listed := map[string]bool{
		"tasks":   len(proj.Workflows.Tasks) > 0,
		"one-off": len(proj.Workflows.OneOff) > 0,
		"init":    len(proj.Workflows.Init) > 0,
	}
	for _, cat := range workflowCategories {
		if !listed[cat] && len(preFiles[cat]) > 0 {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: "warning",
				Message: fmt.Sprintf(
					"workflows.%s: %d file(s) under .sortie/workflows/%s/ are hidden because the category has no listing in .sortie.yml",
					cat, len(preFiles[cat]), cat),
			})
		}
	}

	// Surface a warning for each unreferenced file-based workflow (hidden).
	// Iterate the bucket slices so we report unprefixed names.
	for _, group := range [][]WorkflowConfig{cfg.TaskWorkflows, cfg.OneOff, cfg.InitWorkflows} {
		for _, wf := range group {
			if wf.Hidden {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: "warning",
					Message: fmt.Sprintf(
						"workflow %q from %s is hidden — add it to the relevant .sortie.yml listing to make it active",
						wf.Name, wf.Source),
				})
			}
		}
	}

	return diagnostics, nil
}

// validateUniqueNames ensures workflow names are unique within their category and
// that step names are unique within each workflow.
func validateUniqueNames(cfg *Config) error {
	if err := checkUniqueWorkflowNames("tasks", cfg.TaskWorkflows); err != nil {
		return err
	}
	if err := checkUniqueWorkflowNames("one-off", cfg.OneOff); err != nil {
		return err
	}
	if err := checkUniqueWorkflowNames("init", cfg.InitWorkflows); err != nil {
		return err
	}

	for _, wf := range cfg.Workflows {
		seen := make(map[string]bool, len(wf.Steps))
		for _, step := range wf.Steps {
			if step.Name == "" {
				return fmt.Errorf("workflow %q: step is missing a name", wf.Name)
			}
			if seen[step.Name] {
				return fmt.Errorf("workflow %q: duplicate step name %q", wf.Name, step.Name)
			}
			seen[step.Name] = true
		}
	}
	return nil
}

func checkUniqueWorkflowNames(category string, workflows []WorkflowConfig) error {
	seen := make(map[string]bool, len(workflows))
	for _, wf := range workflows {
		if seen[wf.Name] {
			return fmt.Errorf("workflows.%s: duplicate workflow name %q", category, wf.Name)
		}
		seen[wf.Name] = true
	}
	return nil
}
