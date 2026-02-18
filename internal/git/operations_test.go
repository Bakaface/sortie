package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initTestRepo creates a temporary git repo with an initial commit and returns its path.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "checkout", "-b", "main"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup %v failed: %v\n%s", args, err, out)
		}
	}

	// Create initial commit
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("init"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "-A"},
		{"git", "commit", "-m", "initial commit"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup %v failed: %v\n%s", args, err, out)
		}
	}

	return dir
}

func TestForceDeleteBranch_AfterSquashMerge(t *testing.T) {
	repo := initTestRepo(t)

	// Create a feature branch with a commit
	for _, args := range [][]string{
		{"git", "checkout", "-b", "feature-branch"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "feature.txt"), []byte("feature"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "-A"},
		{"git", "commit", "-m", "add feature"},
		{"git", "checkout", "main"},
		{"git", "merge", "--squash", "feature-branch"},
		{"git", "commit", "-m", "squash merge feature-branch"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
	}

	// DeleteBranch (-d) should fail because squash merge doesn't track merge history
	err := DeleteBranch(repo, "feature-branch")
	if err == nil {
		t.Fatal("expected DeleteBranch to fail after squash merge, but it succeeded")
	}

	// ForceDeleteBranch (-D) should succeed
	err = ForceDeleteBranch(repo, "feature-branch")
	if err != nil {
		t.Fatalf("ForceDeleteBranch should succeed after squash merge: %v", err)
	}
}

func TestDeleteBranch_FullyMerged(t *testing.T) {
	repo := initTestRepo(t)

	// Create a feature branch with a commit
	for _, args := range [][]string{
		{"git", "checkout", "-b", "feature-branch"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "feature.txt"), []byte("feature"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "-A"},
		{"git", "commit", "-m", "add feature"},
		{"git", "checkout", "main"},
		{"git", "merge", "feature-branch"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
	}

	// DeleteBranch (-d) should succeed for a fully merged branch
	err := DeleteBranch(repo, "feature-branch")
	if err != nil {
		t.Fatalf("DeleteBranch should succeed for fully merged branch: %v", err)
	}
}

func TestForceDeleteBranch_NonExistent(t *testing.T) {
	repo := initTestRepo(t)

	err := ForceDeleteBranch(repo, "nonexistent-branch")
	if err == nil {
		t.Fatal("expected ForceDeleteBranch to fail for nonexistent branch")
	}
}
