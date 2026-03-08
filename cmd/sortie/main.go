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
	daemonStartCmd.Flags().BoolP("foreground", "f", false, "Run daemon in foreground")
	tuiCmd.Flags().BoolP("global", "g", false, "Show tasks from all projects")
	logsCmd.Flags().IntP("tail", "n", 0, "Show only the last N lines")

	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	daemonCmd.AddCommand(daemonStatusCmd)

	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(tuiCmd)
	rootCmd.AddCommand(initCmd)
	createCmd.Flags().StringP("priority", "p", "", "Task priority (low, medium, high, urgent)")
	createCmd.Flags().StringP("branch", "b", "", "Custom branch name template")
	createCmd.Flags().StringP("workflow", "w", "", "Workflow to use")
	createCmd.Flags().Bool("no-worktree", false, "Run task in current directory without creating a worktree")
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
	rootCmd.AddCommand(continueCmd)
	rootCmd.AddCommand(logsCmd)
	rootCmd.AddCommand(cleanupCmd)
	rootCmd.AddCommand(attachCmd)
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(editCmd)
	rootCmd.AddCommand(deleteCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
