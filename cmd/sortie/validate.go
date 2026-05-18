package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/aface/sortie/internal/config"
	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:   "validate [path]",
	Short: "Validate a .sortie.yml configuration file",
	Long: `Validate a Sortie .sortie.yml configuration file.

Checks YAML syntax, flags unknown top-level fields, and runs the same
workflow validation the daemon performs at load time (loop targets, step
names, summarization strategies, enum values).

With no argument, validates ./.sortie.yml. Otherwise validates the given path.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := resolveConfigPath(args)
		if err != nil {
			return err
		}

		diagnostics, err := config.Diagnose(path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}

		for _, d := range diagnostics {
			fmt.Fprintf(os.Stderr, "%s: %s: %s\n", path, d.Severity, d.Message)
		}

		fmt.Printf("%s is valid\n", path)
		return nil
	},
}

func resolveConfigPath(args []string) (string, error) {
	if len(args) == 1 {
		abs, err := filepath.Abs(args[0])
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(abs); err != nil {
			return "", fmt.Errorf("config file not found: %s", abs)
		}
		return abs, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	path := filepath.Join(cwd, ".sortie.yml")
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("no .sortie.yml found in %s — pass a path explicitly", cwd)
	}
	return path, nil
}
