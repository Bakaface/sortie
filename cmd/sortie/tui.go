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

		projectID, projectPath, projectName, globalMode := resolveProjectMode(globalFlag)
		return tui.Run(cfg, projectID, projectPath, projectName, globalMode)
	},
}

func resolveProjectMode(globalFlag bool) (projectID int64, projectPath string, projectName string, globalMode bool) {
	if globalFlag {
		return 0, "", "", true
	}

	cwd, err := os.Getwd()
	if err != nil {
		return 0, "", "", true
	}

	if _, err := os.Stat(filepath.Join(cwd, ".sortie.yml")); err != nil {
		// No .sortie.yml — check if we're in a git repo to filter by repo name
		repoRoot, err := gitpkg.GetRepoRoot(cwd)
		if err != nil {
			return 0, "", "", true
		}
		repoName := filepath.Base(repoRoot)
		return 0, "", repoName, true
	}

	repoRoot, err := gitpkg.GetRepoRoot(cwd)
	if err != nil {
		return 0, cwd, "", false
	}

	dbPath := cfg.GetDatabasePath("")
	database, err := db.Open(dbPath)
	if err != nil {
		return 0, repoRoot, "", false
	}
	defer database.Close()

	proj, err := database.GetOrCreateProject(repoRoot)
	if err != nil {
		return 0, repoRoot, "", false
	}

	return proj.ID, repoRoot, "", false
}
