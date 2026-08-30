package winreplace

import (
	"fmt"
	"strings"
)

type scriptSpec struct {
	Source    string
	Target    string
	ParentPID int
	LogPath   string
}

func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func buildPowerShell(spec scriptSpec) string {
	return fmt.Sprintf(`$ErrorActionPreference = 'Stop'
$src = %s
$dst = %s
$parentPid = %d
$log = %s

function Write-GitMakeLog([string]$message) {
    try {
        $stamp = [DateTime]::UtcNow.ToString('o')
        Add-Content -LiteralPath $log -Value ("[$stamp] $message") -Encoding UTF8
    } catch {}
}

Write-GitMakeLog "replacement helper started"

# Never race the GitMake process that created this helper. It may itself be
# running from $dst during a self-upgrade.
for ($i = 0; $i -lt 200; $i++) {
    if (-not (Get-Process -Id $parentPid -ErrorAction SilentlyContinue)) { break }
    Start-Sleep -Milliseconds 50
}

$dstFull = [IO.Path]::GetFullPath($dst)

# A long-lived GitMake MCP stdio process can keep the installed executable
# locked on Windows. Stop only GitMake processes whose executable path is the
# exact install target. Do not kill copies running from Downloads or elsewhere.
Get-Process -Name 'gitmake' -ErrorAction SilentlyContinue | ForEach-Object {
    try {
        $processPath = $_.Path
        if ($processPath) {
            $processFull = [IO.Path]::GetFullPath($processPath)
            if ([string]::Equals($processFull, $dstFull, [StringComparison]::OrdinalIgnoreCase)) {
                Write-GitMakeLog ("stopping locked GitMake process pid=" + $_.Id)
                Stop-Process -Id $_.Id -Force -ErrorAction Stop
            }
        }
    } catch {
        Write-GitMakeLog ("process inspection/stop warning: " + $_.Exception.Message)
    }
}

# Antivirus/indexers can briefly retain a handle even after the owning process
# exits, so retry for up to about one minute.
for ($i = 0; $i -lt 120; $i++) {
    try {
        if (Test-Path -LiteralPath $dst) {
            Remove-Item -LiteralPath $dst -Force -ErrorAction Stop
        }
        Move-Item -LiteralPath $src -Destination $dst -Force -ErrorAction Stop
        Write-GitMakeLog "replacement complete"
        exit 0
    } catch {
        Write-GitMakeLog ("replacement retry " + ($i + 1) + ": " + $_.Exception.Message)
        Start-Sleep -Milliseconds 500
    }
}

Write-GitMakeLog "replacement failed after retries"
exit 1
`, psQuote(spec.Source), psQuote(spec.Target), spec.ParentPID, psQuote(spec.LogPath))
}
