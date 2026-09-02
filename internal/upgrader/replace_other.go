//go:build !windows

package upgrader

import "gitmake/internal/selfupdate"

// applyReplacement installs newExe at target with the same rename-aside
// sequence used on Windows. POSIX systems allow replacing a running
// executable's directory entry, so no deferred helper is ever required.
func applyReplacement(newExe, target string) (res selfupdate.Result, scheduled bool, helperLog string, err error) {
	selfupdate.SweepBackups(target)
	res, err = selfupdate.ReplaceExecutable(newExe, target)
	if err != nil {
		return res, false, "", err
	}
	return res, false, "", nil
}
