package selfupdate

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func exeExt() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// buildVersionPrinter compiles a throwaway executable that prints version, or
// stays alive when given the "hold" argument so that its image file is held
// open by a running process. Replacement has to be proven against a real
// running image: string assertions over a helper script cannot observe the
// Windows behaviour that broke every staged replacement before v1.2.6.
func buildVersionPrinter(t *testing.T, dir, version string) string {
	t.Helper()
	if testing.Short() {
		t.Skip("process-level replacement test skipped in -short mode")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain unavailable for process-level replacement test")
	}

	src := t.TempDir()
	main := `package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "hold" {
		time.Sleep(30 * time.Second)
		return
	}
	fmt.Print("__VERSION__")
}
`
	main = strings.Replace(main, "__VERSION__", version, 1)
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "go.mod"), []byte("module probe\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "probe-"+version+exeExt())
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = src
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot build probe executable: %v: %s", err, string(combined))
	}
	return out
}

func runVersion(t *testing.T, path string) string {
	t.Helper()
	out, err := exec.Command(path).Output()
	if err != nil {
		t.Fatalf("run %s: %v", path, err)
	}
	return strings.TrimSpace(string(out))
}

// TestReplaceExecutableReplacesAnImageThatIsStillRunning is the acceptance
// test for the updater: after replacement, a fresh invocation of the canonical
// path must report the new version, even though a long-lived process (an MCP
// stdio server, in the field) is still running the old image.
func TestReplaceExecutableReplacesAnImageThatIsStillRunning(t *testing.T) {
	root := t.TempDir()
	installDir := filepath.Join(root, "Programs", "GitMake")
	target := filepath.Join(installDir, "gitmake"+exeExt())

	oldExe := buildVersionPrinter(t, filepath.Join(root, "build-old"), "1.0.0")
	newExe := buildVersionPrinter(t, filepath.Join(root, "build-new"), "2.0.0")

	if _, err := ReplaceExecutable(oldExe, target); err != nil {
		t.Fatalf("initial install: %v", err)
	}
	if got := runVersion(t, target); got != "1.0.0" {
		t.Fatalf("installed version = %q, want 1.0.0", got)
	}

	// Hold the installed image open, the way an MCP host keeps a stdio server
	// running against the installed executable.
	holder := exec.Command(target, "hold")
	if err := holder.Start(); err != nil {
		t.Fatalf("start holder: %v", err)
	}
	defer func() {
		_ = holder.Process.Kill()
		_, _ = holder.Process.Wait()
	}()
	time.Sleep(300 * time.Millisecond)

	res, err := ReplaceExecutable(newExe, target)
	if err != nil {
		t.Fatalf("replace while running: %v", err)
	}
	if !res.Replaced {
		t.Fatal("expected Replaced to report that an existing executable was replaced")
	}
	if got := runVersion(t, target); got != "2.0.0" {
		t.Fatalf("after replacement version = %q, want 2.0.0", got)
	}

	// The upgrade must not require killing anything.
	if holder.ProcessState != nil && holder.ProcessState.Exited() {
		t.Fatal("replacement terminated the process that was running the old image")
	}
	if runtime.GOOS == "windows" {
		if res.Backup == "" {
			t.Fatal("expected the displaced image to be kept while it is still running")
		}
		if _, err := os.Stat(res.Backup); err != nil {
			t.Fatalf("displaced image missing at %s: %v", res.Backup, err)
		}
	}
}

// TestReplaceExecutableNeverLeavesTargetMissing guards the destructive failure
// mode of the pre-v1.2.6 helper, which deleted the installed executable before
// it knew the replacement could be put in place.
func TestReplaceExecutableNeverLeavesTargetMissing(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "gitmake"+exeExt())
	if err := os.WriteFile(target, []byte("original"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := ReplaceExecutable(filepath.Join(dir, "does-not-exist"), target); err == nil {
		t.Fatal("expected replacement from a missing source to fail")
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("target was removed by a failed replacement: %v", err)
	}
	if string(got) != "original" {
		t.Fatalf("target content = %q, want the original executable", string(got))
	}
}

func TestReplaceExecutableInstallsWhenNoTargetExists(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	if err := os.WriteFile(source, []byte("payload"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "nested", "gitmake"+exeExt())

	res, err := ReplaceExecutable(source, target)
	if err != nil {
		t.Fatal(err)
	}
	if res.Replaced {
		t.Fatal("first install must not report a replacement")
	}
	if res.Backup != "" {
		t.Fatalf("first install left a backup at %s", res.Backup)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "payload" {
		t.Fatalf("installed content = %q", string(got))
	}
}

func TestSweepBackupsRemovesDisplacedImages(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "gitmake"+exeExt())
	base := filepath.Base(target)
	for _, name := range []string{base + backupSuffix + "1", base + backupSuffix + "2", base + stageSuffix + "3"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	keep := filepath.Join(dir, "unrelated.txt")
	if err := os.WriteFile(keep, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("current"), 0o755); err != nil {
		t.Fatal(err)
	}

	if removed := SweepBackups(target); removed != 3 {
		t.Fatalf("removed %d displaced images, want 3", removed)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("sweep removed an unrelated file")
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatal("sweep removed the installed executable")
	}
}
