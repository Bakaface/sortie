package workflow

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// SyncPathsToWorktree copies the specified paths from srcRoot into dstRoot.
// Each path is relative to srcRoot. Both files and directories are supported.
// Permissions are preserved. Existing files in dstRoot are overwritten.
func SyncPathsToWorktree(srcRoot, dstRoot string, paths []string) error {
	for _, p := range paths {
		srcPath := filepath.Join(srcRoot, p)
		dstPath := filepath.Join(dstRoot, p)

		info, err := os.Lstat(srcPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue // skip missing paths silently
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
