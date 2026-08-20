//go:build windows

package upgrader

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func StageReplacement(newExe string) error {
	current, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate current executable: %w", err)
	}
	current, _ = filepath.Abs(current)
	newExe, _ = filepath.Abs(newExe)
	dir := filepath.Dir(current)
	script := filepath.Join(os.TempDir(), fmt.Sprintf("gitmake-upgrade-%d.cmd", os.Getpid()))
	content := fmt.Sprintf(`@echo off
setlocal
set "SRC=%s"
set "DST=%s"
for /L %%%%i in (1,1,30) do (
  copy /Y "%%SRC%%" "%%DST%%" >nul 2>&1 && goto :done
  timeout /t 1 /nobreak >nul
)
exit /b 1
:done
start "" /D "%s" cmd /c "echo GitMake upgraded successfully. & timeout /t 2 >nul"
del "%%~f0" >nul 2>&1
`, newExe, current, strings.ReplaceAll(dir, "%", "%%"))
	if err := os.WriteFile(script, []byte(content), 0o644); err != nil {
		return err
	}
	cmd := exec.Command("cmd.exe", "/C", "start", "", "/B", script)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start replacement helper: %w", err)
	}
	return nil
}
