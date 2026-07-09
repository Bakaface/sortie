package git

import (
	"errors"
	"fmt"
	"strings"
)

// ErrWorktreeExists indicates that `git worktree add` failed because the
// target worktree path or branch already exists. CreateWorktree classifies
// git's stderr into this sentinel internally and falls back to adding a
// worktree for the existing branch instead of creating a new one.
var ErrWorktreeExists = errors.New("git: worktree or branch already exists")

// ErrNotAWorktree indicates that `git worktree remove` failed because the
// given path is not a registered worktree (e.g. it was already removed, or
// never existed). RemoveWorktree classifies git's stderr into this sentinel
// internally and treats it as a tolerated no-op — the desired end state (no
// worktree at that path) already holds.
var ErrNotAWorktree = errors.New("git: path is not a working tree")

// classifyGitErr wraps a git command failure, attaching one of the sentinel
// errors above via errors.Is when stderr matches a known needle. This is the
// single place git's stderr text is pattern-matched into a typed error —
// every call site in this package (and any future caller) should branch via
// errors.Is against the sentinel rather than re-matching stderr substrings.
//
// Returns nil if stderr doesn't match a recognized class; callers fall back
// to a generic wrapped error in that case.
func classifyGitErr(err error, stderr string) error {
	switch {
	case strings.Contains(stderr, "already exists"):
		return fmt.Errorf("%w: %v (stderr: %s)", ErrWorktreeExists, err, stderr)
	case strings.Contains(stderr, "is not a working tree"):
		return fmt.Errorf("%w: %v (stderr: %s)", ErrNotAWorktree, err, stderr)
	default:
		return nil
	}
}
