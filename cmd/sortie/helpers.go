package main

import (
	"fmt"

	"github.com/aface/sortie/internal/db"
	"github.com/aface/sortie/internal/task"
	"github.com/spf13/cobra"
)

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
