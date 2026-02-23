package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/aface/sortie/internal/client"
	"github.com/aface/sortie/internal/config"
	"github.com/aface/sortie/internal/daemon"
	"github.com/aface/sortie/internal/db"
	gitpkg "github.com/aface/sortie/internal/git"
	"github.com/aface/sortie/internal/task"
	"github.com/aface/sortie/internal/tmux"
	"github.com/aface/sortie/internal/tui"
	"github.com/aface/sortie/internal/workflow"
	"github.com/spf13/cobra"
)

var cfg *config.Config

// Commands that don't require a project config (.sortie.yml)
var noProjectRequired = map[string]bool{
	"init":             true,
	"help":             true,
	"completion":       true,
	"__complete":       true,
	"__completeNoDesc": true,
	// Daemon commands are global — they don't need a project config
	"start":  true, // daemon start
	"stop":   true, // daemon stop
	"status": true, // daemon status
}

var rootCmd = &cobra.Command{
	Use:   "sortie",
	Short: "Sortie orchestrates Claude Code agents",
	Long: `Sortie orchestrates Claude Code agents to work through tasks
systematically. It runs tasks through configurable multi-step workflows in
dedicated git worktrees, and provides real-time monitoring via TUI.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Daemon subcommands and TUI can run without .sortie.yml
		if isDaemonSubcommand(cmd) || cmd.Name() == "tui" {
			return nil
		}

		// Check for .sortie.yml unless this is a command that doesn't need it
		if !noProjectRequired[cmd.Name()] && !cfg.ProjectConfigFound {
			return fmt.Errorf("no .sortie.yml found — run 'sortie init' first")
		}

		return nil
	},
}

// isDaemonSubcommand returns true if the command is a daemon subcommand.
func isDaemonSubcommand(cmd *cobra.Command) bool {
	for p := cmd; p != nil; p = p.Parent() {
		if p.Name() == "daemon" {
			return true
		}
	}
	return false
}

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage the Sortie daemon",
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		foreground, _ := cmd.Flags().GetBool("foreground")
		return daemon.Start(cfg, foreground)
	},
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the daemon gracefully",
	RunE: func(cmd *cobra.Command, args []string) error {
		return daemon.Stop(cfg)
	},
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check if daemon is running",
	RunE: func(cmd *cobra.Command, args []string) error {
		running, pid, err := daemon.Status(cfg)
		if err != nil {
			return err
		}
		if running {
			fmt.Printf("Daemon is running (PID: %d)\n", pid)
		} else {
			fmt.Println("Daemon is not running")
		}
		return nil
	},
}

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch the TUI (connects to daemon)",
	RunE: func(cmd *cobra.Command, args []string) error {
		globalFlag, _ := cmd.Flags().GetBool("global")

		projectID, projectPath, globalMode := resolveProjectMode(globalFlag)
		return tui.Run(cfg, projectID, projectPath, globalMode)
	},
}

// resolveProjectMode determines the project filter for commands.
// If globalFlag is true, returns global mode.
// Otherwise, tries to detect the current project from cwd.
// If not in a project dir, defaults to global mode.
func resolveProjectMode(globalFlag bool) (projectID int64, projectPath string, globalMode bool) {
	if globalFlag {
		return 0, "", true
	}

	// Try to detect project from cwd
	cwd, err := os.Getwd()
	if err != nil {
		return 0, "", true
	}

	// Check if cwd has .sortie.yml (is a project)
	if _, err := os.Stat(filepath.Join(cwd, ".sortie.yml")); err != nil {
		// Not in a project dir — global mode
		return 0, "", true
	}

	// Resolve to git repo root for consistent path
	repoRoot, err := gitpkg.GetRepoRoot(cwd)
	if err != nil {
		return 0, cwd, false
	}

	// Look up or register project in global DB to get its ID
	dbPath := cfg.GetDatabasePath("")
	database, err := db.Open(dbPath)
	if err != nil {
		return 0, repoRoot, false
	}
	defer database.Close()

	proj, err := database.GetOrCreateProject(repoRoot)
	if err != nil {
		return 0, repoRoot, false
	}

	return proj.ID, repoRoot, false
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize Sortie in the current project",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		if !gitpkg.IsGitRepo(cwd) {
			return fmt.Errorf("not a git repository")
		}

		configPath := filepath.Join(cwd, ".sortie.yml")
		if _, err := os.Stat(configPath); err == nil {
			fmt.Println(".sortie.yml already exists")
			return nil
		}

		sortieDir := filepath.Join(cwd, ".sortie")
		if err := os.MkdirAll(sortieDir, 0755); err != nil {
			return fmt.Errorf("failed to create .sortie directory: %w", err)
		}

		detected := config.DetectProject(cwd)

		proj := &config.ProjectConfig{
			MaxWorkers: 3,
			Git: config.GitConfig{
				BranchTemplate: "sortie/{{task_id}}-{{task_slug}}",
				OnComplete:     "commit",
			},
		}

		if err := config.WriteProjectConfig(configPath, proj); err != nil {
			return fmt.Errorf("failed to write .sortie.yml: %w", err)
		}

		fmt.Printf("Initialized Sortie\n")
		fmt.Printf("  Config: %s\n", configPath)
		fmt.Printf("  Data:   %s/\n", sortieDir)
		fmt.Printf("Detected project type: %s\n", detected.Type)
		if detected.Commands.Test != "" {
			fmt.Printf("  Test command: %s\n", detected.Commands.Test)
		}
		if detected.Commands.Lint != "" {
			fmt.Printf("  Lint command: %s\n", detected.Commands.Lint)
		}

		return nil
	},
}

var tasksCmd = &cobra.Command{
	Use:               "tasks [task_id]",
	Short:             "List all tasks or show task detail",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeTaskIDs(),
	RunE: func(cmd *cobra.Command, args []string) error {
		// If a task ID is provided, show task detail
		if len(args) == 1 {
			return showTaskDetail(args[0])
		}

		// Try to connect to daemon first, fall back to direct DB access
		c := client.New(cfg)
		if err := c.Connect(); err != nil {
			return listTasksFromDB()
		}
		defer c.Close()

		tasks, err := c.ListTasks()
		if err != nil {
			return fmt.Errorf("failed to list tasks: %w", err)
		}

		if len(tasks) == 0 {
			fmt.Println("No tasks found. Create tasks via the TUI (n key) or daemon IPC.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tSTATUS\tSTEP\tTITLE")
		fmt.Fprintln(w, "--\t------\t----\t-----")

		for _, t := range tasks {
			title := t.Title
			if title == "" {
				title = truncateStr(t.Description, 50)
			}
			step := t.CurrentStep
			if step == "" {
				step = "-"
			}
			fmt.Fprintf(w, "#%d\t%s\t%s\t%s\n",
				t.ID,
				t.Status,
				step,
				title,
			)
		}
		w.Flush()

		return nil
	},
}

func listTasksFromDB() error {
	dbPath := cfg.GetDatabasePath("")
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	tasks, err := database.GetAllTasks()
	if err != nil {
		return fmt.Errorf("failed to get tasks: %w", err)
	}

	if len(tasks) == 0 {
		fmt.Println("No tasks found. Create tasks via the TUI (n key) or daemon IPC.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSTATUS\tSTEP\tTITLE")
	fmt.Fprintln(w, "--\t------\t----\t-----")

	for _, t := range tasks {
		title := t.Title
		if title == "" {
			title = truncateStr(t.Description, 50)
		}
		step := t.CurrentStep
		if step == "" {
			step = "-"
		}
		fmt.Fprintf(w, "#%d\t%s\t%s\t%s\n",
			t.ID,
			t.Status,
			step,
			title,
		)
	}
	w.Flush()
	return nil
}

func showTaskDetail(idStr string) error {
	taskID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid task ID: %s", idStr)
	}

	// Try daemon first
	c := client.New(cfg)
	if err := c.Connect(); err != nil {
		return showTaskDetailFromDB(taskID)
	}
	defer c.Close()

	t, err := c.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}

	printTaskDetail(t)
	return nil
}

func showTaskDetailFromDB(taskID int64) error {
	dbPath := cfg.GetDatabasePath("")
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	t, err := database.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}

	info := daemon.TaskInfo{
		ID:           t.ID,
		ProjectID:    t.ProjectID,
		Title:        t.Title,
		Description:  t.Description,
		Slug:         t.Slug,
		Status:       string(t.Status),
		StepIndex:    t.StepIndex,
		CurrentStep:  t.CurrentStep,
		Branch:       t.Branch,
		WorktreePath: t.WorktreePath,
		ErrorMessage: t.ErrorMessage,
		BlockedBy:    t.BlockedBy,
		CreatedAt:    t.CreatedAt,
		StartedAt:    t.StartedAt,
		CompletedAt:  t.CompletedAt,
	}
	if proj, err := database.GetProject(t.ProjectID); err == nil {
		info.ProjectName = proj.Name
		info.ProjectPath = proj.Path
	}
	printTaskDetail(&info)
	return nil
}

func printTaskDetail(t *daemon.TaskInfo) {
	fmt.Printf("Task #%d\n", t.ID)
	fmt.Printf("  Title:       %s\n", t.Title)
	fmt.Printf("  Status:      %s\n", t.Status)
	fmt.Printf("  Slug:        %s\n", t.Slug)
	if t.ProjectName != "" {
		fmt.Printf("  Project:     %s\n", t.ProjectName)
	}
	if t.Branch != "" {
		fmt.Printf("  Branch:      %s\n", t.Branch)
	}
	if t.CurrentStep != "" {
		fmt.Printf("  Step:        %s (index %d)\n", t.CurrentStep, t.StepIndex)
	}
	if t.WorktreePath != "" {
		fmt.Printf("  Worktree:    %s\n", t.WorktreePath)
	}
	if len(t.BlockedBy) > 0 {
		fmt.Printf("  Blocked by:  %v\n", t.BlockedBy)
	}
	if t.ErrorMessage != "" {
		fmt.Printf("  Error:       %s\n", t.ErrorMessage)
	}
	fmt.Printf("  Created:     %s\n", t.CreatedAt.Format(time.RFC3339))
	if t.StartedAt != nil {
		fmt.Printf("  Started:     %s\n", t.StartedAt.Format(time.RFC3339))
	}
	if t.CompletedAt != nil {
		fmt.Printf("  Completed:   %s\n", t.CompletedAt.Format(time.RFC3339))
	}
	fmt.Printf("\n  Description:\n    %s\n", t.Description)
}

var startCmd = &cobra.Command{
	Use:               "start <task_id>",
	Short:             "Manually start an agent for a task",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeTaskIDs(task.StatusPending),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid task ID: %s", args[0])
		}

		c := client.New(cfg)
		if err := c.Connect(); err != nil {
			return fmt.Errorf("failed to connect to daemon: %w", err)
		}
		defer c.Close()

		if err := c.StartAgent(taskID); err != nil {
			return fmt.Errorf("failed to start agent: %w", err)
		}

		fmt.Printf("Agent started for task #%d\n", taskID)
		return nil
	},
}

var listCmd = &cobra.Command{
	Use:     "agents",
	Aliases: []string{"list"},
	Short:   "List all running agents",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.New(cfg)
		if err := c.Connect(); err != nil {
			return fmt.Errorf("failed to connect to daemon: %w", err)
		}
		defer c.Close()

		agents, err := c.ListAgents()
		if err != nil {
			return fmt.Errorf("failed to list agents: %w", err)
		}

		if len(agents) == 0 {
			fmt.Println("No agents running")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tTASK\tDESCRIPTION\tSTATE\tDURATION")
		fmt.Fprintln(w, "--\t----\t-----------\t-----\t--------")

		for _, agent := range agents {
			duration := time.Since(agent.StartedAt).Round(time.Second)
			fmt.Fprintf(w, "%s\t#%d\t%s\t%s\t%s\n",
				agent.ID,
				agent.TaskID,
				truncateStr(agent.Description, 40),
				agent.State,
				duration,
			)
		}
		w.Flush()

		return nil
	},
}

var stopCmd = &cobra.Command{
	Use:               "stop <task_id>",
	Short:             "Stop a running task",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeTaskIDs(task.StatusRunning),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid task ID: %s", args[0])
		}

		c := client.New(cfg)
		if err := c.Connect(); err != nil {
			return fmt.Errorf("failed to connect to daemon: %w", err)
		}
		defer c.Close()

		if err := c.StopTask(taskID); err != nil {
			return fmt.Errorf("failed to stop task: %w", err)
		}

		fmt.Printf("Task #%d stopped\n", taskID)
		return nil
	},
}

var approveCmd = &cobra.Command{
	Use:               "approve <task_id>",
	Short:             "Approve a task awaiting approval",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeTaskIDs(task.StatusAwaitingApproval, task.StatusTmux),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid task ID: %s", args[0])
		}

		c := client.New(cfg)
		if err := c.Connect(); err != nil {
			return fmt.Errorf("failed to connect to daemon: %w", err)
		}
		defer c.Close()

		if err := c.ApproveTask(taskID); err != nil {
			return fmt.Errorf("failed to approve task: %w", err)
		}

		fmt.Printf("Task #%d approved and resumed\n", taskID)
		return nil
	},
}

var rejectCmd = &cobra.Command{
	Use:               "reject <task_id>",
	Short:             "Reject a task awaiting approval",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeTaskIDs(task.StatusAwaitingApproval, task.StatusTmux),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid task ID: %s", args[0])
		}

		c := client.New(cfg)
		if err := c.Connect(); err != nil {
			return fmt.Errorf("failed to connect to daemon: %w", err)
		}
		defer c.Close()

		if err := c.RejectTask(taskID); err != nil {
			return fmt.Errorf("failed to reject task: %w", err)
		}

		fmt.Printf("Task #%d rejected\n", taskID)
		return nil
	},
}

var retryCmd = &cobra.Command{
	Use:               "retry <task_id>",
	Short:             "Retry a failed task",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeTaskIDs(task.StatusFailed),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid task ID: %s", args[0])
		}

		c := client.New(cfg)
		if err := c.Connect(); err != nil {
			return fmt.Errorf("failed to connect to daemon: %w", err)
		}
		defer c.Close()

		if err := c.RetryTask(taskID); err != nil {
			return fmt.Errorf("failed to retry task: %w", err)
		}

		fmt.Printf("Task #%d reset for retry\n", taskID)
		return nil
	},
}

var continueCmd = &cobra.Command{
	Use:               "continue <task_id>",
	Short:             "Continue a completed/failed/artifact-missing task",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeTaskIDs(task.StatusCompleted, task.StatusFailed, task.StatusArtifactMissing),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid task ID: %s", args[0])
		}

		c := client.New(cfg)
		if err := c.Connect(); err != nil {
			return fmt.Errorf("failed to connect to daemon: %w", err)
		}
		defer c.Close()

		if err := c.ContinueTask(taskID); err != nil {
			return fmt.Errorf("failed to continue task: %w", err)
		}

		fmt.Printf("Continue session started for task #%d\n", taskID)
		return nil
	},
}

var logsCmd = &cobra.Command{
	Use:               "logs <task_id> [step]",
	Short:             "Show logs for a task",
	Args:              cobra.RangeArgs(1, 2),
	ValidArgsFunction: completeTaskIDs(),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid task ID: %s", args[0])
		}

		step := ""
		if len(args) > 1 {
			step = args[1]
		}

		tail, _ := cmd.Flags().GetInt("tail")

		c := client.New(cfg)
		if err := c.Connect(); err != nil {
			return fmt.Errorf("failed to connect to daemon: %w", err)
		}
		defer c.Close()

		lines, err := c.GetLogs(taskID, step, tail)
		if err != nil {
			return fmt.Errorf("failed to get logs: %w", err)
		}

		if len(lines) == 0 {
			fmt.Println("No logs available")
			return nil
		}

		for _, line := range lines {
			fmt.Println(line)
		}
		return nil
	},
}

var cleanupCmd = &cobra.Command{
	Use:               "cleanup [task_id]",
	Short:             "Remove worktrees for completed/failed tasks",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeTaskIDs(task.StatusCompleted, task.StatusFailed),
	RunE: func(cmd *cobra.Command, args []string) error {
		dbPath := cfg.GetDatabasePath("")
		database, err := db.Open(dbPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer database.Close()

		if len(args) == 1 {
			taskID, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid task ID: %s", args[0])
			}
			return cleanupTask(database, taskID)
		}

		// Clean up all completed/failed tasks
		tasks, err := database.GetAllTasks()
		if err != nil {
			return fmt.Errorf("failed to get tasks: %w", err)
		}

		cleaned := 0
		for _, t := range tasks {
			if t.Status == "completed" || t.Status == "failed" {
				if err := cleanupTask(database, t.ID); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to cleanup task #%d: %v\n", t.ID, err)
				} else {
					cleaned++
				}
			}
		}

		if cleaned == 0 {
			fmt.Println("Nothing to clean up")
		} else {
			fmt.Printf("Cleaned up %d task(s)\n", cleaned)
		}
		return nil
	},
}

func cleanupTask(database *db.DB, taskID int64) error {
	t, err := database.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("task not found: %w", err)
	}

	// Resolve repo root from the task's project
	var repoRoot string
	if proj, err := database.GetProject(t.ProjectID); err == nil {
		repoRoot = proj.Path
	}

	cleaned := false

	if t.WorktreePath != "" && repoRoot != "" {
		if err := gitpkg.RemoveWorktree(repoRoot, t.WorktreePath); err != nil {
			return fmt.Errorf("failed to remove worktree: %w", err)
		}
		if err := database.ClearWorktreePath(taskID); err != nil {
			return fmt.Errorf("failed to clear worktree path: %w", err)
		}
		cleaned = true
	}

	// Remove log directory
	if repoRoot != "" {
		dataDir := filepath.Join(repoRoot, ".sortie")
		logDir := workflow.ProjectLogsDir(dataDir, taskID)
		if err := os.RemoveAll(logDir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to remove log dir for task #%d: %v\n", taskID, err)
		} else {
			cleaned = true
		}
	}

	if cleaned {
		fmt.Printf("Cleaned up task #%d\n", taskID)
	}
	return nil
}

var attachCmd = &cobra.Command{
	Use:               "attach <task_id> [step]",
	Short:             "Attach to a task's tmux session",
	Args:              cobra.RangeArgs(1, 2),
	ValidArgsFunction: completeTaskIDs(task.StatusRunning, task.StatusTmux),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID := args[0]
		if _, err := strconv.ParseInt(taskID, 10, 64); err != nil {
			return fmt.Errorf("invalid task ID: %s", taskID)
		}

		if !tmux.IsAvailable() {
			return fmt.Errorf("tmux is not installed or not in PATH")
		}

		var sessionName string

		if len(args) == 2 {
			// Specific step requested
			step := args[1]
			session := tmux.NewStepSession(taskID, step, "")
			if !session.Exists() {
				return fmt.Errorf("no tmux session found for task #%s step %q", taskID, step)
			}
			sessionName = session.Name
		} else {
			// No step specified — find the most recent session for this task
			prefix := tmux.SessionPrefix + taskID + "-"
			sessions, err := tmux.ListSessions(prefix)
			if err != nil {
				return fmt.Errorf("failed to list tmux sessions: %w", err)
			}
			if len(sessions) == 0 {
				return fmt.Errorf("no tmux sessions found for task #%s", taskID)
			}
			// Attach to the last session (most recent step)
			sessionName = sessions[len(sessions)-1].Name
		}

		attach := tmux.AttachCommand(sessionName)
		attach.Stdin = os.Stdin
		attach.Stdout = os.Stdout
		attach.Stderr = os.Stderr
		return attach.Run()
	},
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func completeTaskIDs(statuses ...task.Status) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		if cfg == nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		dbPath := cfg.GetDatabasePath("")
		database, err := db.Open(dbPath)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		defer database.Close()

		tasks, err := database.GetAllTasks()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		statusFilter := make(map[task.Status]bool, len(statuses))
		for _, s := range statuses {
			statusFilter[s] = true
		}

		var completions []string
		for _, t := range tasks {
			if len(statusFilter) > 0 && !statusFilter[t.Status] {
				continue
			}
			title := t.Title
			if title == "" {
				title = truncateStr(t.Description, 40)
			}
			completions = append(completions, fmt.Sprintf("%d\t[%s] %s", t.ID, t.Status, title))
		}

		return completions, cobra.ShellCompDirectiveNoFileComp
	}
}

func init() {
	daemonStartCmd.Flags().BoolP("foreground", "f", false, "Run daemon in foreground")
	tuiCmd.Flags().BoolP("global", "g", false, "Show tasks from all projects")
	logsCmd.Flags().IntP("tail", "n", 0, "Show only the last N lines")

	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	daemonCmd.AddCommand(daemonStatusCmd)

	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(tuiCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(tasksCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(approveCmd)
	rootCmd.AddCommand(rejectCmd)
	rootCmd.AddCommand(retryCmd)
	rootCmd.AddCommand(continueCmd)
	rootCmd.AddCommand(logsCmd)
	rootCmd.AddCommand(cleanupCmd)
	rootCmd.AddCommand(attachCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
