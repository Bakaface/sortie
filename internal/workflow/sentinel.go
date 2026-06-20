package workflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// StopSentinel is the subset of the Claude Code Stop-hook JSON payload that the
// daemon reads back from a sentinel file. The hook writes the full payload (see
// claudeHookCommandTemplate); SessionID and TranscriptPath identify the agent
// session that actually ran the step, which is authoritative — unlike the
// cwd-matched session finder, which can latch onto an unrelated session when
// several agents share a working directory (notably non-worktree mode).
type StopSentinel struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
}

// sentinelMatchesStep reports whether filename is a Stop-hook sentinel written
// for the step whose sanitised name is stepPrefix. The hook names sentinels
// "<stepPrefix>-<timestamp>.json" where <timestamp> is `$(date +%s%N)` — a run
// of digits (with a trailing literal "N" when BSD date lacks %N support).
//
// Matching on the "<prefix>-<digits…>" shape rather than a bare prefix prevents
// a step from claiming a longer sibling's sentinels — e.g. step "reviewing"
// must not match "reviewing-tests-<ts>.json". The remainder after the prefix
// must start with a digit and contain no '-', which a longer step name's
// suffix ("tests-<ts>") never satisfies.
func sentinelMatchesStep(filename, stepPrefix string) bool {
	if filename == "" || filename[0] == '.' {
		return false
	}
	base, ok := strings.CutSuffix(filename, ".json")
	if !ok {
		return false
	}
	rest, ok := strings.CutPrefix(base, stepPrefix+"-")
	if !ok || rest == "" {
		return false
	}
	if rest[0] < '0' || rest[0] > '9' {
		return false
	}
	return !strings.Contains(rest, "-")
}

// latestStepSentinelFile returns the path of the most recent sentinel (by file
// mtime) written for the given step, or ok=false when none exists. mtime is
// used rather than the encoded timestamp so selection is robust regardless of
// whether the platform's date(1) supports %N.
func latestStepSentinelFile(worktreePath, stepName string) (string, bool) {
	if worktreePath == "" {
		return "", false
	}
	dir := StepDoneDir(worktreePath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	prefix := shellSafeStepName(stepName)
	var bestPath string
	var bestFound bool
	var bestMod int64
	for _, e := range entries {
		if e.IsDir() || !sentinelMatchesStep(e.Name(), prefix) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if mod := info.ModTime().UnixNano(); !bestFound || mod >= bestMod {
			bestFound = true
			bestMod = mod
			bestPath = filepath.Join(dir, e.Name())
		}
	}
	return bestPath, bestFound
}

// StepSentinelExists reports whether at least one Stop-hook sentinel for the
// given step is present. Read errors (missing dir, permission denied) are
// treated as "no sentinel" — the daemon's idle fallback remains the safety net.
func StepSentinelExists(worktreePath, stepName string) bool {
	_, ok := latestStepSentinelFile(worktreePath, stepName)
	return ok
}

// LatestStepSentinel parses the most recent Stop-hook sentinel for the given
// step and returns its payload. ok is false when no sentinel exists or it
// cannot be read or parsed.
func LatestStepSentinel(worktreePath, stepName string) (StopSentinel, bool) {
	path, ok := latestStepSentinelFile(worktreePath, stepName)
	if !ok {
		return StopSentinel{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return StopSentinel{}, false
	}
	var s StopSentinel
	if err := json.Unmarshal(data, &s); err != nil {
		return StopSentinel{}, false
	}
	return s, true
}

// ClearStepSentinels removes every Stop-hook sentinel for the given step from
// the worktree's step-done directory. Scoping to the step name leaves sentinels
// for other (concurrent or earlier) steps untouched. Errors are intentionally
// swallowed: a leftover sentinel triggers at most one redundant advance
// attempt, guarded by the daemon's per-task advancing flag.
func ClearStepSentinels(worktreePath, stepName string) {
	if worktreePath == "" {
		return
	}
	dir := StepDoneDir(worktreePath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	prefix := shellSafeStepName(stepName)
	for _, e := range entries {
		if e.IsDir() || !sentinelMatchesStep(e.Name(), prefix) {
			continue
		}
		_ = os.Remove(filepath.Join(dir, e.Name()))
	}
}
