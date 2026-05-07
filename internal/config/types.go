package config

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// AnimationConfig controls the sortie (airplane) animation on task submission.
type AnimationConfig struct {
	Enabled  *bool `yaml:"enabled,omitempty"`
	Duration *int  `yaml:"duration,omitempty"` // milliseconds
}

// OptionsConfig holds TUI display options configurable via .sortie.yml
type OptionsConfig struct {
	Number     *bool            `yaml:"number,omitempty"`
	Branch     *bool            `yaml:"branch,omitempty"`
	Target     *bool            `yaml:"target,omitempty"`
	BranchView *bool            `yaml:"branchview,omitempty"`
	Animation  *AnimationConfig `yaml:"animation,omitempty"`
}

// WorktreeSyncPathsConfig specifies paths to sync into worktrees via copy or symlink.
// Supports both the new structured format (map with copy/link keys) and the legacy
// plain list format ([]string, treated as copy paths for backward compatibility).
type WorktreeSyncPathsConfig struct {
	Copy []string `yaml:"copy,omitempty"`
	Link []string `yaml:"link,omitempty"`
}

// UnmarshalYAML handles both the new structured format and the legacy list format.
// Legacy: worktree-sync-paths: [".claude", ".env"]  → treated as copy paths
// New:    worktree-sync-paths: { copy: [...], link: [...] }
func (w *WorktreeSyncPathsConfig) UnmarshalYAML(value *yaml.Node) error {
	// Try legacy list format first
	if value.Kind == yaml.SequenceNode {
		var paths []string
		if err := value.Decode(&paths); err != nil {
			return err
		}
		w.Copy = paths
		return nil
	}

	// New structured format
	type raw WorktreeSyncPathsConfig
	var r raw
	if err := value.Decode(&r); err != nil {
		return err
	}
	w.Copy = r.Copy
	w.Link = r.Link
	return nil
}

// IsEmpty returns true if no sync paths are configured.
func (w WorktreeSyncPathsConfig) IsEmpty() bool {
	return len(w.Copy) == 0 && len(w.Link) == 0
}

// AllPaths returns all configured paths (both copy and link) for backward-compatible checks.
func (w WorktreeSyncPathsConfig) AllPaths() []string {
	paths := make([]string, 0, len(w.Copy)+len(w.Link))
	paths = append(paths, w.Copy...)
	paths = append(paths, w.Link...)
	return paths
}

// ProjectConfig is loaded from .sortie.yml (both global ~/.sortie.yml and project-local)
type ProjectConfig struct {
	MaxWorkers               int                     `yaml:"max_workers"`
	DefaultPriority          string                  `yaml:"default_priority"`
	Yolo                     *bool                   `yaml:"yolo,omitempty"`
	Verification             *VerificationConfig     `yaml:"verification,omitempty"`
	Git                      GitConfig               `yaml:"git"`
	Workflows                ProjectWorkflows        `yaml:"workflows"`
	Workflow                 WorkflowConfig          `yaml:"workflow"` // deprecated, backward compat
	Tasks                    []TaskConfig            `yaml:"tasks"`    // deprecated, use workflows.one-off
	Notifications            *NotificationsConfig    `yaml:"notifications,omitempty"`
	TmuxNestedAttachBehavior string                  `yaml:"tmux_nested_attach_behavior"`
	SystemPrompt             string                  `yaml:"system_prompt"`
	WorktreeSyncPaths        WorktreeSyncPathsConfig `yaml:"worktree-sync-paths"`
	WorktreeSetupCommand     string                  `yaml:"worktree-setup-command"`
	WorktreeSetupCommands    []string                `yaml:"worktree-setup-commands"`
	TmuxSetupCommand         string                  `yaml:"tmux-setup-command"`
	Options                  *OptionsConfig          `yaml:"options,omitempty"`
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
	Name                 string                  `yaml:"name"`
	Description          string                  `yaml:"description,omitempty"`
	Tmux                 bool                    `yaml:"tmux,omitempty"`
	Steps                []StepConfig            `yaml:"steps"`
	SummarizerPrompt     string                  `yaml:"summarizer_prompt"`
	WorktreeSyncPaths    WorktreeSyncPathsConfig `yaml:"worktree-sync-paths,omitempty"`
	WorktreeSetupCommand  string                  `yaml:"worktree-setup-command,omitempty"`
	WorktreeSetupCommands []string               `yaml:"worktree-setup-commands,omitempty"`
	TmuxSetupCommand     string                  `yaml:"tmux-setup-command,omitempty"`
}

type StepConfig struct {
	Name                  string      `yaml:"name"`
	Prompt                string      `yaml:"prompt"`
	Mode                  string      `yaml:"mode"`
	Tmux                  *bool       `yaml:"tmux,omitempty"`
	Timeout               string      `yaml:"timeout"`
	Human                 bool        `yaml:"human"`
	Loop                  *LoopConfig `yaml:"loop,omitempty"`
	SummarizationStrategy string      `yaml:"summarization_strategy,omitempty"`
	// SummarizationPrompt is the prompt template used when summarization_strategy is
	// "summarize_chat". Supports {{task.id}}, {{task.title}}, etc. template variables.
	// When empty, the default summarization prompt is used.
	SummarizationPrompt string `yaml:"summarization_prompt,omitempty"`
}

// LoopConfig defines a closed-loop jump back to an earlier step.
type LoopConfig struct {
	Goto          string             `yaml:"goto"`
	MaxIterations int                `yaml:"max_iterations"`
	ExitCondition *LoopExitCondition `yaml:"exit_condition,omitempty"`
}

// LoopExitCondition defines when a loop should exit early.
type LoopExitCondition struct {
	StepContextEmpty string `yaml:"step_context_empty"` // step name whose context to check
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
			if step.Loop.ExitCondition.StepContextEmpty != "" {
				if _, ok := stepIndex[step.Loop.ExitCondition.StepContextEmpty]; !ok {
					return fmt.Errorf("step %q: exit_condition step_context_empty references unknown step %q", step.Name, step.Loop.ExitCondition.StepContextEmpty)
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

// ValidateSteps checks step-level configurations for correctness.
func (wf *WorkflowConfig) ValidateSteps() error {
	for _, step := range wf.Steps {
		if !ValidSummarizationStrategies[step.SummarizationStrategy] {
			return fmt.Errorf("step %q: invalid summarization_strategy %q (must be %q or %q)",
				step.Name, step.SummarizationStrategy,
				SummarizationStrategyLastMessage, SummarizationStrategySummarizeChat)
		}
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

// FirstStepIsTmux returns true when this workflow has at least one step and the
// first step runs in tmux (either via step-level Tmux=true or the workflow-level
// Tmux default). Tmux-first workflows may be started without a description since
// the user drives the session interactively.
func (wf *WorkflowConfig) FirstStepIsTmux() bool {
	if wf == nil || len(wf.Steps) == 0 {
		return false
	}
	return wf.Steps[0].UseTmux(wf.Tmux)
}

const (
	// DefaultStepTimeout is the fallback step timeout when none is configured.
	DefaultStepTimeout = 30 * time.Minute

	// defaultOutputBufferLines is the default size of the per-agent output ring buffer.
	defaultOutputBufferLines = 10000

	// SummarizationStrategyLastMessage uses the Claude result event text as step context (default).
	SummarizationStrategyLastMessage = "last_message"

	// SummarizationStrategySummarizeChat spins up a background haiku process to
	// summarize the full chat log and stores the summary as step context.
	SummarizationStrategySummarizeChat = "summarize_chat"
)

// ValidSummarizationStrategies enumerates accepted values for StepConfig.SummarizationStrategy.
var ValidSummarizationStrategies = map[string]bool{
	"":                                 true, // empty means default (last_message)
	SummarizationStrategyLastMessage:   true,
	SummarizationStrategySummarizeChat: true,
}

// GlobalConfig from ~/.config/sortie/config.yaml
type GlobalConfig struct {
	MaxWorkers               int                 `yaml:"max_workers"`
	Yolo                     *bool               `yaml:"yolo,omitempty"`
	Verification             *VerificationConfig `yaml:"verification,omitempty"`
	Notifications            NotificationsConfig `yaml:"notifications"`
	TmuxNestedAttachBehavior string              `yaml:"tmux_nested_attach_behavior"`
	Options                  *OptionsConfig      `yaml:"options,omitempty"`
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
	MaxWorkers      int
	DefaultPriority string
	Verification    VerificationConfig
	Git             GitConfig
	Workflows       []WorkflowConfig // flat list for engine resolution (all kinds, with prefixed names)
	TaskWorkflows   []WorkflowConfig // "tasks" workflows (for "n" new task menu)
	OneOff          []WorkflowConfig // "one-off" workflows (for "r" run menu)
	InitWorkflows   []WorkflowConfig // "init" workflows (for "i" init menu)

	// System prompt preamble passed via --system-prompt to Claude agents
	SystemPrompt string

	// Paths to sync from project root into new worktrees
	WorktreeSyncPaths WorktreeSyncPathsConfig

	// Command to run after creating a worktree (e.g. dependency installation)
	WorktreeSetupCommand string

	// Commands to run sequentially after creating a worktree
	WorktreeSetupCommands []string

	// Command to run after creating a tmux session (e.g. layout setup)
	TmuxSetupCommand string

	// From global config
	Notifications            NotificationsConfig
	TmuxNestedAttachBehavior string // "switch" (default) or "nest"

	// TUI display options (from .sortie.yml options section)
	Options OptionsConfig

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
}
