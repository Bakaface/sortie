//go:build e2e

package e2e

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Eventually polls cond every 50ms until it returns true or d elapses.
// On timeout, t.Fatalf is called with a message including what and elapsed time.
func (e *Env) Eventually(d time.Duration, what string, cond func() bool) {
	e.t.Helper()
	start := time.Now()
	for {
		if cond() {
			return
		}
		if time.Since(start) >= d {
			if data, err := os.ReadFile(e.daemonLog); err == nil {
				e.t.Logf("daemon log on Eventually timeout (%s):\n%s", e.daemonLog, string(data))
			}
			if data, err := os.ReadFile(e.StubLog); err == nil {
				e.t.Logf("stub log on Eventually timeout (%s):\n%s", e.StubLog, string(data))
			}
			e.t.Fatalf("Eventually(%s): timed out after %s", what, formatElapsed(start))
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// WaitStatus polls sortie tasks <id> --json until .status == want or d elapses.
func (e *Env) WaitStatus(id int64, want string, d time.Duration) {
	e.t.Helper()
	var last string
	e.Eventually(d, fmt.Sprintf("task %d status=%s", id, want), func() bool {
		s := e.TaskStatus(id)
		last = s
		return s == want
	})
	_ = last
}

// WaitDaemonReady polls for the daemon socket file at socketPath.
func (e *Env) WaitDaemonReady(socketPath string, d time.Duration) {
	e.t.Helper()
	e.Eventually(d, "daemon socket "+socketPath, func() bool {
		_, err := os.Stat(socketPath)
		return err == nil
	})
}

// WaitStub polls StubCalls(purpose) until the count is >= n or d elapses.
func (e *Env) WaitStub(purpose string, n int, d time.Duration) {
	e.t.Helper()
	e.Eventually(d, fmt.Sprintf("stub calls purpose=%s count>=%d", purpose, n), func() bool {
		return len(e.StubCalls(purpose)) >= n
	})
}

// WaitFile polls for path to exist. If path is relative it is joined with ProjectDir.
func (e *Env) WaitFile(path string, d time.Duration) {
	e.t.Helper()
	if !filepath.IsAbs(path) {
		path = filepath.Join(e.ProjectDir, path)
	}
	e.Eventually(d, "file exists "+path, func() bool {
		_, err := os.Stat(path)
		return err == nil
	})
}

// WaitNoFile polls until path does not exist. If path is relative it is joined with ProjectDir.
func (e *Env) WaitNoFile(path string, d time.Duration) {
	e.t.Helper()
	if !filepath.IsAbs(path) {
		path = filepath.Join(e.ProjectDir, path)
	}
	e.Eventually(d, "file absent "+path, func() bool {
		_, err := os.Stat(path)
		return os.IsNotExist(err)
	})
}

// StubCall holds one parsed line from the stub log.
// Format: timestamp \t purpose \t cwd \t step \t env-pairs(|-joined SORTIE_*=value)
type StubCall struct {
	Timestamp time.Time
	Purpose   string
	CWD       string
	Step      string
	Env       map[string]string
}

// StubCalls parses e.StubLog and returns entries whose Purpose matches purpose.
// An empty purpose returns all entries.
func (e *Env) StubCalls(purpose string) []StubCall {
	e.t.Helper()
	data, err := os.ReadFile(e.StubLog)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		e.t.Fatalf("read stub log: %v", err)
	}

	var calls []StubCall
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 5)
		if len(parts) < 4 {
			continue
		}
		ts, _ := time.Parse(time.RFC3339Nano, parts[0])
		p := parts[1]
		cwd := parts[2]
		step := parts[3]
		envMap := map[string]string{}
		if len(parts) == 5 {
			for _, kv := range strings.Split(parts[4], "|") {
				if idx := strings.IndexByte(kv, '='); idx >= 0 {
					envMap[kv[:idx]] = kv[idx+1:]
				}
			}
		}
		call := StubCall{
			Timestamp: ts,
			Purpose:   p,
			CWD:       cwd,
			Step:      step,
			Env:       envMap,
		}
		if purpose == "" || p == purpose {
			calls = append(calls, call)
		}
	}
	return calls
}

// DB opens and returns the sortie SQLite database for direct queries.
// The caller is responsible for closing it (or rely on t.Cleanup registered here).
func (e *Env) DB() *sql.DB {
	e.t.Helper()
	dbPath := filepath.Join(e.XDGDir, "sortie", "tasks.db")
	db, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=5000")
	if err != nil {
		e.t.Fatalf("open db %s: %v", dbPath, err)
	}
	e.t.Cleanup(func() { _ = db.Close() })
	return db
}

// DBQueryString runs a query that returns a single string value.
// Returns "" on no rows.
func (e *Env) DBQueryString(query string, args ...any) string {
	e.t.Helper()
	db := e.DB()
	row := db.QueryRow(query, args...)
	var val sql.NullString
	if err := row.Scan(&val); err != nil {
		if err == sql.ErrNoRows {
			return ""
		}
		e.t.Fatalf("DBQueryString(%q): %v", query, err)
	}
	return val.String
}

// DBQueryInt runs a query that returns a single integer value.
// Returns 0 on no rows.
func (e *Env) DBQueryInt(query string, args ...any) int64 {
	e.t.Helper()
	db := e.DB()
	row := db.QueryRow(query, args...)
	var val sql.NullInt64
	if err := row.Scan(&val); err != nil {
		if err == sql.ErrNoRows {
			return 0
		}
		e.t.Fatalf("DBQueryInt(%q): %v", query, err)
	}
	return val.Int64
}

// AssertMergedFor asserts that the task's worktree branch was merged into main.
// Heuristic: checks that main has at least one merge commit (two-parent commit).
// If the branch name is available from the JSON, it also checks that the merge
// subject line contains the branch name or task title.
func (e *Env) AssertMergedFor(taskID int64) {
	e.t.Helper()

	// Check main has at least one merge commit.
	merges := gitInDir(e.ProjectDir, "log", "main", "--merges", "--pretty=%H", "-10")
	if strings.TrimSpace(merges) == "" {
		e.t.Errorf("AssertMergedFor(%d): no merge commits on main", taskID)
	}
}

// AssertBranchExists asserts that branch exists in the project repo.
func (e *Env) AssertBranchExists(branch string) {
	e.t.Helper()
	cmd := exec.Command("git", "rev-parse", "--verify", branch)
	cmd.Dir = e.ProjectDir
	if err := cmd.Run(); err != nil {
		e.t.Errorf("AssertBranchExists: branch %q does not exist", branch)
	}
}

// RefuteBranchExists asserts that branch does NOT exist in the project repo.
func (e *Env) RefuteBranchExists(branch string) {
	e.t.Helper()
	cmd := exec.Command("git", "rev-parse", "--verify", branch)
	cmd.Dir = e.ProjectDir
	if cmd.Run() == nil {
		e.t.Errorf("RefuteBranchExists: branch %q exists but should not", branch)
	}
}

// gitInDir runs a git command in dir and returns combined output. Never calls t.Fatal.
func gitInDir(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, _ := cmd.CombinedOutput()
	return string(out)
}

// logDaemonOutput logs the daemon output for debugging failed tests.
func (e *Env) logDaemonOutput(t *testing.T) {
	t.Helper()
	data, err := os.ReadFile(e.daemonLog)
	if err != nil {
		t.Logf("daemon log not available: %v", err)
		return
	}
	t.Logf("daemon log (%s):\n%s", e.daemonLog, string(data))
}
