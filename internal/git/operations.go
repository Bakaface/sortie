package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

func Commit(workDir, message string) error {
	addCmd := exec.Command("git", "add", "-A")
	addCmd.Dir = workDir

	var stderr bytes.Buffer
	addCmd.Stderr = &stderr

	if err := addCmd.Run(); err != nil {
		return fmt.Errorf("git add failed: %w (stderr: %s)", err, stderr.String())
	}

	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = workDir

	var stdout bytes.Buffer
	statusCmd.Stdout = &stdout

	if err := statusCmd.Run(); err != nil {
		return fmt.Errorf("git status failed: %w", err)
	}

	if strings.TrimSpace(stdout.String()) == "" {
		return nil
	}

	commitCmd := exec.Command("git", "commit", "-m", message)
	commitCmd.Dir = workDir
	stderr.Reset()
	commitCmd.Stderr = &stderr

	if err := commitCmd.Run(); err != nil {
		return fmt.Errorf("git commit failed: %w (stderr: %s)", err, stderr.String())
	}

	return nil
}

func GetCurrentBranch(workDir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to get current branch: %w (stderr: %s)", err, stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}

func HasChanges(workDir string) (bool, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = workDir

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("git status failed: %w", err)
	}

	return strings.TrimSpace(stdout.String()) != "", nil
}

func MergeBranch(repoRoot, branch, baseBranch string) error {
	// Checkout base branch
	checkoutCmd := exec.Command("git", "checkout", baseBranch)
	checkoutCmd.Dir = repoRoot

	var stderr bytes.Buffer
	checkoutCmd.Stderr = &stderr

	if err := checkoutCmd.Run(); err != nil {
		return fmt.Errorf("git checkout %s failed: %w (stderr: %s)", baseBranch, err, stderr.String())
	}

	// Merge the task branch
	mergeCmd := exec.Command("git", "merge", "--no-ff", branch, "-m", fmt.Sprintf("Merge branch '%s'", branch))
	mergeCmd.Dir = repoRoot
	stderr.Reset()
	mergeCmd.Stderr = &stderr

	if err := mergeCmd.Run(); err != nil {
		return fmt.Errorf("git merge failed: %w (stderr: %s)", err, stderr.String())
	}

	return nil
}

func DeleteBranch(repoRoot, branch string) error {
	cmd := exec.Command("git", "branch", "-d", branch)
	cmd.Dir = repoRoot

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git branch -d failed: %w (stderr: %s)", err, stderr.String())
	}

	return nil
}

func ForceDeleteBranch(repoRoot, branch string) error {
	cmd := exec.Command("git", "branch", "-D", branch)
	cmd.Dir = repoRoot

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git branch -D failed: %w (stderr: %s)", err, stderr.String())
	}

	return nil
}

// HasMeaningfulChanges checks whether a worktree has any changes (committed or uncommitted)
// beyond the given exclude list. It checks both:
// - Committed changes vs the base (git diff HEAD --name-only)
// - Uncommitted changes (git status --porcelain)
func HasMeaningfulChanges(workDir string, excludeFiles []string) (bool, error) {
	excludeSet := make(map[string]bool, len(excludeFiles))
	for _, f := range excludeFiles {
		excludeSet[f] = true
	}

	// Check uncommitted changes
	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = workDir
	var statusOut bytes.Buffer
	statusCmd.Stdout = &statusOut
	if err := statusCmd.Run(); err != nil {
		return false, fmt.Errorf("git status failed: %w", err)
	}
	for _, line := range strings.Split(statusOut.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Status output format: "XY filename" — extract filename (last field)
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		filename := parts[len(parts)-1]
		if !excludeSet[filename] {
			return true, nil
		}
	}

	// Check committed changes on this branch vs the merge base
	// Use diff against the first parent to see what this branch added
	diffCmd := exec.Command("git", "diff", "HEAD~1", "--name-only")
	diffCmd.Dir = workDir
	var diffOut, diffErr bytes.Buffer
	diffCmd.Stdout = &diffOut
	diffCmd.Stderr = &diffErr
	if err := diffCmd.Run(); err != nil {
		// If HEAD~1 doesn't exist (first commit), that's fine — check log instead
		logCmd := exec.Command("git", "log", "--oneline", "-1")
		logCmd.Dir = workDir
		var logOut bytes.Buffer
		logCmd.Stdout = &logOut
		if logErr := logCmd.Run(); logErr != nil {
			return false, nil
		}
		// If there's at least one commit, consider it has changes
		return strings.TrimSpace(logOut.String()) != "", nil
	}
	for _, line := range strings.Split(diffOut.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !excludeSet[line] {
			return true, nil
		}
	}

	return false, nil
}

func GetLastCommitMessage(workDir string) (string, error) {
	cmd := exec.Command("git", "log", "-1", "--pretty=%B")
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to get last commit: %w (stderr: %s)", err, stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}
