//go:build windows

package installer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const productDir = "GitMake"

func InstallSelf() (string, bool, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", false, fmt.Errorf("locate GitMake executable: %w", err)
	}
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return "", false, fmt.Errorf("LOCALAPPDATA is not set")
	}
	dir := filepath.Join(local, "Programs", productDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false, fmt.Errorf("create install directory: %w", err)
	}
	target := filepath.Join(dir, "gitmake.exe")
	// If `gitmake install` is run from the already-installed copy, do not try
	// to replace the executable that Windows is currently running. Just repair
	// or verify the user PATH registration.
	if samePath(exe, target, true) {
		added, err := ensureUserPath(dir)
		if err != nil {
			return target, false, err
		}
		return target, added, nil
	}
	src, err := os.Open(exe)
	if err != nil {
		return "", false, fmt.Errorf("open current executable: %w", err)
	}
	defer src.Close()
	tmp := target + ".new"
	dst, err := os.Create(tmp)
	if err != nil {
		return "", false, fmt.Errorf("create installed executable: %w", err)
	}
	if _, err := dst.ReadFrom(src); err != nil {
		dst.Close()
		_ = os.Remove(tmp)
		return "", false, fmt.Errorf("copy executable: %w", err)
	}
	if err := dst.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", false, err
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(target)
		if err2 := os.Rename(tmp, target); err2 != nil {
			_ = os.Remove(tmp)
			return "", false, fmt.Errorf("replace installed executable: %w", err2)
		}
	}
	added, err := ensureUserPath(dir)
	if err != nil {
		return target, false, err
	}
	return target, added, nil
}

func InstallSibling(exePath string) (string, bool, error) {
	abs, err := filepath.Abs(exePath)
	if err != nil {
		return "", false, err
	}
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return "", false, fmt.Errorf("LOCALAPPDATA is not set")
	}
	dir := filepath.Join(local, "Programs", productDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false, err
	}
	target := filepath.Join(dir, "gitmake.exe")
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", false, fmt.Errorf("read gitmake.exe beside setup: %w", err)
	}
	tmp := target + ".new"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return "", false, fmt.Errorf("stage gitmake.exe: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(target)
		if err2 := os.Rename(tmp, target); err2 != nil {
			_ = os.Remove(tmp)
			return "", false, fmt.Errorf("install gitmake.exe: %w", err2)
		}
	}
	added, err := ensureUserPath(dir)
	return target, added, err
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
