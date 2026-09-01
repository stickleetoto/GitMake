//go:build windows

package winreplace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestHelperScriptIsWrittenWithABOM guards a defect that only appears on
// non-English Windows: powershell.exe -File decodes a BOM-less script with the
// system ANSI code page, so a Korean install path arrives at Add-Content as
// mojibake and the helper dies with "Illegal characters in path".
func TestHelperScriptIsWrittenWithABOM(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "gitmake-new.exe")
	if err := os.WriteFile(source, []byte("new-image"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, awkwardDir, "gitmake.exe")

	scriptPath, logPath, err := prepare(source, target, 4242)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Remove(scriptPath)
		_ = os.Remove(logPath)
	})

	raw, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(raw), utf8BOM) {
		t.Fatal("replacement helper script must start with a UTF-8 BOM so Windows PowerShell decodes non-ASCII paths correctly")
	}
}

// TestStagedHelperReplacesInsideANonASCIIPath runs the fallback helper for
// real against a Korean install directory.
func TestStagedHelperReplacesInsideANonASCIIPath(t *testing.T) {
	if testing.Short() {
		t.Skip("process-level helper test skipped in -short mode")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "gitmake-new.exe")
	target := filepath.Join(dir, awkwardDir, "Program Files", "gitmake.exe")
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
		t.Fatalf("staged helper did not start for a non-ASCII target: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(logPath)
		_ = os.Remove(strings.TrimSuffix(logPath, ".log") + ".ps1")
	})

	deadline := time.Now().Add(30 * time.Second)
	for {
		got, readErr := os.ReadFile(target)
		if readErr == nil && string(got) == "new-image" {
			return
		}
		if time.Now().After(deadline) {
			final, _ := os.ReadFile(logPath)
			t.Fatalf("helper never replaced the non-ASCII target; log:\n%s", string(final))
		}
		time.Sleep(100 * time.Millisecond)
	}
}
