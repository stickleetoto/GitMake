//go:build windows

package installer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gitmake/internal/winreplace"
)

const productDir = "GitMake"

func InstallSelf() (InstallResult, error) {
	exe, err := os.Executable()
	if err != nil {
		return InstallResult{}, fmt.Errorf("locate GitMake executable: %w", err)
	}
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return InstallResult{}, fmt.Errorf("LOCALAPPDATA is not set")
	}
	dir := filepath.Join(local, "Programs", productDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return InstallResult{}, fmt.Errorf("create install directory: %w", err)
	}
	target := filepath.Join(dir, "gitmake.exe")
	// If `gitmake install` is run from the already-installed copy, do not try
	// to replace the executable that Windows is currently running. Just repair
	// or verify the user PATH registration.
	if samePath(exe, target, true) {
		added, err := ensureUserPath(dir)
		if err != nil {
			return InstallResult{}, err
		}
		return InstallResult{Target: target, PathAdded: added}, nil
	}

	tmp := target + ".new"
	if err := copyFile(exe, tmp); err != nil {
		return InstallResult{}, err
	}
	staged, logPath, err := replaceOrStageWindows(tmp, target)
	if err != nil {
		_ = os.Remove(tmp)
		return InstallResult{}, err
	}
	added, err := ensureUserPath(dir)
	if err != nil {
		return InstallResult{}, err
	}
	return InstallResult{
		Target: target, PathAdded: added, ReplacementStaged: staged, ReplacementLog: logPath,
	}, nil
}

func InstallSibling(exePath string) (InstallResult, error) {
	abs, err := filepath.Abs(exePath)
	if err != nil {
		return InstallResult{}, err
	}
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return InstallResult{}, fmt.Errorf("LOCALAPPDATA is not set")
	}
	dir := filepath.Join(local, "Programs", productDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return InstallResult{}, err
	}
	target := filepath.Join(dir, "gitmake.exe")
	tmp := target + ".new"
	if err := copyFile(abs, tmp); err != nil {
		return InstallResult{}, fmt.Errorf("stage gitmake.exe: %w", err)
	}
	staged, logPath, err := replaceOrStageWindows(tmp, target)
	if err != nil {
		_ = os.Remove(tmp)
		return InstallResult{}, fmt.Errorf("install gitmake.exe: %w", err)
	}
	added, err := ensureUserPath(dir)
	if err != nil {
		return InstallResult{}, err
	}
	return InstallResult{
		Target: target, PathAdded: added, ReplacementStaged: staged, ReplacementLog: logPath,
	}, nil
}

func copyFile(source, target string) error {
	src, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open current executable: %w", err)
	}
	defer src.Close()
	dst, err := os.Create(target)
	if err != nil {
		return fmt.Errorf("create staged executable: %w", err)
	}
	if _, err := dst.ReadFrom(src); err != nil {
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

func replaceOrStageWindows(tmp, target string) (bool, string, error) {
	// Fast path for a first install where no target exists.
	if _, err := os.Stat(target); os.IsNotExist(err) {
		if err := os.Rename(tmp, target); err == nil {
			return false, "", nil
		}
	}

	// Windows will not replace a running executable. Try the immediate path
	// first; if an MCP/CLI process has the installed binary open, preserve the
	// staged .new file and let a detached exact-path helper finish after exit.
	removeErr := os.Remove(target)
	if removeErr == nil || os.IsNotExist(removeErr) {
		if err := os.Rename(tmp, target); err == nil {
			return false, "", nil
		}
	}
	// When install/setup is running from Downloads (or any executable other
	// than the installed target), complete replacement synchronously. This
	// gives the user a truthful success result and prevents MCP auto-respawn
	// from winning an asynchronous race after the install command returns.
	if current, currentErr := os.Executable(); currentErr == nil && !samePath(current, target, true) {
		logPath, syncErr := winreplace.ReplaceNow(tmp, target)
		if syncErr == nil {
			return false, logPath, nil
		}
		if removeErr != nil && !os.IsNotExist(removeErr) {
			return false, logPath, fmt.Errorf("replace installed executable (target is locked: %v; synchronous helper failed: %w)", removeErr, syncErr)
		}
		return false, logPath, fmt.Errorf("replace installed executable: %w", syncErr)
	}

	// Self-upgrade from the installed executable cannot replace its own image
	// synchronously on Windows, so preserve the detached-after-exit path.
	logPath, err := winreplace.Stage(tmp, target, os.Getpid())
	if err != nil {
		if removeErr != nil && !os.IsNotExist(removeErr) {
			return false, "", fmt.Errorf("replace installed executable (target is locked: %v; helper failed: %w)", removeErr, err)
		}
		return false, "", fmt.Errorf("replace installed executable: %w", err)
	}
	return true, logPath, nil
}

func ensureUserPath(dir string) (bool, error) {
	escaped := strings.ReplaceAll(dir, "'", "''")
	// Query and write the USER PATH, not only the current process PATH. This
	// survives new terminals and avoids requiring administrator privileges.
	script := fmt.Sprintf(`$d='%s'; $p=[Environment]::GetEnvironmentVariable('Path','User'); if($null -eq $p){$p=''}; $parts=@($p -split ';' | Where-Object { $_ -ne '' }); $exists=$false; foreach($x in $parts){ if($x.Trim().Trim('"').TrimEnd('\') -ieq $d.TrimEnd('\')){$exists=$true} }; if($exists){ exit 10 }; $n=@($parts + $d) -join ';'; [Environment]::SetEnvironmentVariable('Path',$n,'User')`, escaped)
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 10 {
			return false, nil
		}
		return false, fmt.Errorf("add GitMake to user PATH: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return true, nil
}

// GetPathStatus intentionally gathers multiple independent signals. On
// Windows, PowerShell command resolution, Go's exec.LookPath, the current
// process PATH, and the persisted user PATH can temporarily disagree after an
// installer changes PATH. Doctor should explain that state, not report a false
// failure.
func GetPathStatus() PathStatus {
	s := PathStatus{InstallDir: InstallDir()}
	if s.InstallDir != "" {
		s.InstallTarget = filepath.Join(s.InstallDir, "gitmake.exe")
		if st, err := os.Stat(s.InstallTarget); err == nil && !st.IsDir() {
			s.InstalledBinary = true
		}
	}

	if exe, err := os.Executable(); err == nil {
		if abs, e := filepath.Abs(exe); e == nil {
			s.CurrentExecutable = abs
		} else {
			s.CurrentExecutable = exe
		}
	}
	s.CurrentIsInstalledCopy = samePath(s.CurrentExecutable, s.InstallTarget, true)

	// Prefer the explicit .exe spelling, then fall back to the extensionless
	// command. Finally scan PATH ourselves to avoid a false negative caused by
	// LookPath/PATHEXT edge cases.
	for _, name := range []string{"gitmake.exe", "gitmake"} {
		if p, err := exec.LookPath(name); err == nil && p != "" {
			if abs, e := filepath.Abs(p); e == nil {
				p = abs
			}
			s.ResolvedPath = p
			s.CommandAvailable = true
			break
		}
	}
	if !s.CommandAvailable {
		if p := scanProcessPathForGitMake(); p != "" {
			s.ResolvedPath = p
			s.CommandAvailable = true
		}
	}

	s.ProcessPathHasInstall = pathListContains(os.Getenv("PATH"), s.InstallDir, ";", true)
	if userPath, err := readUserPath(); err == nil {
		s.UserPathHasInstall = pathListContains(userPath, s.InstallDir, ";", true)
	}
	return s
}

func scanProcessPathForGitMake() string {
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		dir = strings.TrimSpace(strings.Trim(dir, `"`))
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, "gitmake.exe")
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			if abs, e := filepath.Abs(candidate); e == nil {
				return abs
			}
			return candidate
		}
	}
	return ""
}

func readUserPath() (string, error) {
	script := `[Console]::OutputEncoding=[System.Text.Encoding]::UTF8; [Environment]::GetEnvironmentVariable('Path','User')`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func IsInstalledOnPath() bool { return GetPathStatus().Healthy() }

func InstallDir() string {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return ""
	}
	return filepath.Join(local, "Programs", productDir)
}
