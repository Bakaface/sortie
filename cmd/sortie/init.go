package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/aface/sortie/internal/config"
	gitpkg "github.com/aface/sortie/internal/git"
	"github.com/spf13/cobra"
)

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
