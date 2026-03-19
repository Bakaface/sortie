package workflow

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/aface/sortie/internal/config"
)

// SyncPathsToWorktree syncs the configured paths from srcRoot into dstRoot.
// Copy paths are fully copied (files and directories). Link paths are symlinked.
func SyncPathsToWorktree(srcRoot, dstRoot string, paths config.WorktreeSyncPathsConfig) error {
	for _, p := range paths.Copy {
		if err := copyPath(srcRoot, dstRoot, p); err != nil {
			return err
		}
	}
	for _, p := range paths.Link {
		if err := linkPath(srcRoot, dstRoot, p); err != nil {
			return err
		}
	}
	return nil
}

// copyPath copies a single path (file or directory) from srcRoot to dstRoot.
func copyPath(srcRoot, dstRoot, p string) error {
	srcPath := filepath.Join(srcRoot, p)
	dstPath := filepath.Join(dstRoot, p)

	info, err := os.Lstat(srcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // skip missing paths silently
		}
		return fmt.Errorf("stat %s: %w", p, err)
	}

	if info.IsDir() {
		if err := syncDir(srcPath, dstPath); err != nil {
			return fmt.Errorf("sync dir %s: %w", p, err)
		}
	} else {
		if err := syncFile(srcPath, dstPath, info.Mode()); err != nil {
			return fmt.Errorf("sync file %s: %w", p, err)
		}
	}
	return nil
}

// linkPath creates a symlink at dstRoot/p pointing to srcRoot/p.
// If the destination already exists (e.g. from the worktree checkout), it is removed first.
func linkPath(srcRoot, dstRoot, p string) error {
	srcPath := filepath.Join(srcRoot, p)
	dstPath := filepath.Join(dstRoot, p)

	// Verify source exists
	if _, err := os.Lstat(srcPath); err != nil {
		if os.IsNotExist(err) {
			return nil // skip missing paths silently
		}
		return fmt.Errorf("stat %s: %w", p, err)
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return fmt.Errorf("mkdir for link %s: %w", p, err)
	}

	// Remove existing file/dir/symlink at destination
	if err := os.RemoveAll(dstPath); err != nil {
		return fmt.Errorf("remove existing %s: %w", p, err)
	}

	if err := os.Symlink(srcPath, dstPath); err != nil {
		return fmt.Errorf("symlink %s: %w", p, err)
	}
	return nil
}

// syncDir recursively copies a directory from src to dst, preserving permissions.
func syncDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			return os.MkdirAll(target, info.Mode())
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		return syncFile(path, target, info.Mode())
	})
}

// syncFile copies a single file from src to dst with the given permissions.
func syncFile(src, dst string, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}
