package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

func defaultConfig() *Config {
	animEnabled := true
	animDuration := 1500
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
			Command: "claude",
			Yolo:    false,
		},
		Options: OptionsConfig{
			Animation: &AnimationConfig{
				Enabled:  &animEnabled,
				Duration: &animDuration,
			},
		},
		PollInterval: 5 * time.Second,
		Agents: agentsCompat{
			MaxConcurrent:     3,
			OutputBufferLines: 10000,
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

// loadCommon loads the global config and global .sortie.yml into cfg.
func loadCommon(cfg *Config) error {
	// Load global config (~/.config/sortie/config.yaml)
	globalPath := getGlobalConfigPath()
	if globalPath != "" {
		if err := loadGlobalConfig(globalPath, cfg); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	// Load global .sortie.yml (~/.sortie.yml)
	globalSortieYml := getGlobalSortieYmlPath()
	if globalSortieYml != "" {
		if err := loadProjectConfig(globalSortieYml, cfg); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	return nil
}

// Load loads config from global config, global .sortie.yml, and project .sortie.yml,
// returning a merged Config. Loading order (later overrides earlier):
//  1. Built-in defaults
//  2. ~/.config/sortie/config.yaml (global daemon config)
//  3. ~/.sortie.yml (global sortie.yml defaults)
//  4. ./.sortie.yml (project config)
func Load() (*Config, error) {
	cfg := defaultConfig()
	if err := loadCommon(cfg); err != nil {
		return nil, err
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

	if cfg.ProjectDir != "" {
		cfg.ApplyDetectedProject(cfg.ProjectDir)
	}

	return cfg, nil
}

// LoadForProject loads config for a specific project directory.
func LoadForProject(projectDir string) (*Config, error) {
	cfg := defaultConfig()
	if err := loadCommon(cfg); err != nil {
		return nil, err
	}

	// Load project config (.sortie.yml)
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
	cfg.ApplyDetectedProject(cfg.ProjectDir)

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
	if global.Yolo != nil {
		cfg.Claude.Yolo = *global.Yolo
	}
	if global.PollInterval != "" {
		if d, err := time.ParseDuration(global.PollInterval); err == nil && d > 0 {
			cfg.PollInterval = d
		} else if err != nil {
			return fmt.Errorf("invalid poll_interval %q: %w", global.PollInterval, err)
		}
	}
	if global.Verification != nil {
		cfg.Verification = *global.Verification
	}
	cfg.Notifications = global.Notifications
	if global.TmuxNestedAttachBehavior != "" {
		cfg.TmuxNestedAttachBehavior = global.TmuxNestedAttachBehavior
	}
	if global.Claude != nil {
		if global.Claude.Command != "" {
			cfg.Claude.Command = global.Claude.Command
		}
		if len(global.Claude.DefaultArgs) > 0 {
			cfg.Claude.DefaultArgs = global.Claude.DefaultArgs
		}
	}
	if global.Options != nil {
		if global.Options.Number != nil {
			cfg.Options.Number = global.Options.Number
		}
		if global.Options.Branch != nil {
			cfg.Options.Branch = global.Options.Branch
		}
		if global.Options.Target != nil {
			cfg.Options.Target = global.Options.Target
		}
		if global.Options.Animation != nil {
			if cfg.Options.Animation == nil {
				cfg.Options.Animation = &AnimationConfig{}
			}
			if global.Options.Animation.Enabled != nil {
				cfg.Options.Animation.Enabled = global.Options.Animation.Enabled
			}
			if global.Options.Animation.Duration != nil {
				cfg.Options.Animation.Duration = global.Options.Animation.Duration
			}
		}
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
	if proj.PollInterval != "" {
		d, err := time.ParseDuration(proj.PollInterval)
		if err != nil {
			return fmt.Errorf("invalid poll_interval %q: %w", proj.PollInterval, err)
		}
		if d > 0 {
			cfg.PollInterval = d
		}
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
	if proj.Yolo != nil {
		cfg.Claude.Yolo = *proj.Yolo
	}
	if proj.Claude != nil {
		if proj.Claude.Command != "" {
			cfg.Claude.Command = proj.Claude.Command
		}
		if len(proj.Claude.DefaultArgs) > 0 {
			cfg.Claude.DefaultArgs = proj.Claude.DefaultArgs
		}
	}
	if proj.Verification != nil {
		cfg.Verification = *proj.Verification
	}
	if proj.Notifications != nil {
		cfg.Notifications = *proj.Notifications
	}
	if proj.TmuxNestedAttachBehavior != "" {
		cfg.TmuxNestedAttachBehavior = proj.TmuxNestedAttachBehavior
	}
	if proj.SystemPrompt != "" {
		cfg.SystemPrompt = proj.SystemPrompt
	}
	if !proj.WorktreeSyncPaths.IsEmpty() {
		cfg.WorktreeSyncPaths = proj.WorktreeSyncPaths
	}
	if proj.WorktreeSetupCommand != "" {
		cfg.WorktreeSetupCommand = proj.WorktreeSetupCommand
	}
	if len(proj.WorktreeSetupCommands) > 0 {
		cfg.WorktreeSetupCommands = proj.WorktreeSetupCommands
	}
	if proj.TmuxSetupCommand != "" {
		cfg.TmuxSetupCommand = proj.TmuxSetupCommand
	}
	if len(proj.AllowedSummarizationModels) > 0 {
		for _, m := range proj.AllowedSummarizationModels {
			if !ValidSummarizationModels[m] {
				return fmt.Errorf("invalid allowed_summarization_models entry %q (must be one of %q, %q, %q)",
					m, SummarizationModelHaiku, SummarizationModelSonnet, SummarizationModelOpus)
			}
		}
		cfg.AllowedSummarizationModels = append([]string(nil), proj.AllowedSummarizationModels...)
	}
	if proj.Options != nil {
		if proj.Options.Number != nil {
			cfg.Options.Number = proj.Options.Number
		}
		if proj.Options.Branch != nil {
			cfg.Options.Branch = proj.Options.Branch
		}
		if proj.Options.Target != nil {
			cfg.Options.Target = proj.Options.Target
		}
		if proj.Options.Animation != nil {
			if cfg.Options.Animation == nil {
				cfg.Options.Animation = &AnimationConfig{}
			}
			if proj.Options.Animation.Enabled != nil {
				cfg.Options.Animation.Enabled = proj.Options.Animation.Enabled
			}
			if proj.Options.Animation.Duration != nil {
				cfg.Options.Animation.Duration = proj.Options.Animation.Duration
			}
		}
	}

	return resolveWorkflows(cfg, &proj)
}

// resolveWorkflows processes raw project workflows into the Config's flat and categorized lists.
// It handles three input formats (structured, legacy list, ancient singular) and ensures
// all workflows have names and valid loop configurations.
func resolveWorkflows(cfg *Config, proj *ProjectConfig) error {
	// Handle workflows section: supports three formats:
	// 1. New structured: workflows: { one-off: [...], tasks: [...], init: [...] }
	// 2. Legacy list:    workflows: [{ name: ..., steps: [...] }, ...]
	// 3. Ancient singular: workflow: { steps: [...] }
	hasNewFormat := len(proj.Workflows.OneOff) > 0 || len(proj.Workflows.Tasks) > 0 || len(proj.Workflows.Init) > 0

	if hasNewFormat {
		// New structured format - only override categories that are actually defined
		// so that e.g. global init workflows are preserved when project config
		// only defines tasks and one-off workflows.
		if len(proj.Workflows.Tasks) > 0 {
			cfg.TaskWorkflows = proj.Workflows.Tasks
		}
		if len(proj.Workflows.OneOff) > 0 {
			cfg.OneOff = proj.Workflows.OneOff
		}
		if len(proj.Workflows.Init) > 0 {
			cfg.InitWorkflows = proj.Workflows.Init
		}

		// Build flat list for engine resolution from the merged categories
		cfg.Workflows = nil
		for _, wf := range cfg.TaskWorkflows {
			cfg.Workflows = append(cfg.Workflows, wf)
		}
		for _, wf := range cfg.OneOff {
			engineWf := wf
			engineWf.Name = "oneoff:" + wf.Name
			cfg.Workflows = append(cfg.Workflows, engineWf)
		}
		for _, wf := range cfg.InitWorkflows {
			engineWf := wf
			engineWf.Name = "init:" + wf.Name
			cfg.Workflows = append(cfg.Workflows, engineWf)
		}
	} else if len(proj.Workflows.legacy) > 0 {
		// Legacy list format: all items are task workflows
		cfg.TaskWorkflows = proj.Workflows.legacy
		cfg.Workflows = proj.Workflows.legacy
	} else if len(proj.Workflow.Steps) > 0 {
		// Ancient singular format
		cfg.TaskWorkflows = []WorkflowConfig{proj.Workflow}
		cfg.Workflows = []WorkflowConfig{proj.Workflow}
	}

	// Ensure all task workflows have names
	for i := range cfg.TaskWorkflows {
		if cfg.TaskWorkflows[i].Name == "" {
			if i == 0 {
				cfg.TaskWorkflows[i].Name = "default"
			} else {
				cfg.TaskWorkflows[i].Name = fmt.Sprintf("workflow-%d", i+1)
			}
		}
	}
	// Ensure all one-off workflows have names
	for i := range cfg.OneOff {
		if cfg.OneOff[i].Name == "" {
			cfg.OneOff[i].Name = fmt.Sprintf("oneoff-%d", i+1)
		}
	}
	// Ensure all init workflows have names
	for i := range cfg.InitWorkflows {
		if cfg.InitWorkflows[i].Name == "" {
			cfg.InitWorkflows[i].Name = fmt.Sprintf("init-%d", i+1)
		}
	}

	// Sync names into the flat Workflows list
	for i := range cfg.Workflows {
		if cfg.Workflows[i].Name == "" {
			cfg.Workflows[i].Name = fmt.Sprintf("workflow-%d", i+1)
		}
	}

	// Handle deprecated tasks: root key (backward compat — these become one-off)
	if len(proj.Tasks) > 0 {
		for i, task := range proj.Tasks {
			name := task.Name
			if name == "" {
				name = fmt.Sprintf("task-%d", i+1)
			}
			wf := WorkflowConfig{
				Name:             name,
				Description:      task.Description,
				Steps:            task.Steps,
				SummarizerPrompt: task.SummarizerPrompt,
			}
			cfg.OneOff = append(cfg.OneOff, wf)
			// Register for engine resolution with oneoff: prefix
			engineWf := wf
			engineWf.Name = "oneoff:" + name
			cfg.Workflows = append(cfg.Workflows, engineWf)
		}
	}

	// Validate workflow configurations (after all workflows are assembled)
	for i := range cfg.Workflows {
		if err := cfg.Workflows[i].ValidateLoops(); err != nil {
			return fmt.Errorf("workflow %q: %w", cfg.Workflows[i].Name, err)
		}
		if err := cfg.Workflows[i].ValidateSteps(); err != nil {
			return fmt.Errorf("workflow %q: %w", cfg.Workflows[i].Name, err)
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
		OutputBufferLines: defaultOutputBufferLines,
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

// ApplyDetectedProject applies auto-detected project settings.
func (c *Config) ApplyDetectedProject(dir string) {
	if !c.Project.AutoDetect {
		return
	}

	detected := DetectProject(dir)

	if c.Project.Name == "" {
		c.Project.Name = ProjectNameFromPath(dir)
	}

	if detected.Type == ProjectTypeUnknown {
		return
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

func getGlobalSortieYmlPath() string {
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		path := filepath.Join(xdgConfig, "sortie", "config.yml")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(homeDir, ".sortie.yml")
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
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

// SanitizeProjectName replaces characters that are problematic for downstream
// consumers (e.g. tmux silently converts dots to underscores, breaking session
// prefix matching). Applied at project name creation time so all consumers get
// a clean, consistent name.
func SanitizeProjectName(name string) string {
	return strings.ReplaceAll(name, ".", "_")
}

// ProjectNameFromPath derives the canonical project name from a directory path.
// This is the single source of truth for converting a filesystem path into the
// name used as a database key. All call sites that need to look up or store a
// project by its directory must route through this helper to avoid sanitization
// drift between write and read paths (e.g. ".pai" → stored as "_pai").
func ProjectNameFromPath(path string) string {
	return SanitizeProjectName(filepath.Base(path))
}
