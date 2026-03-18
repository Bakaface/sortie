package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ProjectConfig is loaded from .sortie.yml (both global ~/.sortie.yml and project-local)
type ProjectConfig struct {
	MaxWorkers               int                  `yaml:"max_workers"`
	DefaultPriority          string               `yaml:"default_priority"`
	Yolo                     *bool                `yaml:"yolo,omitempty"`
	ValidateArtifact         *bool                `yaml:"validate_artifact,omitempty"`
	Verification             *VerificationConfig  `yaml:"verification,omitempty"`
	Git                      GitConfig            `yaml:"git"`
	Workflows                ProjectWorkflows     `yaml:"workflows"`
	Workflow                 WorkflowConfig       `yaml:"workflow"` // deprecated, backward compat
	Tasks                    []TaskConfig         `yaml:"tasks"`    // deprecated, use workflows.one-off
	Notifications            *NotificationsConfig `yaml:"notifications,omitempty"`
	TmuxNestedAttachBehavior string               `yaml:"tmux_nested_attach_behavior"`
	SystemPrompt             string               `yaml:"system_prompt"`
	WorktreeSyncPaths        []string             `yaml:"worktree-sync-paths"`
	WorktreeSetupCommand     string               `yaml:"worktree-setup-command"`
	TmuxSetupCommand         string               `yaml:"tmux-setup-command"`
}

// ProjectWorkflows is the consolidated workflows section in .sortie.yml.
// Supports both the new structured format (map with one-off/tasks/init keys)
// and the legacy list format ([]WorkflowConfig).
type ProjectWorkflows struct {
	OneOff []WorkflowConfig `yaml:"one-off"`
	Tasks  []WorkflowConfig `yaml:"tasks"`
	Init   []WorkflowConfig `yaml:"init"`

	// legacy holds workflows parsed from the old list format
	legacy []WorkflowConfig
}

// UnmarshalYAML handles both the new structured format and the legacy list format.
func (pw *ProjectWorkflows) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.SequenceNode {
		return value.Decode(&pw.legacy)
	}
	type raw ProjectWorkflows
	return value.Decode((*raw)(pw))
}

// IsEmpty returns true if no workflows are configured in any format.
func (pw *ProjectWorkflows) IsEmpty() bool {
	return len(pw.OneOff) == 0 && len(pw.Tasks) == 0 && len(pw.Init) == 0 && len(pw.legacy) == 0
}

type VerificationConfig struct {
	ArtifactRetry    bool `yaml:"artifact_retry"`
	MaxRetries       int  `yaml:"max_retries"`
	VerifySummarizer bool `yaml:"verify_summarizer"`
}

// TaskConfig defines a predefined task with a built-in description and workflow steps.
// Deprecated: use workflows.one-off in .sortie.yml instead.
type TaskConfig struct {
	Name             string       `yaml:"name"`
	Description      string       `yaml:"description"`
	Unlisted         bool         `yaml:"unlisted"` // deprecated, ignored
	Steps            []StepConfig `yaml:"steps"`
	SummarizerPrompt string       `yaml:"summarizer_prompt"`
}

type GitConfig struct {
	BaseBranch     string `yaml:"base_branch"`
	BranchTemplate string `yaml:"branch_template"`
	OnComplete     string `yaml:"on_complete"`
}

type WorkflowConfig struct {
	Name              string       `yaml:"name"`
	Description       string       `yaml:"description,omitempty"`
	Tmux              bool         `yaml:"tmux,omitempty"`
	Steps             []StepConfig `yaml:"steps"`
	SummarizerPrompt  string       `yaml:"summarizer_prompt"`
	WorktreeSyncPaths []string     `yaml:"worktree-sync-paths,omitempty"`
	WorktreeSetupCommand string   `yaml:"worktree-setup-command,omitempty"`
	TmuxSetupCommand     string   `yaml:"tmux-setup-command,omitempty"`
}

type StepConfig struct {
	Name             string      `yaml:"name"`
	Prompt           string      `yaml:"prompt"`
	Mode             string      `yaml:"mode"`
	Tmux             *bool       `yaml:"tmux,omitempty"`
	Timeout          string      `yaml:"timeout"`
	Human            bool        `yaml:"human"`
	Artifact         bool        `yaml:"artifact"`
	Loop             *LoopConfig `yaml:"loop,omitempty"`
}

// LoopConfig defines a closed-loop jump back to an earlier step.
type LoopConfig struct {
	Goto          string             `yaml:"goto"`
	MaxIterations int                `yaml:"max_iterations"`
	ExitCondition *LoopExitCondition `yaml:"exit_condition,omitempty"`
}

// LoopExitCondition defines when a loop should exit early.
type LoopExitCondition struct {
	ArtifactEmpty string `yaml:"artifact_empty"` // step name whose artifact to check
}

// ValidateLoops checks all loop configurations in a workflow for correctness.
func (wf *WorkflowConfig) ValidateLoops() error {
	stepIndex := make(map[string]int)
	for i, s := range wf.Steps {
		stepIndex[s.Name] = i
	}

	// Track loop ranges [goto_target, loop_step] to detect overlaps
	type loopRange struct {
		from, to int
		stepName string
	}
	var ranges []loopRange

	for i, step := range wf.Steps {
		if step.Loop == nil {
			continue
		}

		// goto must reference an existing step
		targetIdx, ok := stepIndex[step.Loop.Goto]
		if !ok {
			return fmt.Errorf("step %q: loop goto references unknown step %q", step.Name, step.Loop.Goto)
		}

		// goto must be an earlier step (no forward jumps, no self-reference)
		if targetIdx >= i {
			return fmt.Errorf("step %q: loop goto must reference an earlier step (got %q at index %d, current at %d)", step.Name, step.Loop.Goto, targetIdx, i)
		}

		// max_iterations is required and must be >= 1
		if step.Loop.MaxIterations < 1 {
			return fmt.Errorf("step %q: loop max_iterations must be >= 1", step.Name)
		}

		// A step with loop cannot have human or tmux
		if step.Human {
			return fmt.Errorf("step %q: loop steps cannot have human: true", step.Name)
		}
		if step.Tmux != nil && *step.Tmux {
			return fmt.Errorf("step %q: loop steps cannot have tmux: true", step.Name)
		}

		// Validate exit condition
		if step.Loop.ExitCondition != nil {
			if step.Loop.ExitCondition.ArtifactEmpty != "" {
				if _, ok := stepIndex[step.Loop.ExitCondition.ArtifactEmpty]; !ok {
					return fmt.Errorf("step %q: exit_condition artifact_empty references unknown step %q", step.Name, step.Loop.ExitCondition.ArtifactEmpty)
				}
			}
		}

		// Check for overlapping loops
		newRange := loopRange{from: targetIdx, to: i, stepName: step.Name}
		for _, existing := range ranges {
			// Overlapping if one range starts inside the other
			if (newRange.from > existing.from && newRange.from < existing.to) ||
				(newRange.to > existing.from && newRange.to < existing.to) ||
				(existing.from > newRange.from && existing.from < newRange.to) {
				return fmt.Errorf("step %q: loop range [%d..%d] overlaps with step %q range [%d..%d]",
					step.Name, newRange.from, newRange.to,
					existing.stepName, existing.from, existing.to)
			}
		}
		ranges = append(ranges, newRange)
	}

	return nil
}

// UseTmux returns whether this step should use tmux execution.
// Step-level setting overrides the workflow default.
func (s *StepConfig) UseTmux(workflowDefault bool) bool {
	if s.Tmux != nil {
		return *s.Tmux
	}
	return workflowDefault
}

const (
	// DefaultStepTimeout is the fallback step timeout when none is configured.
	DefaultStepTimeout = 30 * time.Minute

	// defaultOutputBufferLines is the default size of the per-agent output ring buffer.
	defaultOutputBufferLines = 10000

	// defaultMaxRetries is the default number of retries for artifact verification.
	defaultMaxRetries = 3
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

// GlobalConfig from ~/.config/sortie/config.yaml
type GlobalConfig struct {
	MaxWorkers               int                 `yaml:"max_workers"`
	Yolo                     *bool               `yaml:"yolo,omitempty"`
	ValidateArtifact         *bool               `yaml:"validate_artifact,omitempty"`
	Verification             *VerificationConfig `yaml:"verification,omitempty"`
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
	Yolo        bool     // whether to pass --dangerously-skip-permissions
}

// Args returns the effective argument list, including --dangerously-skip-permissions if Yolo is enabled.
func (c *ClaudeConfig) Args() []string {
	args := append([]string{}, c.DefaultArgs...)
	if c.Yolo {
		args = append(args, "--dangerously-skip-permissions")
	}
	return args
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
	MaxWorkers       int
	DefaultPriority  string
	ValidateArtifact bool
	Verification     VerificationConfig
	Git              GitConfig
	Workflows        []WorkflowConfig // flat list for engine resolution (all kinds, with prefixed names)
	TaskWorkflows    []WorkflowConfig // "tasks" workflows (for "n" new task menu)
	OneOff           []WorkflowConfig // "one-off" workflows (for "r" run menu)
	InitWorkflows    []WorkflowConfig // "init" workflows (for "i" init menu)

	// System prompt preamble passed via --system-prompt to Claude agents
	SystemPrompt string

	// Paths to sync from project root into new worktrees
	WorktreeSyncPaths []string

	// Command to run after creating a worktree (e.g. dependency installation)
	WorktreeSetupCommand string

	// Command to run after creating a tmux session (e.g. layout setup)
	TmuxSetupCommand string

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
			Command: "claude",
			Yolo:    false,
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
	if global.ValidateArtifact != nil {
		cfg.ValidateArtifact = *global.ValidateArtifact
	}
	if global.Verification != nil {
		cfg.Verification = *global.Verification
		if cfg.Verification.ArtifactRetry && cfg.Verification.MaxRetries == 0 {
			cfg.Verification.MaxRetries = 1
		}
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
	if proj.Yolo != nil {
		cfg.Claude.Yolo = *proj.Yolo
	}
	if proj.ValidateArtifact != nil {
		cfg.ValidateArtifact = *proj.ValidateArtifact
	}
	if proj.Verification != nil {
		cfg.Verification = *proj.Verification
		if cfg.Verification.ArtifactRetry && cfg.Verification.MaxRetries == 0 {
			cfg.Verification.MaxRetries = 1
		}
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
	if len(proj.WorktreeSyncPaths) > 0 {
		cfg.WorktreeSyncPaths = proj.WorktreeSyncPaths
	}
	if proj.WorktreeSetupCommand != "" {
		cfg.WorktreeSetupCommand = proj.WorktreeSetupCommand
	}
	if proj.TmuxSetupCommand != "" {
		cfg.TmuxSetupCommand = proj.TmuxSetupCommand
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

	// Validate loop configurations (after all workflows are assembled)
	for i := range cfg.Workflows {
		if err := cfg.Workflows[i].ValidateLoops(); err != nil {
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
		MaxRetries:        defaultMaxRetries,
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

// ListWorkflowNames returns the names of all configured task workflows
// (the "tasks" kind used for new task creation). If none configured, returns ["default"].
func (c *Config) ListWorkflowNames() []string {
	if len(c.TaskWorkflows) == 0 {
		return []string{"default"}
	}
	names := make([]string, len(c.TaskWorkflows))
	for i, w := range c.TaskWorkflows {
		names[i] = w.Name
	}
	return names
}

// ListPredefinedTaskNames returns the names of all one-off workflows
// (the "one-off" kind shown in the "r" run menu).
func (c *Config) ListPredefinedTaskNames() []string {
	names := make([]string, len(c.OneOff))
	for i, t := range c.OneOff {
		names[i] = t.Name
	}
	return names
}

// GetPredefinedTask returns the one-off workflow config with the given name, or nil.
func (c *Config) GetPredefinedTask(name string) *WorkflowConfig {
	for i := range c.OneOff {
		if c.OneOff[i].Name == name {
			return &c.OneOff[i]
		}
	}
	return nil
}

// ListInitWorkflowNames returns the names of all init workflows
// (the "init" kind shown in the "i" init menu).
func (c *Config) ListInitWorkflowNames() []string {
	names := make([]string, len(c.InitWorkflows))
	for i, w := range c.InitWorkflows {
		names[i] = w.Name
	}
	return names
}

// GetInitWorkflow returns the init workflow config with the given name, or nil.
func (c *Config) GetInitWorkflow(name string) *WorkflowConfig {
	for i := range c.InitWorkflows {
		if c.InitWorkflows[i].Name == name {
			return &c.InitWorkflows[i]
		}
	}
	return nil
}

// GetWorktreeSyncPaths returns the sync paths for a workflow, falling back to the global config.
func (c *Config) GetWorktreeSyncPaths(wf *WorkflowConfig) []string {
	if wf != nil && len(wf.WorktreeSyncPaths) > 0 {
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

// ApplyDetectedProject applies auto-detected project settings.
func (c *Config) ApplyDetectedProject(dir string) {
	if !c.Project.AutoDetect {
		return
	}

	detected := DetectProject(dir)

	if c.Project.Name == "" {
		if detected.Name != "" {
			c.Project.Name = SanitizeProjectName(detected.Name)
		} else {
			c.Project.Name = SanitizeProjectName(filepath.Base(dir))
		}
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
