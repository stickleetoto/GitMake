//go:build windows

package selfupdate

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	// CREATE_NO_WINDOW hides the helper console without detaching it.
	//
	// DETACHED_PROCESS must NOT be used here. powershell.exe is a console
	// subsystem application: started with DETACHED_PROCESS it has no console to
	// attach to, and Windows PowerShell exits immediately with code 0 WITHOUT
	// running the -File script. That silently turned every staged replacement
	// into a no-op that still looked successful. See
	// TestStagedHelperActuallyRuns for the regression guard.
	createNoWindow        = 0x08000000
	createNewProcessGroup = 0x00000200

	// helperStartTimeout bounds how long Stage waits for proof that the helper
	// process really began executing the script.
	helperStartTimeout = 10 * time.Second
)

// replacementSeq keeps every staged replacement in this process on its own
// script and log.
var replacementSeq uint64

func prepare(source, target string, parentPID int) (scriptPath, logPath string, err error) {
	source, err = filepath.Abs(source)
	if err != nil {
		return "", "", fmt.Errorf("resolve staged executable: %w", err)
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return "", "", fmt.Errorf("resolve install target: %w", err)
	}
	if _, err := os.Stat(source); err != nil {
		return "", "", fmt.Errorf("staged executable is unavailable: %w", err)
	}

	// The token has to be unique per call, not per process. Keying it on the
	// pid alone made every replacement in one process share a script and a log,
	// so a helper still running from an earlier call could overwrite the log the
	// next call is waiting on, or have its script deleted out from under it.
	// Two GitMake processes replacing at once would have collided the same way.
	//
	// The counter rather than a timestamp: Windows clock granularity is around
	// 15 ms, so consecutive calls read the same nanosecond value.
	token := fmt.Sprintf("%d-%d", os.Getpid(), atomic.AddUint64(&replacementSeq, 1))
	if parentPID == 0 {
		token += "-sync"
	}
	scriptPath = filepath.Join(os.TempDir(), "gitmake-replace-"+token+".ps1")
	logPath = filepath.Join(os.TempDir(), "gitmake-replace-"+token+".log")
	// A stale log would make the start handshake pass without the helper ever
	// running. Unique tokens make that impossible, but removing it costs
	// nothing and keeps the handshake's precondition explicit.
	_ = os.Remove(logPath)
	content := buildPowerShell(scriptSpec{
		Source: source, Target: target, ParentPID: parentPID, LogPath: logPath,
	})
	// Windows PowerShell 5.1 decodes a -File script as the system ANSI code
	// page unless it starts with a BOM. On a Korean (or any non-Latin) Windows
	// install that mangles every non-ASCII character in the embedded paths, and
	// the helper fails with "Illegal characters in path". The BOM forces UTF-8.
	if err := os.WriteFile(scriptPath, append([]byte(utf8BOM), content...), 0o600); err != nil {
		return "", "", fmt.Errorf("write replacement helper: %w", err)
	}
	return scriptPath, logPath, nil
}

func powershell(scriptPath string) *exec.Cmd {
	return exec.Command(
		"powershell.exe",
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy", "Bypass",
		"-File", scriptPath,
	)
}

// ReplaceNow performs a synchronous exact-path Windows replacement. It is
// intended for installers that are running from a different executable than
// target, so the caller itself cannot be one of the target locks.
func ReplaceNow(source, target string) (string, error) {
	scriptPath, logPath, err := prepare(source, target, 0)
	if err != nil {
		return "", err
	}
	defer os.Remove(scriptPath)

	cmd := powershell(scriptPath)
	cmd.Stdin = nil
	out, err := cmd.CombinedOutput()
	if err != nil {
		return logPath, fmt.Errorf("run replacement helper: %w: %s", err, string(out))
	}
	// A zero exit code is not proof of work: verify the helper actually ran.
	if _, statErr := os.Stat(logPath); statErr != nil {
		return logPath, fmt.Errorf("replacement helper exited without running (no log at %s): %s", logPath, string(out))
	}
	return logPath, nil
}

// Stage schedules a Windows executable replacement after the caller exits.
// The helper is deliberately path-scoped: it may stop only GitMake processes
// whose executable path exactly equals target.
//
// Stage returns only after the helper has proven it is running, so a caller
// can never report a staged replacement that silently never started.
func Stage(source, target string, parentPID int) (string, error) {
	scriptPath, logPath, err := prepare(source, target, parentPID)
	if err != nil {
		return "", err
	}

	cmd := powershell(scriptPath)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow | createNewProcessGroup,
	}
	if err := cmd.Start(); err != nil {
		_ = os.Remove(scriptPath)
		return "", fmt.Errorf("start replacement helper: %w", err)
	}
	if err := waitForHelperStart(logPath); err != nil {
		_ = os.Remove(scriptPath)
		return logPath, err
	}
	return logPath, nil
}

// waitForHelperStart blocks until the helper writes its first log line. The
// helper logs before it waits for the parent, so this proves the script is
// executing without waiting for the replacement itself to finish.
func waitForHelperStart(logPath string) error {
	deadline := time.Now().Add(helperStartTimeout)
	for {
		if st, err := os.Stat(logPath); err == nil && st.Size() > 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("replacement helper did not start within %s (no log at %s)", helperStartTimeout, logPath)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
