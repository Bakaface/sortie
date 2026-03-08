package main

import (
	"os"
	"path/filepath"

	gitpkg "github.com/aface/sortie/internal/git"
	"github.com/aface/sortie/internal/db"
	"github.com/aface/sortie/internal/tui"
	"github.com/spf13/cobra"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch the TUI (connects to daemon)",
	RunE: func(cmd *cobra.Command, args []string) error {
		globalFlag, _ := cmd.Flags().GetBool("global")

		projectID, projectPath, projectName, globalMode, defaultWorktree := resolveProjectMode(globalFlag)
		return tui.Run(cfg, projectID, projectPath, projectName, globalMode, defaultWorktree)
	},
}

func resolveProjectMode(globalFlag bool) (projectID int64, projectPath string, projectName string, globalMode bool, defaultWorktree bool) {
	if globalFlag {
		return 0, "", "", true, true
	}

	cwd, err := os.Getwd()
	if err != nil {
		return 0, "", "", true, true
	}

	if _, err := os.Stat(filepath.Join(cwd, ".sortie.yml")); err != nil {
		// No .sortie.yml — check if we're in a git repo to filter by repo name
		repoRoot, err := gitpkg.GetRepoRoot(cwd)
		if err != nil {
			return 0, "", "", true, true
		}
		repoName := filepath.Base(repoRoot)
		return 0, repoRoot, repoName, false, true
	}

	repoRoot, err := gitpkg.GetRepoRoot(cwd)
	if err != nil {
		return 0, cwd, "", false, true
	}

	dbPath := cfg.GetDatabasePath("")
	database, err := db.Open(dbPath)
	if err != nil {
		return 0, repoRoot, "", false, true
	}
	defer database.Close()

	proj, err := database.GetOrCreateProject(repoRoot)
	if err != nil {
		return 0, repoRoot, "", false, true
	}

	return proj.ID, repoRoot, "", false, proj.DefaultWorktree
}
