package selfupdate

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Result describes the outcome of an executable replacement.
type Result struct {
	// Target is the absolute path that now holds the new executable.
	Target string
	// Backup is the absolute path of the previous image when it could not be
	// deleted yet because a process is still running it. Empty when the old
	// image was removed immediately.
	Backup string
	// Replaced reports whether an existing executable was replaced. False for
	// a first install where no target existed.
	Replaced bool
}

const (
	backupSuffix = ".old-"
	stageSuffix  = ".new-"
)

// ReplaceExecutable installs source at target without ever leaving target
// missing.
//
// Windows refuses to delete or overwrite the file backing a running image, but
// it does permit renaming it. Replacement therefore never deletes the target:
// it renames the running image aside, moves the new image into the canonical
// path, and only then tries to delete the displaced file. Processes that are
// still running the old image keep running from the renamed file, which is the
// correct behaviour — no process has to be killed for an upgrade to succeed,
// including the process performing the upgrade on its own executable.
//
// Every step is a same-directory rename, so it is atomic and cannot strand the
// target on a half-copied file. If any step fails the previous image is put
// back before the error is returned.
func ReplaceExecutable(source, target string) (Result, error) {
	var res Result
	source, err := filepath.Abs(source)
	if err != nil {
		return res, fmt.Errorf("resolve staged executable: %w", err)
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return res, fmt.Errorf("resolve install target: %w", err)
	}
	res.Target = target
	if _, err := os.Stat(source); err != nil {
		return res, fmt.Errorf("staged executable is unavailable: %w", err)
	}
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return res, fmt.Errorf("create install directory: %w", err)
	}

	// Stage inside the destination directory so the final step is a
	// same-volume rename rather than a cross-volume copy.
	staged := fmt.Sprintf("%s%s%d", target, stageSuffix, time.Now().UnixNano())
	if err := copyFile(source, staged); err != nil {
		return res, err
	}

	_, statErr := os.Stat(target)
	targetExists := statErr == nil
	backup := ""
	if targetExists {
		backup = fmt.Sprintf("%s%s%d", target, backupSuffix, time.Now().UnixNano())
		// Renaming a running image is permitted; deleting it is not.
		if err := renameWithRetry(target, backup); err != nil {
			_ = os.Remove(staged)
			return res, fmt.Errorf("move the current executable aside: %w", err)
		}
	}

	if err := renameWithRetry(staged, target); err != nil {
		// Put the previous executable back so the install never disappears.
		if backup != "" {
			_ = renameWithRetry(backup, target)
		}
		_ = os.Remove(staged)
		return res, fmt.Errorf("install the new executable: %w", err)
	}

	res.Replaced = targetExists
	if backup != "" {
		// Fails while another process still runs the old image. That is
		// expected and harmless; the file is swept on a later run.
		if err := os.Remove(backup); err != nil {
			res.Backup = backup
		}
	}
	return res, nil
}

// SweepBackups deletes displaced executables left behind by earlier
// replacements once nothing is running them any more. It reports how many were
// removed and never fails the caller.
func SweepBackups(target string) int {
	target, err := filepath.Abs(target)
	if err != nil {
		return 0
	}
	dir := filepath.Dir(target)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	base := filepath.Base(target)
	removed := 0
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		if !strings.HasPrefix(name, base+backupSuffix) && !strings.HasPrefix(name, base+stageSuffix) {
			continue
		}
		if os.Remove(filepath.Join(dir, name)) == nil {
			removed++
		}
	}
	return removed
}

// renameWithRetry absorbs the brief sharing violations an antivirus scanner or
// the search indexer can cause right after a file is written. It is a short
// bounded wait, not the retry-until-it-works loop that earlier versions relied
// on instead of fixing the actual lifecycle.
func renameWithRetry(from, to string) error {
	const attempts = 20
	var err error
	for i := 0; i < attempts; i++ {
		if err = os.Rename(from, to); err == nil {
			return nil
		}
		if os.IsNotExist(err) {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return err
}

func copyFile(source, target string) error {
	src, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open staged executable: %w", err)
	}
	defer src.Close()
	dst, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return fmt.Errorf("create staged executable: %w", err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		_ = os.Remove(target)
		return fmt.Errorf("copy executable: %w", err)
	}
	if err := dst.Close(); err != nil {
		_ = os.Remove(target)
		return fmt.Errorf("close staged executable: %w", err)
	}
	return nil
}
