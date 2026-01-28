package workflow

import (
	"os"
	"path/filepath"
)

// ArtifactsDir returns the path to the artifacts directory in a worktree.
func ArtifactsDir(worktreePath string) string {
	return filepath.Join(worktreePath, ".rtk", "artifacts")
}

// LogsDir returns the path to the logs directory in a worktree.
func LogsDir(worktreePath string) string {
	return filepath.Join(worktreePath, ".rtk", "logs")
}

// LogPath returns the log file path for a specific step.
func LogPath(worktreePath, stepName string) string {
	return filepath.Join(LogsDir(worktreePath), stepName+".log")
}

// EnsureRTKDirs creates the .rtk/artifacts and .rtk/logs directories in a worktree.
func EnsureRTKDirs(worktreePath string) error {
	if err := os.MkdirAll(ArtifactsDir(worktreePath), 0755); err != nil {
		return err
	}
	return os.MkdirAll(LogsDir(worktreePath), 0755)
}

// ReadArtifact reads the artifact file for a given step. Returns empty string if not found.
func ReadArtifact(worktreePath, stepName string) (string, error) {
	path := filepath.Join(ArtifactsDir(worktreePath), stepName+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// CollectArtifacts reads artifacts from all prior steps.
func CollectArtifacts(worktreePath string, priorStepNames []string) map[string]string {
	artifacts := make(map[string]string)
	for _, name := range priorStepNames {
		content, err := ReadArtifact(worktreePath, name)
		if err == nil && content != "" {
			artifacts[name] = content
		}
	}
	return artifacts
}
