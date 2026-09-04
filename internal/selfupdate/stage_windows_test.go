//go:build windows

package selfupdate

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// deadPID returns a process id that has already exited, so the helper's
// parent-wait loop finishes immediately.
func deadPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("cmd.exe", "/c", "exit")
	if err := cmd.Run(); err != nil {
		t.Fatalf("spawn short-lived process: %v", err)
	}
	return cmd.ProcessState.Pid()
}

// TestStagedHelperActuallyRunsTheScript is the regression guard for the defect
// that silently disabled every staged replacement from v1.2.3 to v1.2.5.
//
// The helper was launched with DETACHED_PROCESS. powershell.exe is a console
// subsystem application: with no console it exits immediately with status 0
// WITHOUT executing -File. Start() succeeded, the exit code looked clean, and
// GitMake reported "Upgrade staged" while nothing whatsoever had happened. The
// old unit tests only asserted on the generated script text, so they could
// never see it.
//
// This test launches the helper for real and requires observable evidence that
// the script executed.
func TestStagedHelperActuallyRunsTheScript(t *testing.T) {
	if testing.Short() {
		t.Skip("process-level helper test skipped in -short mode")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "gitmake-new.exe")
	target := filepath.Join(dir, "install", "gitmake.exe")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("new-image"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("old-image"), 0o755); err != nil {
		t.Fatal(err)
	}

	logPath, err := Stage(source, target, deadPID(t))
	if err != nil {
		t.Fatalf("staged helper did not start: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(logPath)
		_ = os.Remove(strings.TrimSuffix(logPath, ".log") + ".ps1")
	})

	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("helper produced no log, so it never ran: %v", err)
	}
	if !strings.Contains(string(logged), "replacement helper started") {
		t.Fatalf("helper log does not show the script executing: %q", string(logged))
	}

	deadline := time.Now().Add(30 * time.Second)
	for {
		got, readErr := os.ReadFile(target)
		if readErr == nil && string(got) == "new-image" {
			return
		}
		if time.Now().After(deadline) {
			final, _ := os.ReadFile(logPath)
			t.Fatalf("helper never replaced the target; log:\n%s", string(final))
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// TestEachReplacementGetsItsOwnFiles is the regression guard for a flake that
// only appeared under CI timing. The script and log paths were keyed on the
// process id alone, so every replacement in one process shared them: a helper
// still running from an earlier call could recreate the log a later call was
// waiting on, or lose its script to that call's cleanup. Two GitMake processes
// replacing at the same time would have collided identically.
func TestEachReplacementGetsItsOwnFiles(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "gitmake-new.exe")
	target := filepath.Join(dir, "gitmake.exe")
	if err := os.WriteFile(source, []byte("new-image"), 0o755); err != nil {
		t.Fatal(err)
	}

	seenScripts := map[string]bool{}
	seenLogs := map[string]bool{}
	for i := 0; i < 5; i++ {
		scriptPath, logPath, err := prepare(source, target, 4242)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = os.Remove(scriptPath)
			_ = os.Remove(logPath)
		})
		if seenScripts[scriptPath] {
			t.Fatalf("two replacements share the script %s", scriptPath)
		}
		if seenLogs[logPath] {
			t.Fatalf("two replacements share the log %s", logPath)
		}
		seenScripts[scriptPath] = true
		seenLogs[logPath] = true

		// The handshake waits for this file to appear, so it must not exist yet.
		if _, err := os.Stat(logPath); err == nil {
			t.Fatalf("prepare left a log at %s; the start handshake would prove nothing", logPath)
		}
	}
}
