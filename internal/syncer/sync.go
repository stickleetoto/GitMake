package syncer

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func MirrorSnapshot(source, repo string) error {
	entries, err := os.ReadDir(repo)
	if err != nil {
		return fmt.Errorf("read repository: %w", err)
	}
	for _, e := range entries {
		if strings.EqualFold(e.Name(), ".git") {
			continue
		}
		path := filepath.Join(repo, e.Name())
		makeWritable(path)
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("remove old repository path %q: %w", e.Name(), err)
		}
	}

	entries, err = os.ReadDir(source)
	if err != nil {
		return fmt.Errorf("read snapshot: %w", err)
	}
	for _, e := range entries {
		src := filepath.Join(source, e.Name())
		dst := filepath.Join(repo, e.Name())
		if err := copyTree(src, dst); err != nil {
			return err
		}
	}
	return nil
}

// makeWritable is best-effort cleanup for Windows repositories that contain
// read-only files. os.RemoveAll can fail on those files even though replacing
// them is exactly what the snapshot mirror is supposed to do.
func makeWritable(root string) {
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		mode := info.Mode().Perm() | 0o200
		if d.IsDir() {
			mode |= 0o100
		}
		_ = os.Chmod(path, mode)
		return nil
	})
}

func copyTree(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("snapshot symlink is not allowed: %s", src)
	}
	if info.IsDir() {
		if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := copyTree(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("unsupported snapshot file type: %s", src)
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fs.FileMode(info.Mode().Perm()))
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
