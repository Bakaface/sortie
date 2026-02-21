package workflow

import (
	"fmt"
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

// ProjectLogsDir returns the path to the logs directory for a task in the project data dir.
func ProjectLogsDir(dataDir string, taskID int64) string {
	return filepath.Join(dataDir, "logs", fmt.Sprintf("%d", taskID))
}

// ProjectLogPath returns the log file path for a specific step in the project data dir.
func ProjectLogPath(dataDir string, taskID int64, stepName string) string {
	return filepath.Join(ProjectLogsDir(dataDir, taskID), stepName+".log")
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

// ImagesDir returns the path to the images directory in a worktree.
func ImagesDir(worktreePath string) string {
	return filepath.Join(worktreePath, ".rtk", "images")
}

// CopyImagesToWorktree copies the given image files into .rtk/images/ in the worktree.
// Returns the list of worktree-relative paths for the copied images.
func CopyImagesToWorktree(worktreePath string, imagePaths []string) ([]string, error) {
	if len(imagePaths) == 0 {
		return nil, nil
	}

	imagesDir := ImagesDir(worktreePath)
	if err := os.MkdirAll(imagesDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create images dir: %w", err)
	}

	var relativePaths []string
	for _, src := range imagePaths {
		name := filepath.Base(src)
		dst := filepath.Join(imagesDir, name)

		data, err := os.ReadFile(src)
		if err != nil {
			return nil, fmt.Errorf("failed to read image %s: %w", src, err)
		}
		if err := os.WriteFile(dst, data, 0644); err != nil {
			return nil, fmt.Errorf("failed to write image %s: %w", dst, err)
		}

		relPath, _ := filepath.Rel(worktreePath, dst)
		relativePaths = append(relativePaths, relPath)
	}

	return relativePaths, nil
}
