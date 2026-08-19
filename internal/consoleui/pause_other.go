//go:build !windows

package consoleui

func LaunchedStandalone() bool { return false }
func Pause()                   {}
