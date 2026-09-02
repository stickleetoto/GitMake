//go:build !windows

package installer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gitmake/internal/selfupdate"
)

const profileBegin = "# >>> GitMake PATH >>>"
const profileEnd = "# <<< GitMake PATH <<<"

func InstallSelf() (InstallResult, error) {
	exe, err := os.Executable()
	if err != nil {
		return InstallResult{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return InstallResult{}, err
	}
	dir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return InstallResult{}, err
	}
	target := filepath.Join(dir, "gitmake")
	if err := copyExecutable(exe, target); err != nil {
		return InstallResult{}, err
	}
	added, err := ensureProfilePATH(home, dir)
	if err != nil {
		return InstallResult{}, err
	}
	return InstallResult{Target: target, PathAdded: added}, nil
}

func InstallSibling(name string) (InstallResult, error) {
	exe, err := os.Executable()
	if err != nil {
		return InstallResult{}, err
	}
	sibling := filepath.Join(filepath.Dir(exe), name)
	if _, err := os.Stat(sibling); err != nil {
		return InstallResult{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return InstallResult{}, err
	}
	dir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return InstallResult{}, err
	}
	target := filepath.Join(dir, "gitmake")
	if err := copyExecutable(sibling, target); err != nil {
		return InstallResult{}, err
	}
	added, err := ensureProfilePATH(home, dir)
	if err != nil {
		return InstallResult{}, err
	}
	return InstallResult{Target: target, PathAdded: added}, nil
}

// copyExecutable installs src at dst through the same rename-aside sequence
// Windows uses. POSIX allows replacing a running executable's directory entry,
// so no deferred helper is ever needed here, but sharing the primitive means
// one implementation is tested rather than two behaving almost alike.
func copyExecutable(src, dst string) error {
	selfupdate.SweepBackups(dst)
	if _, err := selfupdate.ReplaceExecutable(src, dst); err != nil {
		return fmt.Errorf("install GitMake: %w", err)
	}
	return nil
}

func ensureProfilePATH(home, dir string) (bool, error) {
	if pathListContains(os.Getenv("PATH"), dir, string(os.PathListSeparator), false) {
		return false, nil
	}
	shell := strings.ToLower(os.Getenv("SHELL"))
	profile := filepath.Join(home, ".profile")
	if strings.Contains(shell, "zsh") {
		profile = filepath.Join(home, ".zprofile")
	}
	data, err := os.ReadFile(profile)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	text := string(data)
	if strings.Contains(text, profileBegin) {
		return false, nil
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += profileBegin + "\nexport PATH=\"$HOME/.local/bin:$PATH\"\n" + profileEnd + "\n"
	return true, os.WriteFile(profile, []byte(text), 0o644)
}

func GetPathStatus() PathStatus {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".local", "bin")
	target := filepath.Join(dir, "gitmake")
	s := PathStatus{InstallDir: dir, InstallTarget: target}
	if st, err := os.Stat(target); err == nil && st.Mode().IsRegular() {
		s.InstalledBinary = true
	}
	if exe, err := os.Executable(); err == nil {
		s.CurrentExecutable, _ = filepath.Abs(exe)
		s.CurrentIsInstalledCopy = samePath(s.CurrentExecutable, target, false)
	}
	if found, err := exec.LookPath("gitmake"); err == nil {
		s.CommandAvailable = true
		s.ResolvedPath, _ = filepath.Abs(found)
	}
	s.ProcessPathHasInstall = pathListContains(os.Getenv("PATH"), dir, string(os.PathListSeparator), false)
	s.UserPathHasInstall = s.ProcessPathHasInstall || profileHasGitMake(home)
	return s
}

func profileHasGitMake(home string) bool {
	for _, name := range []string{".profile", ".zprofile"} {
		data, err := os.ReadFile(filepath.Join(home, name))
		if err == nil && strings.Contains(string(data), profileBegin) {
			return true
		}
	}
	return false
}

func IsInstalledOnPath() bool { return GetPathStatus().Healthy() }
func InstallDir() string      { return GetPathStatus().InstallDir }
