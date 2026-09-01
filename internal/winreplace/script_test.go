package winreplace

import (
	"strings"
	"testing"
)

func testScript(parentPID int) string {
	return buildPowerShell(scriptSpec{
		Source:    `C:\Temp\gitmake.exe.new`,
		Target:    `C:\Users\me\AppData\Local\Programs\GitMake\gitmake.exe`,
		ParentPID: parentPID,
		LogPath:   `C:\Temp\gitmake.log`,
	})
}

func TestPowerShellReplacementIsPathScopedAndWaitsForParent(t *testing.T) {
	script := testScript(4242)
	mustContain := []string{
		"$parentPid = 4242",
		"Get-Process -Id $parentPid",
		"function Stop-GitMakeAtTarget",
		"Get-Process -Name 'gitmake'",
		"[string]::Equals($processFull, $dstFull, [StringComparison]::OrdinalIgnoreCase)",
		"Stop-Process -Id $_.Id -Force",
		"Wait-Process -Id $id -Timeout 2",
		"for ($i = 0; $i -lt 240; $i++)",
		"Move-Item -LiteralPath $src -Destination $dstFull -Force",
	}
	for _, want := range mustContain {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q", want)
		}
	}
	if strings.Contains(script, "taskkill /IM gitmake.exe") {
		t.Fatal("replacement helper must not kill every gitmake.exe by image name")
	}
}

// TestReplacementNeverDeletesTheTargetFirst pins the non-destructive ordering.
// The pre-v1.2.6 helper ran `Remove-Item $dst` before `Move-Item`, so a move
// that failed after a successful delete left the machine with no gitmake.exe
// at all.
func TestReplacementNeverDeletesTheTargetFirst(t *testing.T) {
	script := testScript(4242)
	if strings.Contains(script, "Remove-Item -LiteralPath $dst -Force") {
		t.Fatal("helper must move the current executable aside, not delete it before the replacement is in place")
	}
	aside := strings.Index(script, "Move-Item -LiteralPath $dstFull -Destination $backup")
	install := strings.Index(script, "Move-Item -LiteralPath $src -Destination $dstFull")
	if aside < 0 || install < 0 || aside > install {
		t.Fatal("helper must rename the current executable aside before installing the new one")
	}
	if !strings.Contains(script, "restored the previous executable after a failed attempt") {
		t.Fatal("helper must restore the previous executable when an attempt fails")
	}
}

func TestSynchronousReplacementSkipsParentWaitAtRuntime(t *testing.T) {
	script := testScript(0)
	for _, want := range []string{"$parentPid = 0", "if ($parentPid -gt 0)"} {
		if !strings.Contains(script, want) {
			t.Fatalf("synchronous replacement script missing %q", want)
		}
	}
}

func TestPowerShellQuoteEscapesSingleQuote(t *testing.T) {
	got := psQuote(`C:\Users\O'Brien\gitmake.exe`)
	if got != `'C:\Users\O''Brien\gitmake.exe'` {
		t.Fatalf("unexpected quoted path %q", got)
	}
}

// TestHelperLogsBeforeDoingAnything protects the start handshake: Stage proves
// the helper is running by waiting for its first log line, so that line has to
// be written before the parent-wait loop.
func TestHelperLogsBeforeDoingAnything(t *testing.T) {
	script := testScript(4242)
	start := strings.Index(script, `Write-GitMakeLog "replacement helper started"`)
	wait := strings.Index(script, "if ($parentPid -gt 0)")
	if start < 0 || wait < 0 || start > wait {
		t.Fatal("helper must log that it started before waiting for the parent process")
	}
}

func TestReplacementEvictsRespawnedTargetOnEveryRetry(t *testing.T) {
	script := testScript(4242)
	loop := strings.Index(script, "for ($i = 0; $i -lt 240; $i++)")
	if loop < 0 {
		t.Fatal("replacement retry loop is missing")
	}
	body := script[loop:]
	if !strings.Contains(body, "Stop-GitMakeAtTarget") {
		t.Fatal("replacement retry loop must evict exact-target GitMake processes so an MCP respawn cannot win forever")
	}
	// Eviction is a recovery step, not a speculative one: rename-aside
	// normally succeeds without stopping any process.
	retry := strings.Index(body, `Write-GitMakeLog ("replacement retry "`)
	evict := strings.Index(body, "Stop-GitMakeAtTarget")
	if retry < 0 || evict < retry {
		t.Fatal("processes must only be stopped after an attempt has actually failed")
	}
}
