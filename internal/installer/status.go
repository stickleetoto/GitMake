package installer

import (
	"path/filepath"
	"strings"
)

// PathStatus describes GitMake's Windows per-user installation and command
// resolution state. Keeping these signals separate prevents false negatives
// when the registry/user PATH is correct but the current process environment
// is stale (or when exec.LookPath behaves differently from PowerShell).
type PathStatus struct {
	InstallDir             string
	InstallTarget          string
	InstalledBinary        bool
	CurrentExecutable      string
	CurrentIsInstalledCopy bool
	ResolvedPath           string
	CommandAvailable       bool
	ProcessPathHasInstall  bool
	UserPathHasInstall     bool
}

// Healthy means GitMake is installed and should be callable as `gitmake` now
// or after opening a fresh terminal. A current-process PATH refresh is not
// required for the installation itself to be considered healthy.
func (s PathStatus) Healthy() bool {
	if !s.InstalledBinary {
		return false
	}
	return s.CommandAvailable || s.CurrentIsInstalledCopy || s.ProcessPathHasInstall || s.UserPathHasInstall
}

func pathListContains(pathList, target string, separator string, caseInsensitive bool) bool {
	target = normalizePathForCompare(target)
	if target == "" {
		return false
	}
	for _, part := range strings.Split(pathList, separator) {
		part = strings.TrimSpace(strings.Trim(part, `"`))
		if part == "" {
			continue
		}
		part = normalizePathForCompare(part)
		if caseInsensitive {
			if strings.EqualFold(part, target) {
				return true
			}
		} else if part == target {
			return true
		}
	}
	return false
}

func samePath(a, b string, caseInsensitive bool) bool {
	a = normalizePathForCompare(a)
	b = normalizePathForCompare(b)
	if a == "" || b == "" {
		return false
	}
	if caseInsensitive {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func normalizePathForCompare(v string) string {
	v = strings.TrimSpace(strings.Trim(v, `"`))
	if v == "" {
		return ""
	}
	v = filepath.Clean(v)
	v = strings.TrimRight(v, `\/`)
	return v
}
