package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/aface/sortie/internal/client"
	"github.com/aface/sortie/internal/daemon"
	"github.com/aface/sortie/internal/db"
	gitpkg "github.com/aface/sortie/internal/git"
	"github.com/aface/sortie/internal/task"
	"github.com/aface/sortie/internal/tmux"
	"github.com/aface/sortie/internal/workflow"
	"github.com/spf13/cobra"
)

var tasksCmd = &cobra.Command{
	Use:               "tasks [task_id]",
	Short:             "List all tasks or show task detail",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeTaskIDs(),
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonOut, _ := cmd.Flags().GetBool("json")
		if len(args) == 1 {
			return showTaskDetail(args[0], jsonOut)
		}

		c := client.New(cfg)
		if err := c.Connect(); err != nil {
			return listTasksFromDB(jsonOut)
		}
		defer c.Close()

		tasks, err := c.ListTasks()
		if err != nil {
			return fmt.Errorf("failed to list tasks: %w", err)
		}

		if jsonOut {
			return writeJSON(os.Stdout, tasks)
		}

		if len(tasks) == 0 {
			fmt.Println("No tasks found. Create tasks with 'sortie create' or via the TUI.")
			return nil
		}

		rows := make([]taskTableRow, len(tasks))
		for i, t := range tasks {
			title := t.Title
			if title == "" {
				title = truncateStr(t.Description, 50)
			}
			step := t.CurrentStep
			if step == "" {
				step = "-"
			}
			rows[i] = taskTableRow{id: t.ID, status: t.Status, step: step, title: title}
		}
		printTaskTable(rows)

		return nil
	},
}

func writeJSON(w *os.File, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func listTasksFromDB(jsonOut bool) error {
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

	if jsonOut {
		return writeJSON(os.Stdout, tasks)
	}

	if len(tasks) == 0 {
		fmt.Println("No tasks found. Create tasks with 'sortie create' or via the TUI.")
		return nil
	}

	rows := make([]taskTableRow, len(tasks))
	for i, t := range tasks {
		title := t.Title
		if title == "" {
			title = truncateStr(t.Description, 50)
		}
		step := t.CurrentStep
		if step == "" {
			step = "-"
		}
		rows[i] = taskTableRow{id: t.ID, status: string(t.Status), step: step, title: title}
	}
	printTaskTable(rows)
	return nil
}

func showTaskDetail(idStr string, jsonOut bool) error {
	taskID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid task ID: %s", idStr)
	}

	c := client.New(cfg)
	if err := c.Connect(); err != nil {
		return showTaskDetailFromDB(taskID, jsonOut)
	}
	defer c.Close()

	t, err := c.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}

	if jsonOut {
		return writeJSON(os.Stdout, t)
	}
	printTaskDetail(t)
	return nil
}

func showTaskDetailFromDB(taskID int64, jsonOut bool) error {
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
		BranchName:   t.BranchName,
		Branch:       t.Branch,
		Worktree:     t.Worktree,
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
	if jsonOut {
		return writeJSON(os.Stdout, &info)
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
	if t.BranchName != "" {
		fmt.Printf("  Branch tmpl: %s\n", t.BranchName)
	}
	if t.CurrentStep != "" {
		fmt.Printf("  Step:        %s (index %d)\n", t.CurrentStep, t.StepIndex)
	}
	if !t.Worktree {
		fmt.Printf("  Worktree:    off (runs in current directory)\n")
	} else if t.WorktreePath != "" {
		fmt.Printf("  Worktree:    %s\n", t.WorktreePath)
	}
	if len(t.BlockedBy) > 0 {
		fmt.Printf("  Blocked by:  %v\n", t.BlockedBy)
	}
	if len(t.Commits) > 0 {
		fmt.Printf("  Commits:     %v\n", t.Commits)
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

		jsonOut, _ := cmd.Flags().GetBool("json")
		if jsonOut {
			return writeJSON(os.Stdout, agents)
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

var retryCmd = &cobra.Command{
	Use:               "retry <task_id>",
	Short:             "Retry a failed task",
	Long:              "Retry a task. By default the workflow restarts from the first step. Pass --from-step <name> to restart at a specific step while preserving completed work from earlier steps.",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeTaskIDs(task.StatusFailed),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid task ID: %s", args[0])
		}

		fromStep, _ := cmd.Flags().GetString("from-step")

		c := client.New(cfg)
		if err := c.Connect(); err != nil {
			return fmt.Errorf("failed to connect to daemon: %w", err)
		}
		defer c.Close()

		if err := c.RetryTask(taskID, fromStep); err != nil {
			return fmt.Errorf("failed to retry task: %w", err)
		}

		if fromStep != "" {
			fmt.Printf("Task #%d reset for retry from step %q\n", taskID, fromStep)
		} else {
			fmt.Printf("Task #%d reset for retry\n", taskID)
		}
		return nil
	},
}

var revertCmd = &cobra.Command{
	Use:               "revert <task_id>",
	Short:             "Revert all commits made by a task",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeTaskIDs(task.StatusCompleted),
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

		if err := c.RevertTask(taskID); err != nil {
			return fmt.Errorf("failed to revert task: %w", err)
		}

		fmt.Printf("Task #%d reverted\n", taskID)
		return nil
	},
}

var continueCmd = &cobra.Command{
	Use:               "continue <task_id>",
	Short:             "Continue a task (awaiting-approval, completed, or failed)",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeTaskIDs(task.StatusAwaitingApproval, task.StatusTmux, task.StatusCompleted, task.StatusFailed),
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

		if err := c.ContinueTask(taskID, "", ""); err != nil {
			return fmt.Errorf("failed to continue task: %w", err)
		}

		fmt.Printf("Continue session started for task #%d\n", taskID)
		return nil
	},
}

var logsCmd = &cobra.Command{
	Use:               "logs <task_id>",
	Short:             "Show logs for a task",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeTaskIDs(),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid task ID: %s", args[0])
		}

		tail, _ := cmd.Flags().GetInt("tail")

		c := client.New(cfg)
		if err := c.Connect(); err != nil {
			return fmt.Errorf("failed to connect to daemon: %w", err)
		}
		defer c.Close()

		lines, _, err := c.GetLogs(taskID, tail, 0)
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

	var repoRoot string
	if proj, err := database.GetProject(t.ProjectID); err == nil {
		repoRoot = proj.Path
	}

	cleaned := false

	if t.Worktree && t.WorktreePath != "" && repoRoot != "" {
		if err := gitpkg.RemoveWorktree(repoRoot, t.WorktreePath); err != nil {
			return fmt.Errorf("failed to remove worktree: %w", err)
		}
		if err := database.ClearWorktreePath(taskID); err != nil {
			return fmt.Errorf("failed to clear worktree path: %w", err)
		}
		cleaned = true
	}

	// Delete the task branch (but preserve branches the user provided via --checkout)
	if t.Worktree && t.Branch != "" && repoRoot != "" && t.CheckoutBranch == "" {
		if err := gitpkg.ForceDeleteBranch(repoRoot, t.Branch); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to delete branch %s for task #%d: %v\n", t.Branch, taskID, err)
		} else {
			cleaned = true
		}
	}

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

var detachCmd = &cobra.Command{
	Use:   "detach <task_id>",
	Short: "Detach worktree branch so it can be checked out elsewhere",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid task ID: %s", args[0])
		}

		c := client.New(cfg)
		if err := c.Connect(); err != nil {
			return err
		}
		defer c.Close()

		if err := c.DetachBranch(taskID); err != nil {
			return err
		}
		fmt.Printf("Branch detached from task #%d worktree\n", taskID)
		return nil
	},
}

var attachBranchCmd = &cobra.Command{
	Use:   "attach-branch <task_id>",
	Short: "Reattach branch to worktree after detach",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid task ID: %s", args[0])
		}

		c := client.New(cfg)
		if err := c.Connect(); err != nil {
			return err
		}
		defer c.Close()

		if err := c.AttachBranch(taskID); err != nil {
			return err
		}
		fmt.Printf("Branch reattached to task #%d worktree\n", taskID)
		return nil
	},
}

var attachCmd = &cobra.Command{
	Use:               "attach <task_id>",
	Short:             "Attach to a task's tmux session",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeTaskIDs(task.StatusRunning, task.StatusTmux),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID := args[0]
		if _, err := strconv.ParseInt(taskID, 10, 64); err != nil {
			return fmt.Errorf("invalid task ID: %s", taskID)
		}

		if !tmux.IsAvailable() {
			return fmt.Errorf("tmux is not installed or not in PATH")
		}

		session := tmux.NewSession(cfg.Project.Name, taskID, "")
		if !session.Exists() {
			return fmt.Errorf("no tmux session found for task #%s", taskID)
		}

		attach := tmux.AttachCommand(session.Name)
		attach.Stdin = os.Stdin
		attach.Stdout = os.Stdout
		attach.Stderr = os.Stderr
		return attach.Run()
	},
}
