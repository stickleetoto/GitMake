package winreplace

import (
	"strings"
	"testing"
)

func TestPowerShellReplacementIsPathScopedAndWaitsForParent(t *testing.T) {
	script := buildPowerShell(scriptSpec{
		Source:    `C:\\Temp\\gitmake.exe.new`,
		Target:    `C:\\Users\\me\\AppData\\Local\\Programs\\GitMake\\gitmake.exe`,
		ParentPID: 4242,
		LogPath:   `C:\\Temp\\gitmake.log`,
	})
	mustContain := []string{
		"$parentPid = 4242",
		"Get-Process -Id $parentPid",
		"function Stop-GitMakeAtTarget",
		"Get-Process -Name 'gitmake'",
		"[string]::Equals($processFull, $dstFull, [StringComparison]::OrdinalIgnoreCase)",
		"Stop-Process -Id $_.Id -Force",
		"Wait-Process -Id $id -Timeout 2",
		"for ($i = 0; $i -lt 240; $i++)",
		"Stop-GitMakeAtTarget",
		"Move-Item -LiteralPath $src -Destination $dst -Force",
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

func TestSynchronousReplacementSkipsParentWaitAtRuntime(t *testing.T) {
	script := buildPowerShell(scriptSpec{
		Source:    `C:\\Temp\\gitmake.exe.new`,
		Target:    `C:\\Users\\me\\AppData\\Local\\Programs\\GitMake\\gitmake.exe`,
		ParentPID: 0,
		LogPath:   `C:\\Temp\\gitmake.log`,
	})
	for _, want := range []string{"$parentPid = 0", "if ($parentPid -gt 0)"} {
		if !strings.Contains(script, want) {
			t.Fatalf("synchronous replacement script missing %q", want)
		}
	}
}

func TestPowerShellQuoteEscapesSingleQuote(t *testing.T) {
	got := psQuote(`C:\\Users\\O'Brien\\gitmake.exe`)
	if got != `'C:\\Users\\O''Brien\\gitmake.exe'` {
		t.Fatalf("unexpected quoted path %q", got)
	}
}

func TestReplacementEvictsRespawnedTargetOnEveryRetry(t *testing.T) {
	script := buildPowerShell(scriptSpec{
		Source:    `C:\\Temp\\gitmake.exe.new`,
		Target:    `C:\\Users\\me\\AppData\\Local\\Programs\\GitMake\\gitmake.exe`,
		ParentPID: 4242,
		LogPath:   `C:\\Temp\\gitmake.log`,
	})
	loop := strings.Index(script, "for ($i = 0; $i -lt 240; $i++)")
	stop := strings.Index(script[loop:], "Stop-GitMakeAtTarget")
	move := strings.Index(script[loop:], "Move-Item -LiteralPath $src -Destination $dst -Force")
	if loop < 0 || stop < 0 || move < 0 || stop > move {
		t.Fatal("replacement retry loop must evict exact-target GitMake processes before every move attempt")
	}
}
