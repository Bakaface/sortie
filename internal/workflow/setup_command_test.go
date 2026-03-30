package workflow

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRunWorktreeSetupCommandsSequential(t *testing.T) {
	dir := t.TempDir()
	markerFile := filepath.Join(dir, "marker.txt")

	// Commands write sequentially to the same file — order matters
	commands := []string{
		"echo first > " + markerFile,
		"echo second >> " + markerFile,
		"echo third >> " + markerFile,
	}

	err := RunWorktreeSetupCommands(context.Background(), dir, dir, commands)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(markerFile)
	if err != nil {
		t.Fatalf("failed to read marker file: %v", err)
	}

	expected := "first\nsecond\nthird\n"
	if string(data) != expected {
		t.Errorf("expected %q, got %q", expected, string(data))
	}
}

func TestRunWorktreeSetupCommandsStopsOnFailure(t *testing.T) {
	dir := t.TempDir()
	markerFile := filepath.Join(dir, "marker.txt")

	commands := []string{
		"echo first > " + markerFile,
		"false", // fails
		"echo third >> " + markerFile,
	}

	err := RunWorktreeSetupCommands(context.Background(), dir, dir, commands)
	if err == nil {
		t.Fatal("expected error from failing command")
	}

	data, err := os.ReadFile(markerFile)
	if err != nil {
		t.Fatalf("failed to read marker file: %v", err)
	}

	// Only first command should have run
	if string(data) != "first\n" {
		t.Errorf("expected only first command output, got %q", string(data))
	}
}

func TestRunWorktreeSetupCommandsEmpty(t *testing.T) {
	err := RunWorktreeSetupCommands(context.Background(), "/tmp", "/tmp", nil)
	if err != nil {
		t.Fatalf("unexpected error for nil commands: %v", err)
	}

	err = RunWorktreeSetupCommands(context.Background(), "/tmp", "/tmp", []string{})
	if err != nil {
		t.Fatalf("unexpected error for empty commands: %v", err)
	}
}

func TestRunWorktreeSetupCommandsSkipsEmptyStrings(t *testing.T) {
	dir := t.TempDir()
	markerFile := filepath.Join(dir, "marker.txt")

	commands := []string{
		"echo first > " + markerFile,
		"",
		"echo second >> " + markerFile,
	}

	err := RunWorktreeSetupCommands(context.Background(), dir, dir, commands)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(markerFile)
	if err != nil {
		t.Fatalf("failed to read marker file: %v", err)
	}

	expected := "first\nsecond\n"
	if string(data) != expected {
		t.Errorf("expected %q, got %q", expected, string(data))
	}
}

func TestRunWorktreeSetupCommandsTemplateSubstitution(t *testing.T) {
	dir := t.TempDir()
	worktreePath := "/fake/worktree/path"
	markerFile := filepath.Join(dir, "marker.txt")

	commands := []string{
		"echo {{worktree_path}} > " + markerFile,
	}

	err := RunWorktreeSetupCommands(context.Background(), dir, worktreePath, commands)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(markerFile)
	if err != nil {
		t.Fatalf("failed to read marker file: %v", err)
	}

	expected := worktreePath + "\n"
	if string(data) != expected {
		t.Errorf("expected %q, got %q", expected, string(data))
	}
}
