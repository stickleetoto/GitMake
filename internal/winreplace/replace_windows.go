//go:build windows

package winreplace

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

const (
	detachedProcess       = 0x00000008
	createNewProcessGroup = 0x00000200
)

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

	token := fmt.Sprintf("%d", os.Getpid())
	if parentPID == 0 {
		token += "-sync"
	}
	scriptPath = filepath.Join(os.TempDir(), "gitmake-replace-"+token+".ps1")
	logPath = filepath.Join(os.TempDir(), "gitmake-replace-"+token+".log")
	content := buildPowerShell(scriptSpec{
		Source: source, Target: target, ParentPID: parentPID, LogPath: logPath,
	})
	if err := os.WriteFile(scriptPath, []byte(content), 0o600); err != nil {
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
	return logPath, nil
}

// Stage schedules a Windows executable replacement after the caller exits.
// The helper is deliberately path-scoped: it may stop only GitMake processes
// whose executable path exactly equals target.
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
		CreationFlags: detachedProcess | createNewProcessGroup,
	}
	if err := cmd.Start(); err != nil {
		_ = os.Remove(scriptPath)
		return "", fmt.Errorf("start replacement helper: %w", err)
	}
	return logPath, nil
}
