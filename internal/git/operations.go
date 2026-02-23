package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
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

func MergeBranch(repoRoot, branch, baseBranch, commitMsg string) error {
	// Checkout base branch
	checkoutCmd := exec.Command("git", "checkout", baseBranch)
	checkoutCmd.Dir = repoRoot

	var stderr bytes.Buffer
	checkoutCmd.Stderr = &stderr

	if err := checkoutCmd.Run(); err != nil {
		return fmt.Errorf("git checkout %s failed: %w (stderr: %s)", baseBranch, err, stderr.String())
	}

	// Squash merge the task branch
	mergeCmd := exec.Command("git", "merge", "--squash", branch)
	mergeCmd.Dir = repoRoot
	stderr.Reset()
	mergeCmd.Stderr = &stderr

	if err := mergeCmd.Run(); err != nil {
		// Clean up the failed merge so the working directory is not left dirty
		// for the next merge operation. git reset --hard restores the index and
		// working tree to the last commit on baseBranch.
		resetCmd := exec.Command("git", "reset", "--hard", "HEAD")
		resetCmd.Dir = repoRoot
		resetCmd.Run() // best-effort cleanup
		return fmt.Errorf("git merge --squash failed: %w (stderr: %s)", err, stderr.String())
	}

	// Squash merge stages changes but doesn't commit — create the commit
	if commitMsg == "" {
		commitMsg = fmt.Sprintf("Squash %s into %s", branch, baseBranch)
	}
	commitCmd := exec.Command("git", "commit", "-m", commitMsg)
	commitCmd.Dir = repoRoot
	stderr.Reset()
	commitCmd.Stderr = &stderr

	if err := commitCmd.Run(); err != nil {
		resetCmd := exec.Command("git", "reset", "--hard", "HEAD")
		resetCmd.Dir = repoRoot
		resetCmd.Run() // best-effort cleanup
		return fmt.Errorf("git commit after squash failed: %w (stderr: %s)", err, stderr.String())
	}

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

// GetSquashCommitMessage returns the best commit message to use when squash-merging
// a branch into the base branch. It looks through the branch's commits (newest first)
// and returns the first conventional commit message found. If none match, it returns
// the first commit subject. If the branch has no commits, it returns fallback.
func GetSquashCommitMessage(repoRoot, baseBranch, branch, fallback string) string {
	// Get commit subjects from the branch (newest first)
	cmd := exec.Command("git", "log", "--format=%s", baseBranch+".."+branch)
	cmd.Dir = repoRoot

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil || strings.TrimSpace(stdout.String()) == "" {
		return ConventionalCommitFromTitle(fallback)
	}

	var subjects []string
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			subjects = append(subjects, line)
		}
	}

	if len(subjects) == 0 {
		return ConventionalCommitFromTitle(fallback)
	}

	// Prefer the first conventional commit message found (newest first)
	for _, s := range subjects {
		if conventionalCommitRe.MatchString(s) {
			return s
		}
	}

	// No conventional commit found — convert the most recent subject
	return ConventionalCommitFromTitle(subjects[0])
}
