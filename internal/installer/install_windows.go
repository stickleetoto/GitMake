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
		return "", false, fmt.Errorf("copy executable: %w", err)
	}
	if err := dst.Close(); err != nil {
		return "", false, err
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(target)
		if err2 := os.Rename(tmp, target); err2 != nil {
			return "", false, fmt.Errorf("replace installed executable: %w", err)
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
	old, err := os.Executable()
	if err != nil {
		return "", false, err
	}
	_ = old
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
	if err := os.WriteFile(target, data, 0o755); err != nil {
		return "", false, fmt.Errorf("install gitmake.exe: %w", err)
	}
	added, err := ensureUserPath(dir)
	return target, added, err
}

func ensureUserPath(dir string) (bool, error) {
	escaped := strings.ReplaceAll(dir, "'", "''")
	script := fmt.Sprintf(`$d='%s'; $p=[Environment]::GetEnvironmentVariable('Path','User'); if($null -eq $p){$p=''}; $parts=@($p -split ';' | Where-Object { $_ -ne '' }); $exists=$false; foreach($x in $parts){ if($x.TrimEnd('\') -ieq $d.TrimEnd('\')){$exists=$true} }; if($exists){ exit 10 }; $n=@($parts + $d) -join ';'; [Environment]::SetEnvironmentVariable('Path',$n,'User')`, escaped)
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 10 {
			return false, nil
		}
		return false, fmt.Errorf("add GitMake to user PATH: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return true, nil
}

func IsInstalledOnPath() bool {
	p, err := exec.LookPath("gitmake")
	if err != nil {
		return false
	}
	return p != ""
}

func InstallDir() string {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return ""
	}
	return filepath.Join(local, "Programs", productDir)
}
