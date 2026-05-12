package config

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

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
func ValidateFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	var proj ProjectConfig
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&proj); err != nil {
		return fmt.Errorf("parse yaml: %w", err)
	}

	return validateProject(&proj)
}

// validateProject runs structural validation on a parsed ProjectConfig.
func validateProject(proj *ProjectConfig) error {
	// Enum sanity
	if !validOnCompleteValues[proj.Git.OnComplete] {
		return fmt.Errorf(`git.on_complete: invalid value %q (must be "commit", "merge", or "none")`, proj.Git.OnComplete)
	}
	if !validTmuxNestedAttachBehaviors[proj.TmuxNestedAttachBehavior] {
		return fmt.Errorf(`tmux_nested_attach_behavior: invalid value %q (must be "switch" or "nest")`, proj.TmuxNestedAttachBehavior)
	}
	if !validPriorities[proj.DefaultPriority] {
		return fmt.Errorf(`default_priority: invalid value %q (must be "low", "medium", "high", or "urgent")`, proj.DefaultPriority)
	}

	// Reuse the production assembly path to apply identical validation rules
	// (loops, steps, summarization strategies) the daemon enforces at load time.
	cfg := defaultConfig()
	if err := resolveWorkflows(cfg, proj); err != nil {
		return err
	}

	if err := validateUniqueNames(cfg); err != nil {
		return err
	}

	return nil
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
