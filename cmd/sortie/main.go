package main

import (
	"fmt"
	"os"

	"github.com/aface/sortie/internal/config"
	"github.com/spf13/cobra"
)

var cfg *config.Config

var noProjectRequired = map[string]bool{
	"init":             true,
	"help":             true,
	"completion":       true,
	"__complete":       true,
	"__completeNoDesc": true,
	"start":            true,
	"stop":             true,
	"status":           true,
	"validate":         true,
	"mcp":              true,
	"backfill-context": true,
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
			// The validate command surfaces config errors itself, so don't
			// bubble them up generically here.
			if cmd.Name() == "validate" {
				return nil
			}
			return fmt.Errorf("failed to load config: %w", err)
		}

		if isDaemonSubcommand(cmd) || cmd.Name() == "tui" {
			return nil
		}

		if !noProjectRequired[cmd.Name()] && !cfg.ProjectConfigFound {
			return fmt.Errorf("no .sortie.yml found — run 'sortie init' first")
		}

		return nil
	},
}

func isDaemonSubcommand(cmd *cobra.Command) bool {
	for p := cmd; p != nil; p = p.Parent() {
		if p.Name() == "daemon" {
			return true
		}
	}
	return false
}

func init() {
	tuiCmd.Flags().BoolP("global", "g", false, "Show tasks from all projects")
	logsCmd.Flags().IntP("tail", "n", 0, "Show only the last N lines")
	tasksCmd.Flags().BoolP("json", "j", false, "Output as JSON")
	listCmd.Flags().BoolP("json", "j", false, "Output as JSON")

	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	daemonCmd.AddCommand(daemonStatusCmd)

	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(tuiCmd)
	rootCmd.AddCommand(initCmd)
	createCmd.Flags().StringP("priority", "p", "", "Task priority (low, medium, high, urgent)")
	createCmd.Flags().StringP("branch", "b", "", "Custom branch name template")
	createCmd.Flags().StringP("workflow", "w", "", "Workflow to use")
	createCmd.Flags().StringP("title", "t", "", "Skip AI title generation; use this title directly")
	createCmd.Flags().Bool("no-worktree", false, "Run task in current directory without creating a worktree")
	createCmd.Flags().String("target", "", "Target branch to branch from and merge into (overrides git.base_branch)")
	createCmd.Flags().String("checkout", "", "Check out an existing branch instead of creating a new one")
	editCmd.Flags().StringP("title", "t", "", "New title")
	editCmd.Flags().StringP("description", "d", "", "New description")
	editCmd.Flags().StringP("context", "c", "", "New context")
	editCmd.Flags().StringP("priority", "p", "", "New priority (low, medium, high, urgent)")
	deleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	rootCmd.AddCommand(tasksCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(retryCmd)
	rootCmd.AddCommand(revertCmd)
	rootCmd.AddCommand(continueCmd)
	rootCmd.AddCommand(logsCmd)
	rootCmd.AddCommand(cleanupCmd)
	rootCmd.AddCommand(attachCmd)
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(editCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(detachCmd)
	rootCmd.AddCommand(attachBranchCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(mcpCmd)
	rootCmd.AddCommand(backfillContextCmd)

	dependsOnCmd.AddCommand(dependsOnAddCmd)
	dependsOnCmd.AddCommand(dependsOnRmCmd)
	dependsOnCmd.AddCommand(dependsOnListCmd)
	rootCmd.AddCommand(dependsOnCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
