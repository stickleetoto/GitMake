//go:build windows

package upgrader

import (
	"fmt"
	"os"

	"gitmake/internal/selfupdate"
)

// applyReplacement installs newExe at target.
//
// The normal path is a synchronous in-process rename-aside, which Windows
// permits even when the target is the image of a running process — including
// this one. Nothing has to be killed and the result is verifiable before the
// command returns. The detached helper survives only as a fallback for the
// rare case where the rename itself is refused, and it is now started in a way
// that actually runs.
func applyReplacement(newExe, target string) (res selfupdate.Result, scheduled bool, helperLog string, err error) {
	selfupdate.SweepBackups(target)

	res, replaceErr := selfupdate.ReplaceExecutable(newExe, target)
	if replaceErr == nil {
		return res, false, "", nil
	}

	logPath, stageErr := selfupdate.Stage(newExe, target, os.Getpid())
	if stageErr != nil {
		return res, false, logPath, fmt.Errorf("replace GitMake executable: %v; deferred helper also failed: %w", replaceErr, stageErr)
	}
	return res, true, logPath, nil
}
