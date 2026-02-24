package git

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const WorktreePrefix = "sortie-task-"

type Worktree struct {
	Path       string
	Branch     string
	RepoRoot   string
	WorktreeDir string
}

func CreateWorktree(repoRoot string, taskID int64, baseBranch, branchName string) (*Worktree, error) {
	if branchName == "" {
		branchName = fmt.Sprintf("%s%d", WorktreePrefix, taskID)
	}
	// Sanitize branch name for use as directory name (replace / with -)
	dirName := strings.ReplaceAll(branchName, "/", "-")
	worktreePath := filepath.Join(repoRoot, ".sortie", "worktrees", dirName)

	if err := os.MkdirAll(filepath.Dir(worktreePath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create worktree directory: %w", err)
	}

	if _, err := os.Stat(worktreePath); err == nil {
		if err := RemoveWorktree(repoRoot, worktreePath); err != nil {
			return nil, fmt.Errorf("failed to remove existing worktree: %w", err)
		}
	}

	if baseBranch == "" {
		baseBranch = getDefaultBranch(repoRoot)
	}

	args := []string{"worktree", "add", "-b", branchName, worktreePath, baseBranch}
	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), "already exists") {
			args = []string{"worktree", "add", worktreePath, branchName}
			cmd = exec.Command("git", args...)
			cmd.Dir = repoRoot
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				return nil, fmt.Errorf("failed to create worktree: %w (stderr: %s)", err, stderr.String())
			}
		} else {
			return nil, fmt.Errorf("failed to create worktree: %w (stderr: %s)", err, stderr.String())
		}
	}

	return &Worktree{
		Path:       worktreePath,
		Branch:     branchName,
		RepoRoot:   repoRoot,
		WorktreeDir: worktreePath,
	}, nil
}

func RemoveWorktree(repoRoot, worktreePath string) error {
	cmd := exec.Command("git", "worktree", "remove", "--force", worktreePath)
	cmd.Dir = repoRoot

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if !strings.Contains(stderr.String(), "is not a working tree") {
			return fmt.Errorf("failed to remove worktree: %w (stderr: %s)", err, stderr.String())
		}
	}

	if _, err := os.Stat(worktreePath); err == nil {
		os.RemoveAll(worktreePath)
	}

	return nil
}

func ListWorktrees(repoRoot string) ([]string, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = repoRoot

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w (stderr: %s)", err, stderr.String())
	}

	var worktrees []string
	lines := strings.Split(stdout.String(), "\n")

	for _, line := range lines {
		if strings.HasPrefix(line, "worktree ") {
			path := strings.TrimPrefix(line, "worktree ")
			if strings.Contains(path, WorktreePrefix) || strings.Contains(path, "sortie-") {
				worktrees = append(worktrees, path)
			}
		}
	}

	return worktrees, nil
}

func CleanupWorktrees(repoRoot string) error {
	cmd := exec.Command("git", "worktree", "prune")
	cmd.Dir = repoRoot

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to prune worktrees: %w (stderr: %s)", err, stderr.String())
	}

	return nil
}

func getDefaultBranch(repoRoot string) string {
	cmd := exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD")
	cmd.Dir = repoRoot

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err == nil {
		ref := strings.TrimSpace(stdout.String())
		parts := strings.Split(ref, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}

	for _, branch := range []string{"main", "master"} {
		cmd := exec.Command("git", "rev-parse", "--verify", branch)
		cmd.Dir = repoRoot
		if cmd.Run() == nil {
			return branch
		}
	}

	return "HEAD"
}

func IsGitRepo(path string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = path

	return cmd.Run() == nil
}

func GetRepoRoot(path string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = path

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("not a git repository: %w (stderr: %s)", err, stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}
