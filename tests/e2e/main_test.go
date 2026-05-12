//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMain(m *testing.M) {
	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: find repo root: %v\n", err)
		os.Exit(1)
	}
	repoRoot = root

	binPath, err := buildSortieBinary(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: build sortie binary: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(binPath)

	sortieBinPath = binPath
	os.Exit(m.Run())
}

// findRepoRoot walks up from the directory containing this file to find
// the Go module root (directory containing go.mod).
func findRepoRoot() (string, error) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("runtime.Caller failed")
	}
	dir := filepath.Dir(filename)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found walking up from %s", filepath.Dir(filename))
		}
		dir = parent
	}
}

// buildSortieBinary builds the sortie binary into a temp directory and returns
// the path to the compiled binary.
func buildSortieBinary(repoRoot string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "sortie-e2e-bin-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}

	binPath := filepath.Join(tmpDir, "sortie")
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/sortie")
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("go build: %w\n%s", err, out)
	}
	return binPath, nil
}
