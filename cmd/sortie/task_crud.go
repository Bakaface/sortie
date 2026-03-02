package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/aface/sortie/internal/client"
	"github.com/aface/sortie/internal/daemon"
	"github.com/aface/sortie/internal/task"
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:   "create <description>",
	Short: "Create a new task",
	Long: `Create a new task with the given description.

The description can be provided as a positional argument or via stdin.
The daemon must be running.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var description string
		if len(args) == 1 {
			description = args[0]
		} else {
			// Read from stdin if no argument provided
			stat, err := os.Stdin.Stat()
			if err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
				scanner := bufio.NewScanner(os.Stdin)
				var lines []string
				for scanner.Scan() {
					lines = append(lines, scanner.Text())
				}
				description = strings.Join(lines, "\n")
			}
		}

		description = strings.TrimSpace(description)
		if description == "" {
			return fmt.Errorf("description is required (provide as argument or via stdin)")
		}

		priority, _ := cmd.Flags().GetString("priority")
		if priority != "" && !task.IsValidPriority(priority) {
			return fmt.Errorf("invalid priority %q (valid: low, medium, high, urgent)", priority)
		}

		branch, _ := cmd.Flags().GetString("branch")
		workflow, _ := cmd.Flags().GetString("workflow")

		c := client.New(cfg)
		if err := c.Connect(); err != nil {
			return fmt.Errorf("failed to connect to daemon: %w", err)
		}
		defer c.Close()

		t, err := c.CreateTaskWithOptions(daemon.CreateTaskRequest{
			Description: description,
			Workflow:    workflow,
			Priority:    priority,
			BranchName:  branch,
			ProjectPath: cfg.ProjectDir,
		})
		if err != nil {
			return fmt.Errorf("failed to create task: %w", err)
		}

		fmt.Printf("Task #%d created\n", t.ID)
		return nil
	},
}

var editCmd = &cobra.Command{
	Use:   "edit <task_id>",
	Short: "Edit a task's fields",
	Long: `Edit a task's title, description, context, or priority.

At least one field flag must be provided.`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeTaskIDs(),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid task ID: %s", args[0])
		}

		title, _ := cmd.Flags().GetString("title")
		description, _ := cmd.Flags().GetString("description")
		context, _ := cmd.Flags().GetString("context")
		priority, _ := cmd.Flags().GetString("priority")

		if title == "" && description == "" && context == "" && priority == "" {
			return fmt.Errorf("at least one field flag is required (--title, --description, --context, --priority)")
		}

		if priority != "" && !task.IsValidPriority(priority) {
			return fmt.Errorf("invalid priority %q (valid: low, medium, high, urgent)", priority)
		}

		c := client.New(cfg)
		if err := c.Connect(); err != nil {
			return fmt.Errorf("failed to connect to daemon: %w", err)
		}
		defer c.Close()

		updated := 0

		if title != "" {
			if err := c.UpdateTaskField(taskID, "title", title); err != nil {
				return fmt.Errorf("failed to update title: %w", err)
			}
			updated++
		}

		if description != "" {
			if err := c.UpdateTaskField(taskID, "description", description); err != nil {
				return fmt.Errorf("failed to update description: %w", err)
			}
			updated++
		}

		if context != "" {
			if err := c.UpdateTaskField(taskID, "context", context); err != nil {
				return fmt.Errorf("failed to update context: %w", err)
			}
			updated++
		}

		if priority != "" {
			if err := c.UpdateTaskPriority(taskID, priority); err != nil {
				return fmt.Errorf("failed to update priority: %w", err)
			}
			updated++
		}

		fmt.Printf("Task #%d updated (%d field(s))\n", taskID, updated)
		return nil
	},
}

var deleteCmd = &cobra.Command{
	Use:               "delete <task_id>",
	Short:             "Delete a task",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeTaskIDs(),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid task ID: %s", args[0])
		}

		yes, _ := cmd.Flags().GetBool("yes")
		if !yes {
			fmt.Printf("Delete task #%d? This will stop any running agent, remove the worktree, and delete the branch. [y/N] ", taskID)
			reader := bufio.NewReader(os.Stdin)
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(strings.ToLower(answer))
			if answer != "y" && answer != "yes" {
				fmt.Println("Cancelled")
				return nil
			}
		}

		c := client.New(cfg)
		if err := c.Connect(); err != nil {
			return fmt.Errorf("failed to connect to daemon: %w", err)
		}
		defer c.Close()

		if err := c.DeleteTask(taskID); err != nil {
			return fmt.Errorf("failed to delete task: %w", err)
		}

		fmt.Printf("Task #%d deleted\n", taskID)
		return nil
	},
}
