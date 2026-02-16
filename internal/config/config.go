package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ProjectConfig is loaded from .rtk.yaml at repo root
type ProjectConfig struct {
	MaxWorkers int                `yaml:"max_workers"`
	Planner    PlannerConfig      `yaml:"planner"`
	Git        GitConfig          `yaml:"git"`
	Workflows  []WorkflowConfig   `yaml:"workflows"`
	Workflow   WorkflowConfig     `yaml:"workflow"` // deprecated, backward compat
}

type PlannerConfig struct {
	Instructions string `yaml:"instructions"`
}

type GitConfig struct {
	BaseBranch     string `yaml:"base_branch"`
	BranchTemplate string `yaml:"branch_template"`
	OnComplete     string `yaml:"on_complete"`
}

type WorkflowConfig struct {
	Name             string       `yaml:"name"`
	Steps            []StepConfig `yaml:"steps"`
	SummarizerPrompt string       `yaml:"summarizer_prompt"`
}

type StepConfig struct {
	Name             string `yaml:"name"`
	Prompt           string `yaml:"prompt"`
	Mode             string `yaml:"mode"`
	Timeout          string `yaml:"timeout"`
	ApprovalRequired bool   `yaml:"approval_required"`
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

// GlobalConfig from ~/.config/ralph-tamer-kit/config.yaml
type GlobalConfig struct {
	MaxWorkers    int                 `yaml:"max_workers"`
	Notifications NotificationsConfig `yaml:"notifications"`
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
// It combines global config, project config (.rtk.yaml), and computed paths.
type Config struct {
	// From .rtk.yaml (project config)
	MaxWorkers int
	Planner    PlannerConfig
	Git        GitConfig
	Workflows  []WorkflowConfig

	// From global config
	Notifications NotificationsConfig

	// Internal defaults (not in yaml)
	Claude ClaudeConfig

	// Whether a project config (.rtk.yaml) was found
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
		Planner:    PlannerConfig{},
		Git: GitConfig{
			BranchTemplate: "rtk/{{task_id}}-{{task_slug}}",
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
				Name:   "implement",
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

// Load loads config from global + project .rtk.yaml, returning a merged Config.
func Load() (*Config, error) {
	cfg := defaultConfig()

	// Load global config
	globalPath := getGlobalConfigPath()
	if globalPath != "" {
		if err := loadGlobalConfig(globalPath, cfg); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}

	// Load project config (.rtk.yaml at repo root)
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

	projectPath := filepath.Join(projectDir, ".rtk.yaml")
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
	if proj.Planner.Instructions != "" {
		cfg.Planner = proj.Planner
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

	return nil
}

func (c *Config) computePaths() {
	if c.ProjectDir == "" {
		cwd, _ := os.Getwd()
		c.ProjectDir = cwd
	}

	rtkDir := filepath.Join(c.ProjectDir, ".rtk")
	c.DatabasePath = filepath.Join(rtkDir, "tasks.db")
	c.SocketPath = filepath.Join(rtkDir, "daemon.sock")
	c.PidFile = filepath.Join(rtkDir, "daemon.pid")
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

// EnsureDirs creates the .rtk directory and any parent dirs needed.
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

// GetDatabasePath returns the database path, potentially relative to projectDir.
func (c *Config) GetDatabasePath(projectDir string) string {
	if c.DatabasePath != "" && filepath.IsAbs(c.DatabasePath) {
		return c.DatabasePath
	}

	if projectDir != "" {
		return filepath.Join(projectDir, ".rtk", "tasks.db")
	}

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

// ListWorkflowNames returns the names of all configured workflows.
// If no workflows are configured, returns ["default"].
func (c *Config) ListWorkflowNames() []string {
	if len(c.Workflows) == 0 {
		return []string{"default"}
	}
	names := make([]string, len(c.Workflows))
	for i, w := range c.Workflows {
		names[i] = w.Name
	}
	return names
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

// WriteProjectConfig writes a .rtk.yaml file.
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
		MaxWorkers:    c.MaxWorkers,
		Notifications: c.Notifications,
	}

	data, err := yaml.Marshal(global)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

func getGlobalConfigPath() string {
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		return filepath.Join(xdgConfig, "ralph-tamer-kit", "config.yaml")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(homeDir, ".config", "ralph-tamer-kit", "config.yaml")
}

func getProjectConfigPath() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	// Look for .rtk.yaml at repo root
	path := filepath.Join(cwd, ".rtk.yaml")
	if _, err := os.Stat(path); err == nil {
		return path
	}

	// Legacy: .ralph-tamer-kit/config.yaml
	legacyPath := filepath.Join(cwd, ".ralph-tamer-kit", "config.yaml")
	if _, err := os.Stat(legacyPath); err == nil {
		return legacyPath
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
