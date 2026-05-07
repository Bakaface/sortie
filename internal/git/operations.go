package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
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

// CleanRepoState forcibly restores a repository's working tree and index to a
// clean state. It aborts any in-progress merge, then hard-resets to HEAD.
// Returns an error only if the repo is still dirty after cleanup.
func CleanRepoState(repoRoot string) error {
	// Abort any in-progress merge first (handles conflicted/half-merged index)
	abortCmd := exec.Command("git", "merge", "--abort")
	abortCmd.Dir = repoRoot
	abortCmd.Run() // ignore error — no merge in progress is fine

	// Hard reset index and working tree
	resetCmd := exec.Command("git", "reset", "--hard", "HEAD")
	resetCmd.Dir = repoRoot
	if err := resetCmd.Run(); err != nil {
		return fmt.Errorf("git reset --hard failed: %w", err)
	}

	// Verify the cleanup actually worked
	dirty, err := HasChanges(repoRoot)
	if err != nil {
		return fmt.Errorf("failed to verify clean state: %w", err)
	}
	if dirty {
		return fmt.Errorf("repo still dirty after cleanup")
	}
	return nil
}

// MergeBranch merges the task branch into baseBranch with `--no-ff`, preserving
// the task branch's individual commits in the base branch's history. The merge
// always produces a merge commit (even when a fast-forward would be possible)
// so the task's commit lineage stays addressable via the second parent.
func MergeBranch(repoRoot, branch, baseBranch, commitMsg string) error {
	// Safety net: if we exit without a successful commit, ensure we don't
	// leave staged changes on the base branch (the root cause of the race
	// condition where parallel merges leave pending changes on main).
	committed := false
	defer func() {
		if !committed {
			CleanRepoState(repoRoot) // best-effort: abort merge + hard reset
		}
	}()

	// Checkout base branch
	checkoutCmd := exec.Command("git", "checkout", baseBranch)
	checkoutCmd.Dir = repoRoot

	var stderr bytes.Buffer
	checkoutCmd.Stderr = &stderr

	if err := checkoutCmd.Run(); err != nil {
		return fmt.Errorf("git checkout %s failed: %w (stderr: %s)", baseBranch, err, stderr.String())
	}

	// Merge the task branch as-is, forcing a merge commit so individual
	// commits from the task branch remain reachable from the base branch.
	if commitMsg == "" {
		commitMsg = fmt.Sprintf("Merge %s into %s", branch, baseBranch)
	}
	mergeCmd := exec.Command("git", "merge", "--no-ff", "-m", commitMsg, branch)
	mergeCmd.Dir = repoRoot
	stderr.Reset()
	mergeCmd.Stderr = &stderr

	if err := mergeCmd.Run(); err != nil {
		return fmt.Errorf("git merge --no-ff failed: %w (stderr: %s)", err, stderr.String())
	}

	committed = true
	return nil
}

// RebaseBranch rebases a branch onto the target branch using the worktree.
// This updates the branch so it's based on the latest target, reducing merge conflicts.
func RebaseBranch(worktreePath, baseBranch string) error {
	cmd := exec.Command("git", "rebase", baseBranch)
	cmd.Dir = worktreePath

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Abort the failed rebase to restore clean state
		abortCmd := exec.Command("git", "rebase", "--abort")
		abortCmd.Dir = worktreePath
		abortCmd.Run() // best-effort
		return fmt.Errorf("git rebase %s failed: %w (stderr: %s)", baseBranch, err, stderr.String())
	}

	return nil
}

// ListLocalBranches returns a sorted list of local branch names in the given
// repository, excluding the currently checked-out branch.
func ListLocalBranches(repoRoot string) ([]string, error) {
	currentBranch, err := GetCurrentBranch(repoRoot)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command("git", "branch", "--list", "--format=%(refname:short)")
	cmd.Dir = repoRoot

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git branch --list failed: %w (stderr: %s)", err, stderr.String())
	}

	var branches []string
	for _, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && line != currentBranch {
			branches = append(branches, line)
		}
	}

	sort.Strings(branches)
	return branches, nil
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

// DiffStat returns the --stat output for all commits on the current branch
// relative to the given base branch. Returns empty string if no diff is available.
func DiffStat(workDir, baseBranch string) (string, error) {
	// Find the merge base to diff against
	mergeBaseCmd := exec.Command("git", "merge-base", baseBranch, "HEAD")
	mergeBaseCmd.Dir = workDir
	var mbOut, mbErr bytes.Buffer
	mergeBaseCmd.Stdout = &mbOut
	mergeBaseCmd.Stderr = &mbErr
	if err := mergeBaseCmd.Run(); err != nil {
		// Fallback: try diffing against HEAD~1
		diffCmd := exec.Command("git", "diff", "--stat", "HEAD~1")
		diffCmd.Dir = workDir
		var diffOut bytes.Buffer
		diffCmd.Stdout = &diffOut
		if diffErr := diffCmd.Run(); diffErr != nil {
			return "", nil
		}
		return strings.TrimSpace(diffOut.String()), nil
	}

	mergeBase := strings.TrimSpace(mbOut.String())
	diffCmd := exec.Command("git", "diff", "--stat", mergeBase)
	diffCmd.Dir = workDir
	var diffOut, diffErr2 bytes.Buffer
	diffCmd.Stdout = &diffOut
	diffCmd.Stderr = &diffErr2
	if err := diffCmd.Run(); err != nil {
		return "", fmt.Errorf("git diff --stat failed: %w", err)
	}
	return strings.TrimSpace(diffOut.String()), nil
}

// MergeInto merges baseBranch into the current branch in the worktree.
// On clean merge, returns nil. On conflict, returns a non-nil error but does NOT
// abort the merge — the caller should resolve conflicts then call CompleteMerge.
func MergeInto(worktreePath, baseBranch string) error {
	cmd := exec.Command("git", "merge", baseBranch, "--no-edit")
	cmd.Dir = worktreePath

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git merge %s failed: %w (stderr: %s)", baseBranch, err, stderr.String())
	}

	return nil
}

// GetConflictedFiles returns the list of files with unresolved merge conflicts.
// Returns an empty slice if there are no conflicts.
func GetConflictedFiles(workDir string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git diff --diff-filter=U failed: %w (stderr: %s)", err, stderr.String())
	}

	var files []string
	for _, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

// CompleteMerge stages all files and commits with the default merge message.
// Should be called after resolving conflicts from a MergeInto call.
func CompleteMerge(workDir string) error {
	addCmd := exec.Command("git", "add", "-A")
	addCmd.Dir = workDir

	var stderr bytes.Buffer
	addCmd.Stderr = &stderr

	if err := addCmd.Run(); err != nil {
		return fmt.Errorf("git add failed: %w (stderr: %s)", err, stderr.String())
	}

	commitCmd := exec.Command("git", "commit", "--no-edit")
	commitCmd.Dir = workDir
	stderr.Reset()
	commitCmd.Stderr = &stderr

	if err := commitCmd.Run(); err != nil {
		return fmt.Errorf("git commit (merge) failed: %w (stderr: %s)", err, stderr.String())
	}

	return nil
}

// AbortMerge aborts an in-progress merge to restore clean state.
func AbortMerge(workDir string) error {
	cmd := exec.Command("git", "merge", "--abort")
	cmd.Dir = workDir

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git merge --abort failed: %w (stderr: %s)", err, stderr.String())
	}

	return nil
}

// GetLastCommitHash returns the SHA of the most recent commit on the current branch.
func GetLastCommitHash(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get last commit hash: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// RevertCommits performs git revert for each commit hash in reverse order (newest first).
func RevertCommits(dir string, commits []string) error {
	// Revert in reverse order (newest first) to avoid conflicts
	for i := len(commits) - 1; i >= 0; i-- {
		cmd := exec.Command("git", "revert", "--no-edit", commits[i])
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to revert commit %s: %w\n%s", commits[i], err, string(out))
		}
	}
	return nil
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

// conventionalCommitRe matches conventional commit subject lines like "feat: ...", "fix(scope): ...", "feat!: ..."
var conventionalCommitRe = regexp.MustCompile(`^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(\([^)]+\))?!?:\s`)

// titlePrefixes maps common leading verbs in task titles to conventional commit types.
var titlePrefixes = []struct {
	prefix string
	ccType string
}{
	{"fix ", "fix"},
	{"bugfix ", "fix"},
	{"repair ", "fix"},
	{"refactor ", "refactor"},
	{"rework ", "refactor"},
	{"restructure ", "refactor"},
	{"test ", "test"},
	{"document ", "docs"},
	{"style ", "style"},
	{"optimize ", "perf"},
	{"revert ", "revert"},
}

// ConventionalCommitFromTitle converts a freeform task title into a conventional
// commit message. If the title already is a conventional commit, it's returned as-is.
// Otherwise the leading verb is used to infer the type (defaulting to "feat").
func ConventionalCommitFromTitle(title string) string {
	if conventionalCommitRe.MatchString(title) {
		return title
	}

	lower := strings.ToLower(title)
	for _, p := range titlePrefixes {
		if strings.HasPrefix(lower, p.prefix) {
			// Strip the verb prefix and lowercase first char of the remainder
			desc := title[len(p.prefix):]
			if desc == "" {
				desc = title
			}
			return p.ccType + ": " + lowercaseFirst(desc)
		}
	}

	return "feat: " + lowercaseFirst(title)
}

func lowercaseFirst(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	if size == 0 {
		return s
	}
	return string(unicode.ToLower(r)) + s[size:]
}

