package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ProjectConfig is loaded from .sortie.yml at repo root
type ProjectConfig struct {
	MaxWorkers      int              `yaml:"max_workers"`
	DefaultPriority string           `yaml:"default_priority"`
	Git             GitConfig        `yaml:"git"`
	Workflows       []WorkflowConfig `yaml:"workflows"`
	Workflow        WorkflowConfig   `yaml:"workflow"` // deprecated, backward compat
	Tasks           []TaskConfig     `yaml:"tasks"`
}

// TaskConfig defines a predefined task with a built-in description and workflow steps.
// Unlike workflows, predefined tasks don't require the user to enter a prompt.
type TaskConfig struct {
	Name             string       `yaml:"name"`
	Description      string       `yaml:"description"`
	Steps            []StepConfig `yaml:"steps"`
	SummarizerPrompt string       `yaml:"summarizer_prompt"`
}

type GitConfig struct {
	BaseBranch     string `yaml:"base_branch"`
	BranchTemplate string `yaml:"branch_template"`
	OnComplete     string `yaml:"on_complete"`
}

type WorkflowConfig struct {
	Name             string       `yaml:"name"`
	Tmux             bool         `yaml:"tmux,omitempty"`
	Steps            []StepConfig `yaml:"steps"`
	SummarizerPrompt string       `yaml:"summarizer_prompt"`
}

type StepConfig struct {
	Name             string `yaml:"name"`
	Prompt           string `yaml:"prompt"`
	Mode             string `yaml:"mode"`
	Tmux             *bool  `yaml:"tmux,omitempty"`
	Timeout          string `yaml:"timeout"`
	Human            bool   `yaml:"human"`
	Artifact         bool   `yaml:"artifact"`
}

// UseTmux returns whether this step should use tmux execution.
// Step-level setting overrides the workflow default.
func (s *StepConfig) UseTmux(workflowDefault bool) bool {
	if s.Tmux != nil {
		return *s.Tmux
	}
	return workflowDefault
}

const DefaultStepTimeout = 30 * time.Minute

// GetStepTimeout parses the step's timeout string or returns the default.
func (c *Config) GetStepTimeout(step StepConfig) time.Duration {
	if step.Timeout != "" {
		if d, err := time.ParseDuration(step.Timeout); err == nil {
			return d
		}
	}
	return DefaultStepTimeout
}

// GlobalConfig from ~/.config/sortie/config.yaml
type GlobalConfig struct {
	MaxWorkers               int                 `yaml:"max_workers"`
	Notifications            NotificationsConfig `yaml:"notifications"`
	TmuxNestedAttachBehavior string              `yaml:"tmux_nested_attach_behavior"`
}

type NotificationsConfig struct {
	Enabled        bool `yaml:"enabled"`
	OnComplete     bool `yaml:"on_complete"`
	OnFailed       bool `yaml:"on_failed"`
	OnWaitingInput bool `yaml:"on_waiting_input"`
}

type ClaudeConfig struct {
	Command     string   `yaml:"command"`
	DefaultArgs []string `yaml:"default_args"`
}

// CommandsConfig is used during init for project detection
type CommandsConfig struct {
	Test  string `yaml:"test"`
	Lint  string `yaml:"lint"`
	Build string `yaml:"build"`
}

// Config is the merged runtime config used by all components.
// It combines global config, project config (.sortie.yml), and computed paths.
type Config struct {
	// From .sortie.yml (project config)
	MaxWorkers      int
	DefaultPriority string
	Git             GitConfig
	Workflows       []WorkflowConfig
	Tasks           []TaskConfig

	// From global config
	Notifications            NotificationsConfig
	TmuxNestedAttachBehavior string // "switch" (default) or "nest"

	// Internal defaults (not in yaml)
	Claude ClaudeConfig

	// Whether a project config (.sortie.yml) was found
	ProjectConfigFound bool

	// Computed paths (project-local)
	ProjectDir   string
	DatabasePath string
	SocketPath   string
	PidFile      string
	PollInterval time.Duration

	// Legacy compat fields used by detect.go during init
	Project struct {
		Name       string
		WorkDir    string
		Commands   CommandsConfig
		AutoDetect bool
	}

	// Legacy compat — these wrap into the new struct for existing consumers
	Daemon   daemonCompat
	Database databaseCompat
	Agents   agentsCompat
}

// daemonCompat provides backward-compatible access for daemon package
type daemonCompat struct {
	SocketPath   string
	PidFile      string
	PollInterval time.Duration
}

type databaseCompat struct {
	Path string
}

type agentsCompat struct {
	MaxConcurrent     int
	OutputBufferLines int
	MaxRetries        int
}

func defaultConfig() *Config {
	return &Config{
		MaxWorkers: 3,
		Git: GitConfig{
			BranchTemplate: "sortie/{{task_id}}-{{task_slug}}",
			OnComplete:     "commit",
		},
		Workflows: nil, // Empty - DefaultWorkflow() handles fallback
		Notifications: NotificationsConfig{
			Enabled:        true,
			OnComplete:     true,
			OnFailed:       true,
			OnWaitingInput: true,
		},
		Claude: ClaudeConfig{
			Command:     "claude",
			DefaultArgs: []string{"--dangerously-skip-permissions"},
		},
		PollInterval: 5 * time.Second,
		Agents: agentsCompat{
			MaxConcurrent:     3,
			OutputBufferLines: 10000,
			MaxRetries:        3,
		},
	}
}

// DefaultWorkflow returns the single-step default workflow when no workflow is configured
func DefaultWorkflow() WorkflowConfig {
	return WorkflowConfig{
		Name: "default",
		Steps: []StepConfig{
			{
				Name:   "implementing",
				Prompt: "Implement the task described in this worktree's CLAUDE.md",
				Mode:   "automatic",
			},
		},
	}
}

// DefaultWorkflowSteps returns the single-step default when no workflow is configured
// Kept for backward compatibility - delegates to DefaultWorkflow().Steps
func DefaultWorkflowSteps() []StepConfig {
	return DefaultWorkflow().Steps
}

// Load loads config from global + project .sortie.yml, returning a merged Config.
func Load() (*Config, error) {
	cfg := defaultConfig()

	// Load global config
	globalPath := getGlobalConfigPath()
	if globalPath != "" {
		if err := loadGlobalConfig(globalPath, cfg); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}

	// Load project config (.sortie.yml at repo root)
	projectPath := getProjectConfigPath()
	if projectPath != "" {
		if err := loadProjectConfig(projectPath, cfg); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		cfg.ProjectDir = filepath.Dir(projectPath)
		cfg.ProjectConfigFound = true
	}

	cfg.computePaths()
	cfg.syncCompat()

	return cfg, nil
}

// LoadForProject loads config for a specific project directory.
func LoadForProject(projectDir string) (*Config, error) {
	cfg := defaultConfig()

	globalPath := getGlobalConfigPath()
	if globalPath != "" {
		if err := loadGlobalConfig(globalPath, cfg); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}

	projectPath := filepath.Join(projectDir, ".sortie.yml")
	if _, err := os.Stat(projectPath); err == nil {
		if err := loadProjectConfig(projectPath, cfg); err != nil {
			return nil, err
		}
		cfg.ProjectConfigFound = true
	}

	cfg.ProjectDir = projectDir
	cfg.computePaths()
	cfg.syncCompat()

	return cfg, nil
}

func loadGlobalConfig(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var global GlobalConfig
	if err := yaml.Unmarshal(data, &global); err != nil {
		return err
	}

	if global.MaxWorkers > 0 {
		cfg.MaxWorkers = global.MaxWorkers
	}
	cfg.Notifications = global.Notifications
	if global.TmuxNestedAttachBehavior != "" {
		cfg.TmuxNestedAttachBehavior = global.TmuxNestedAttachBehavior
	}

	return nil
}

func loadProjectConfig(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var proj ProjectConfig
	if err := yaml.Unmarshal(data, &proj); err != nil {
		return err
	}

	if proj.MaxWorkers > 0 {
		cfg.MaxWorkers = proj.MaxWorkers
	}
	if proj.Git.BaseBranch != "" {
		cfg.Git.BaseBranch = proj.Git.BaseBranch
	}
	if proj.Git.BranchTemplate != "" {
		cfg.Git.BranchTemplate = proj.Git.BranchTemplate
	}
	if proj.Git.OnComplete != "" {
		cfg.Git.OnComplete = proj.Git.OnComplete
	}
	if proj.DefaultPriority != "" {
		cfg.DefaultPriority = proj.DefaultPriority
	}

	// Handle workflows: prefer new plural form, fall back to old singular
	if len(proj.Workflows) > 0 {
		cfg.Workflows = proj.Workflows
	} else if len(proj.Workflow.Steps) > 0 {
		// Backward compat: convert old singular workflow to workflows slice
		cfg.Workflows = []WorkflowConfig{proj.Workflow}
	}

	// Ensure all workflows have names
	for i := range cfg.Workflows {
		if cfg.Workflows[i].Name == "" {
			if i == 0 {
				cfg.Workflows[i].Name = "default"
			} else {
				cfg.Workflows[i].Name = fmt.Sprintf("workflow-%d", i+1)
			}
		}
	}

	// Load predefined tasks and register their steps as synthetic workflows
	if len(proj.Tasks) > 0 {
		cfg.Tasks = proj.Tasks
		for i, task := range cfg.Tasks {
			if task.Name == "" {
				cfg.Tasks[i].Name = fmt.Sprintf("task-%d", i+1)
			}
			// Register as a workflow so the engine can resolve it
			wfName := "task:" + cfg.Tasks[i].Name
			cfg.Workflows = append(cfg.Workflows, WorkflowConfig{
				Name:             wfName,
				Steps:            task.Steps,
				SummarizerPrompt: task.SummarizerPrompt,
			})
		}
	}

	return nil
}

func (c *Config) computePaths() {
	if c.ProjectDir == "" {
		cwd, _ := os.Getwd()
		c.ProjectDir = cwd
	}

	// Daemon paths are global (under ~/.config/sortie/)
	globalDir := getGlobalDataDir()
	c.DatabasePath = filepath.Join(globalDir, "tasks.db")
	c.SocketPath = filepath.Join(globalDir, "daemon.sock")
	c.PidFile = filepath.Join(globalDir, "daemon.pid")
}

// getGlobalDataDir returns the global data directory for daemon state.
func getGlobalDataDir() string {
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		return filepath.Join(xdgConfig, "sortie")
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "sortie")
	}
	return filepath.Join(homeDir, ".config", "sortie")
}

// GetGlobalDataDir is the exported version for use by other packages.
func GetGlobalDataDir() string {
	return getGlobalDataDir()
}

func (c *Config) syncCompat() {
	c.Daemon = daemonCompat{
		SocketPath:   c.SocketPath,
		PidFile:      c.PidFile,
		PollInterval: c.PollInterval,
	}
	c.Database = databaseCompat{
		Path: c.DatabasePath,
	}
	c.Agents = agentsCompat{
		MaxConcurrent:     c.MaxWorkers,
		OutputBufferLines: 10000,
		MaxRetries:        3,
	}
	c.Project.AutoDetect = true
}

// EnsureDirs creates the .sortie directory and any parent dirs needed.
func (c *Config) EnsureDirs() error {
	dirs := []string{
		filepath.Dir(c.SocketPath),
		filepath.Dir(c.PidFile),
		filepath.Dir(c.DatabasePath),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return nil
}

// GetDatabasePath returns the global database path.
// The projectDir parameter is kept for backward compatibility but is no longer used.
func (c *Config) GetDatabasePath(_ string) string {
	return c.DatabasePath
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

// ListWorkflowNames returns the names of all configured workflows,
// excluding synthetic task: workflows. If no workflows are configured, returns ["default"].
func (c *Config) ListWorkflowNames() []string {
	if len(c.Workflows) == 0 {
		return []string{"default"}
	}
	var names []string
	for _, w := range c.Workflows {
		if !strings.HasPrefix(w.Name, "task:") {
			names = append(names, w.Name)
		}
	}
	if len(names) == 0 {
		return []string{"default"}
	}
	return names
}

// ListPredefinedTaskNames returns the names of all configured predefined tasks.
func (c *Config) ListPredefinedTaskNames() []string {
	names := make([]string, len(c.Tasks))
	for i, t := range c.Tasks {
		names[i] = t.Name
	}
	return names
}

// GetPredefinedTask returns the predefined task config with the given name, or nil.
func (c *Config) GetPredefinedTask(name string) *TaskConfig {
	for i := range c.Tasks {
		if c.Tasks[i].Name == name {
			return &c.Tasks[i]
		}
	}
	return nil
}

// GetWorkflowSteps returns configured steps or the default single step.
// Kept for backward compatibility - use GetWorkflow instead.
func (c *Config) GetWorkflowSteps() []StepConfig {
	wf := c.GetWorkflow("")
	return wf.Steps
}

// ResolveBranchName applies the branch template for a task.
func (c *Config) ResolveBranchName(taskID int64, taskSlug string) string {
	tmpl := c.Git.BranchTemplate
	tmpl = strings.ReplaceAll(tmpl, "{{task_id}}", fmt.Sprintf("%d", taskID))
	tmpl = strings.ReplaceAll(tmpl, "{{task_slug}}", taskSlug)
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

	global := GlobalConfig{
		MaxWorkers:               c.MaxWorkers,
		Notifications:            c.Notifications,
		TmuxNestedAttachBehavior: c.TmuxNestedAttachBehavior,
	}

	data, err := yaml.Marshal(global)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

func getGlobalConfigPath() string {
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		return filepath.Join(xdgConfig, "sortie", "config.yaml")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(homeDir, ".config", "sortie", "config.yaml")
}

func getProjectConfigPath() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	// Look for .sortie.yml at repo root
	path := filepath.Join(cwd, ".sortie.yml")
	if _, err := os.Stat(path); err == nil {
		return path
	}

	return ""
}

// ApplyDetectedProject applies auto-detected project settings.
func (c *Config) ApplyDetectedProject(dir string) {
	if !c.Project.AutoDetect {
		return
	}

	detected := DetectProject(dir)
	if detected.Type == ProjectTypeUnknown {
		return
	}

	if c.Project.Name == "" {
		c.Project.Name = detected.Name
	}
	if c.Project.Commands.Test == "" {
		c.Project.Commands.Test = detected.Commands.Test
	}
	if c.Project.Commands.Lint == "" {
		c.Project.Commands.Lint = detected.Commands.Lint
	}
	if c.Project.Commands.Build == "" {
		c.Project.Commands.Build = detected.Commands.Build
	}
}
