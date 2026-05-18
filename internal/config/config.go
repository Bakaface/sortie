package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// workflowCategories lists the supported on-disk workflow categories under
// .sortie/workflows/<category>/. The order matters for flat-list assembly
// (tasks first, then oneoff, then init) and matches the engine prefix scheme.
var workflowCategories = []string{"tasks", "one-off", "init"}

// validWorkflowFilename matches kebab-case workflow filenames.
// File extension is checked separately (.yml or .yaml).
var validWorkflowFilename = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

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

	// Discover file-based workflows under <dir>/.sortie/workflows/<category>/
	baseDir := filepath.Dir(path)
	filePool, err := loadWorkflowFilePool(baseDir)
	if err != nil {
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

	return resolveWorkflows(cfg, &proj, filePool)
}

// workflowFilePool holds workflow definitions discovered on disk under
// .sortie/workflows/<category>/, keyed by category → name → loaded workflow.
// Files that haven't been referenced from .sortie.yml at the end of resolution
// are appended to their category's slice as Hidden=true.
type workflowFilePool struct {
	// byCategory[category][name] → WorkflowConfig (with Source set, Hidden=false).
	byCategory map[string]map[string]WorkflowConfig
	// order[category] preserves alphabetical iteration order over files for
	// stable Hidden appending.
	order map[string][]string
}

func newWorkflowFilePool() *workflowFilePool {
	return &workflowFilePool{
		byCategory: make(map[string]map[string]WorkflowConfig, len(workflowCategories)),
		order:      make(map[string][]string, len(workflowCategories)),
	}
}

// lookup returns the file-based workflow for category/name and reports whether
// it was found.
func (p *workflowFilePool) lookup(category, name string) (WorkflowConfig, bool) {
	if p == nil {
		return WorkflowConfig{}, false
	}
	m, ok := p.byCategory[category]
	if !ok {
		return WorkflowConfig{}, false
	}
	wf, ok := m[name]
	return wf, ok
}

// remove deletes a workflow from the pool (used to mark a file as "claimed"
// by an active string ref so we can identify unreferenced files at the end).
func (p *workflowFilePool) remove(category, name string) {
	if p == nil {
		return
	}
	if m, ok := p.byCategory[category]; ok {
		delete(m, name)
	}
}

// remainingNames returns the alphabetically-ordered names left in the pool for
// category. Used to append hidden workflows in stable order.
func (p *workflowFilePool) remainingNames(category string) []string {
	if p == nil {
		return nil
	}
	var names []string
	for _, n := range p.order[category] {
		if _, ok := p.byCategory[category][n]; ok {
			names = append(names, n)
		}
	}
	return names
}

// loadWorkflowFilePool scans <baseDir>/.sortie/workflows/<category>/ for each
// known category and returns the discovered workflows. Returns an empty pool
// when the .sortie/workflows directory doesn't exist (not an error).
func loadWorkflowFilePool(baseDir string) (*workflowFilePool, error) {
	pool := newWorkflowFilePool()
	if baseDir == "" {
		return pool, nil
	}
	root := filepath.Join(baseDir, ".sortie", "workflows")
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return pool, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return pool, nil
	}

	for _, category := range workflowCategories {
		dir := filepath.Join(root, category)
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", dir, err)
		}
		// Deterministic order — os.ReadDir already sorts by name, but make it explicit.
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

		for _, entry := range entries {
			if entry.IsDir() {
				return nil, fmt.Errorf("workflows.%s: subdirectories not supported (found %q)", category, entry.Name())
			}
			fname := entry.Name()
			ext := filepath.Ext(fname)
			if ext != ".yml" && ext != ".yaml" {
				return nil, fmt.Errorf("workflows.%s: invalid file extension %q (must be .yml or .yaml)", category, fname)
			}
			base := strings.TrimSuffix(fname, ext)
			if !validWorkflowFilename.MatchString(base) {
				return nil, fmt.Errorf("workflows.%s: invalid filename %q (must be kebab-case: [a-z0-9-]+)", category, fname)
			}

			path := filepath.Join(dir, fname)
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", path, err)
			}

			// Reject any `name:` field in file-based workflows — filename is the name.
			if err := assertNoNameField(data); err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}

			var wf WorkflowConfig
			if err := yaml.Unmarshal(data, &wf); err != nil {
				return nil, fmt.Errorf("parse %s: %w", path, err)
			}
			wf.Name = base
			wf.Source = path

			if pool.byCategory[category] == nil {
				pool.byCategory[category] = make(map[string]WorkflowConfig)
			}
			if _, dup := pool.byCategory[category][base]; dup {
				return nil, fmt.Errorf("workflows.%s: duplicate file-based workflow %q", category, base)
			}
			pool.byCategory[category][base] = wf
			pool.order[category] = append(pool.order[category], base)
		}
	}

	return pool, nil
}

// assertNoNameField rejects file-based workflow definitions that set a `name:`
// field. The filename is authoritative; allowing `name:` invites name/file
// drift.
func assertNoNameField(data []byte) error {
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil // surface the parse error from the main decode path
	}
	// The top of a document is a DocumentNode containing one MappingNode.
	root := &node
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		root = root.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		key := root.Content[i]
		if key.Kind == yaml.ScalarNode && key.Value == "name" {
			return fmt.Errorf("file-based workflows must not set `name:` (filename is the name)")
		}
	}
	return nil
}

// resolveWorkflows processes raw project workflows into the Config's flat and categorized lists.
// It handles three input formats (structured, legacy list, ancient singular), merges in
// file-based workflows from filePool, ensures all workflows have names, and validates them.
//
// File-based workflows referenced via string entries in .sortie.yml become active (in listing
// order). Files in the pool not referenced from .sortie.yml are appended to their category's
// slice as Hidden=true (alphabetical order for stability). Inline + file collision is a hard
// error.
func resolveWorkflows(cfg *Config, proj *ProjectConfig, filePool *workflowFilePool) error {
	// Handle workflows section: supports three formats:
	// 1. New structured: workflows: { one-off: [...], tasks: [...], init: [...] }
	// 2. Legacy list:    workflows: [{ name: ..., steps: [...] }, ...]
	// 3. Ancient singular: workflow: { steps: [...] }
	hasNewFormat := len(proj.Workflows.OneOff) > 0 || len(proj.Workflows.Tasks) > 0 || len(proj.Workflows.Init) > 0

	// Track whether the file pool was consulted at all so we can append hidden
	// workflows even when .sortie.yml has no entries for that category.
	hasFilePool := filePool != nil && len(filePool.byCategory) > 0

	if hasNewFormat || hasFilePool {
		// New structured format — only override categories actually defined in
		// proj.Workflows so global init workflows persist when project config
		// only defines a subset of categories. The file pool is consulted only
		// for categories whose pool is non-empty.
		if len(proj.Workflows.Tasks) > 0 || hasCategoryFiles(filePool, "tasks") {
			resolved, err := resolveCategory("tasks", proj.Workflows.Tasks, filePool)
			if err != nil {
				return err
			}
			cfg.TaskWorkflows = resolved
		}
		if len(proj.Workflows.OneOff) > 0 || hasCategoryFiles(filePool, "one-off") {
			resolved, err := resolveCategory("one-off", proj.Workflows.OneOff, filePool)
			if err != nil {
				return err
			}
			cfg.OneOff = resolved
		}
		if len(proj.Workflows.Init) > 0 || hasCategoryFiles(filePool, "init") {
			resolved, err := resolveCategory("init", proj.Workflows.Init, filePool)
			if err != nil {
				return err
			}
			cfg.InitWorkflows = resolved
		}

		// Build flat list for engine resolution from the merged categories.
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
		// Legacy list format: all items are task workflows. No file-pool merge —
		// callers using the legacy format opt out of the new mechanism.
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
				Source:           "inline",
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

// hasCategoryFiles reports whether filePool has any workflows for category.
func hasCategoryFiles(pool *workflowFilePool, category string) bool {
	if pool == nil {
		return false
	}
	return len(pool.byCategory[category]) > 0
}

// resolveCategory expands a single category's entries (string refs + inline
// defs) into a flat slice of WorkflowConfig. Active workflows come first in
// listing order; any files in the pool not referenced are appended as Hidden.
func resolveCategory(category string, entries []WorkflowEntry, filePool *workflowFilePool) ([]WorkflowConfig, error) {
	// Track names seen so we can flag duplicates and inline/file collisions.
	seen := make(map[string]bool, len(entries))
	out := make([]WorkflowConfig, 0, len(entries))

	for _, entry := range entries {
		switch {
		case entry.Ref != "":
			name := entry.Ref
			if seen[name] {
				return nil, fmt.Errorf("workflows.%s: duplicate workflow name %q", category, name)
			}
			wf, ok := filePool.lookup(category, name)
			if !ok {
				return nil, fmt.Errorf("workflows.%s: referenced workflow %q has no file at .sortie/workflows/%s/%s.yml", category, name, category, name)
			}
			wf.Hidden = false
			out = append(out, wf)
			filePool.remove(category, name)
			seen[name] = true
		case entry.Inline != nil:
			wf := *entry.Inline
			if wf.Name == "" {
				return nil, fmt.Errorf("workflows.%s: inline workflow is missing a name", category)
			}
			if seen[wf.Name] {
				return nil, fmt.Errorf("workflows.%s: duplicate workflow name %q", category, wf.Name)
			}
			if _, dup := filePool.lookup(category, wf.Name); dup {
				return nil, fmt.Errorf("workflows.%s: inline workflow %q collides with file at .sortie/workflows/%s/%s.yml — define it in one place only", category, wf.Name, category, wf.Name)
			}
			wf.Source = "inline"
			wf.Hidden = false
			out = append(out, wf)
			seen[wf.Name] = true
		default:
			return nil, fmt.Errorf("workflows.%s: empty entry", category)
		}
	}

	// Append unreferenced file-based workflows as hidden, alphabetical order.
	for _, name := range filePool.remainingNames(category) {
		wf, ok := filePool.lookup(category, name)
		if !ok {
			continue
		}
		wf.Hidden = true
		out = append(out, wf)
	}

	return out, nil
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
