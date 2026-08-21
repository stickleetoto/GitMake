//go:build !windows

package installer

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const profileBegin = "# >>> GitMake PATH >>>"
const profileEnd = "# <<< GitMake PATH <<<"

func InstallSelf() (string, bool, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", false, fmt.Errorf("locate GitMake executable: %w", err)
	}
	return installFrom(exe)
}

func InstallSibling(name string) (string, bool, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", false, err
	}
	src := filepath.Join(filepath.Dir(exe), name)
	return installFrom(src)
}

func installFrom(src string) (string, bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false, err
	}
	dir := filepath.Join(home, ".local", "bin")
	target := filepath.Join(dir, "gitmake")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false, err
	}
	srcAbs, _ := filepath.Abs(src)
	targetAbs, _ := filepath.Abs(target)
	if !samePath(srcAbs, targetAbs, false) {
		if err := copyExecutable(srcAbs, targetAbs); err != nil {
			return "", false, err
		}
	}
	changed, err := ensureProfilePATH(home, dir)
	if err != nil {
		return target, false, err
	}
	return target, changed, nil
}

func copyExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open GitMake executable: %w", err)
	}
	defer in.Close()
	tmp := dst + ".new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	_ = os.Chmod(tmp, 0o755)
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
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
