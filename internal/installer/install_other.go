//go:build !windows

package installer

import "fmt"

func InstallSelf() (string, bool, error) {
	return "", false, fmt.Errorf("gitmake install is currently supported on Windows only")
}
func InstallSibling(string) (string, bool, error) {
	return "", false, fmt.Errorf("GitMake Setup is currently supported on Windows only")
}
func IsInstalledOnPath() bool { return false }
func InstallDir() string      { return "" }
