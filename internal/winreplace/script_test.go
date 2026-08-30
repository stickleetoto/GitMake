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
		"Get-Process -Name 'gitmake'",
		"[string]::Equals($processFull, $dstFull, [StringComparison]::OrdinalIgnoreCase)",
		"Stop-Process -Id $_.Id -Force",
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

func TestPowerShellQuoteEscapesSingleQuote(t *testing.T) {
	got := psQuote(`C:\\Users\\O'Brien\\gitmake.exe`)
	if got != `'C:\\Users\\O''Brien\\gitmake.exe'` {
		t.Fatalf("unexpected quoted path %q", got)
	}
}
