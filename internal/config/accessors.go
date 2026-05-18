package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// GetStepTimeout parses the step's timeout string or returns the default.
func (c *Config) GetStepTimeout(step StepConfig) time.Duration {
	if step.Timeout != "" {
		if d, err := time.ParseDuration(step.Timeout); err == nil {
			return d
		}
	}
	return DefaultStepTimeout
}

// GetWorkflow returns the workflow with the given name.
// If name is empty and there are workflows, returns the first one.
// If the workflow is not found, returns the default workflow.
func (c *Config) GetWorkflow(name string) *WorkflowConfig {
	for i := range c.Workflows {
		if c.Workflows[i].Name == name {
			return &c.Workflows[i]
		}
	}
	// If name is empty and there are workflows, return first
	if name == "" && len(c.Workflows) > 0 {
		return &c.Workflows[0]
	}
	// Return default
	def := DefaultWorkflow()
	return &def
}

// ListWorkflowNames returns the names of active (non-hidden) task workflows
// (the "tasks" kind used for new task creation menus). If none configured,
// returns ["default"]. Use ListAllWorkflowNames for picker surfaces that
// should expose hidden workflows too.
func (c *Config) ListWorkflowNames() []string {
	return c.listTaskNames(false)
}

// ListAllWorkflowNames returns the names of all task workflows including
// hidden ones. Used by pickers and tab completion where every reachable
// workflow should be selectable.
func (c *Config) ListAllWorkflowNames() []string {
	return c.listTaskNames(true)
}

func (c *Config) listTaskNames(includeHidden bool) []string {
	if len(c.TaskWorkflows) == 0 {
		return []string{"default"}
	}
	names := make([]string, 0, len(c.TaskWorkflows))
	for _, w := range c.TaskWorkflows {
		if !includeHidden && w.Hidden {
			continue
		}
		names = append(names, w.Name)
	}
	if len(names) == 0 && !includeHidden {
		// All workflows hidden — fall back to default so menu surfaces still
		// have something to display.
		return []string{"default"}
	}
	return names
}

// ListPredefinedTaskNames returns the names of active one-off workflows
// (the "one-off" kind shown in the "x" / :RunOneOff menu).
func (c *Config) ListPredefinedTaskNames() []string {
	return c.listOneOffNames(false)
}

// ListAllPredefinedTaskNames returns the names of all one-off workflows
// including hidden ones. Used by the :RunOneOff picker.
func (c *Config) ListAllPredefinedTaskNames() []string {
	return c.listOneOffNames(true)
}

func (c *Config) listOneOffNames(includeHidden bool) []string {
	names := make([]string, 0, len(c.OneOff))
	for _, t := range c.OneOff {
		if !includeHidden && t.Hidden {
			continue
		}
		names = append(names, t.Name)
	}
	return names
}

// GetPredefinedTask returns the one-off workflow config with the given name, or nil.
// Returns hidden workflows too — callers wanting active-only must filter.
func (c *Config) GetPredefinedTask(name string) *WorkflowConfig {
	for i := range c.OneOff {
		if c.OneOff[i].Name == name {
			return &c.OneOff[i]
		}
	}
	return nil
}

// ListInitWorkflowNames returns the names of active init workflows
// (the "init" kind shown in the "i" / :RunInit menu).
func (c *Config) ListInitWorkflowNames() []string {
	return c.listInitNames(false)
}

// ListAllInitWorkflowNames returns the names of all init workflows including
// hidden ones. Used by the :RunInit picker.
func (c *Config) ListAllInitWorkflowNames() []string {
	return c.listInitNames(true)
}

func (c *Config) listInitNames(includeHidden bool) []string {
	names := make([]string, 0, len(c.InitWorkflows))
	for _, w := range c.InitWorkflows {
		if !includeHidden && w.Hidden {
			continue
		}
		names = append(names, w.Name)
	}
	return names
}

// GetInitWorkflow returns the init workflow config with the given name, or nil.
// Returns hidden workflows too — callers wanting active-only must filter.
func (c *Config) GetInitWorkflow(name string) *WorkflowConfig {
	for i := range c.InitWorkflows {
		if c.InitWorkflows[i].Name == name {
			return &c.InitWorkflows[i]
		}
	}
	return nil
}

// GetTaskWorkflow returns the task-category workflow config with the given
// name, or nil. Returns hidden workflows too.
func (c *Config) GetTaskWorkflow(name string) *WorkflowConfig {
	for i := range c.TaskWorkflows {
		if c.TaskWorkflows[i].Name == name {
			return &c.TaskWorkflows[i]
		}
	}
	return nil
}

// GetWorktreeSyncPaths returns the sync paths for a workflow, falling back to the global config.
func (c *Config) GetWorktreeSyncPaths(wf *WorkflowConfig) WorktreeSyncPathsConfig {
	if wf != nil && !wf.WorktreeSyncPaths.IsEmpty() {
		return wf.WorktreeSyncPaths
	}
	return c.WorktreeSyncPaths
}

// GetWorktreeSetupCommand returns the setup command for a workflow, falling back to the global config.
func (c *Config) GetWorktreeSetupCommand(wf *WorkflowConfig) string {
	if wf != nil && wf.WorktreeSetupCommand != "" {
		return wf.WorktreeSetupCommand
	}
	return c.WorktreeSetupCommand
}

// GetWorktreeSetupCommands returns the setup commands for a workflow, falling back to the global config.
func (c *Config) GetWorktreeSetupCommands(wf *WorkflowConfig) []string {
	if wf != nil && len(wf.WorktreeSetupCommands) > 0 {
		return wf.WorktreeSetupCommands
	}
	return c.WorktreeSetupCommands
}

// GetTmuxSetupCommand returns the tmux setup command for a workflow, falling back to the global config.
func (c *Config) GetTmuxSetupCommand(wf *WorkflowConfig) string {
	if wf != nil && wf.TmuxSetupCommand != "" {
		return wf.TmuxSetupCommand
	}
	return c.TmuxSetupCommand
}

// ResolveBranchForTask resolves the branch name for a task. If branchName is non-empty,
// it uses ResolveBranchTemplate; otherwise falls back to the config's branch template.
func (c *Config) ResolveBranchForTask(taskID int64, taskTitle, taskSlug, branchName string) string {
	if branchName != "" {
		return ResolveBranchTemplate(branchName, taskID, taskTitle, taskSlug)
	}
	return c.ResolveBranchName(taskID, taskSlug)
}

// ResolveBranchName applies the branch template for a task.
func (c *Config) ResolveBranchName(taskID int64, taskSlug string) string {
	tmpl := c.Git.BranchTemplate
	tmpl = strings.ReplaceAll(tmpl, "{{task_id}}", fmt.Sprintf("%d", taskID))
	tmpl = strings.ReplaceAll(tmpl, "{{task_slug}}", taskSlug)
	return tmpl
}

// ResolveBranchTemplate resolves a branch name template with task variables.
// Supports both config-style ({{task_id}}, {{task_slug}}) and workflow-style
// ({{task.title}}, {{task.id}}) placeholders.
func ResolveBranchTemplate(tmpl string, taskID int64, taskTitle, taskSlug string) string {
	// Config-style placeholders
	tmpl = strings.ReplaceAll(tmpl, "{{task_id}}", fmt.Sprintf("%d", taskID))
	tmpl = strings.ReplaceAll(tmpl, "{{task_slug}}", taskSlug)

	// Workflow-style placeholders
	tmpl = strings.ReplaceAll(tmpl, "{{task.id}}", fmt.Sprintf("%d", taskID))
	tmpl = strings.ReplaceAll(tmpl, "{{task.title}}", taskTitle)
	tmpl = strings.ReplaceAll(tmpl, "{{task.slug}}", taskSlug)

	return tmpl
}

// WriteProjectConfig writes a .sortie.yml file.
func WriteProjectConfig(path string, proj *ProjectConfig) error {
	data, err := yaml.Marshal(proj)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (c *Config) Save() error {
	configPath := getGlobalConfigPath()
	if configPath == "" {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
	}

	yolo := c.Claude.Yolo
	global := GlobalConfig{
		MaxWorkers:               c.MaxWorkers,
		Yolo:                     &yolo,
		Notifications:            c.Notifications,
		TmuxNestedAttachBehavior: c.TmuxNestedAttachBehavior,
	}

	data, err := yaml.Marshal(global)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}
