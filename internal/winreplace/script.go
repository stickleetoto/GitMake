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

# Detached self-upgrade helpers must not race the GitMake process that created
# them. Synchronous installer replacement passes parentPid=0 and skips this wait
# because the installer is running from a different executable path.
if ($parentPid -gt 0) {
    for ($i = 0; $i -lt 200; $i++) {
        if (-not (Get-Process -Id $parentPid -ErrorAction SilentlyContinue)) { break }
        Start-Sleep -Milliseconds 50
    }
}

$dstFull = [IO.Path]::GetFullPath($dst)

function Stop-GitMakeAtTarget {
    $ids = @()
    Get-Process -Name 'gitmake' -ErrorAction SilentlyContinue | ForEach-Object {
        try {
            $processPath = $_.Path
            if ($processPath) {
                $processFull = [IO.Path]::GetFullPath($processPath)
                if ([string]::Equals($processFull, $dstFull, [StringComparison]::OrdinalIgnoreCase)) {
                    $ids += $_.Id
                    Write-GitMakeLog ("stopping locked GitMake process pid=" + $_.Id)
                    Stop-Process -Id $_.Id -Force -ErrorAction Stop
                }
            }
        } catch {
            Write-GitMakeLog ("process inspection/stop warning: " + $_.Exception.Message)
        }
    }

    # Stop-Process requests termination, but image-section/file handles can
    # survive for a short moment. Wait briefly before attempting the move.
    foreach ($id in $ids) {
        try {
            Wait-Process -Id $id -Timeout 2 -ErrorAction SilentlyContinue
        } catch {}
    }
}

# MCP hosts may automatically respawn a stdio server after it exits. Therefore
# process eviction must happen on EVERY replacement attempt, not just once.
# This closes the race where a freshly respawned old GitMake immediately locks
# the target again between retries.
for ($i = 0; $i -lt 240; $i++) {
    Stop-GitMakeAtTarget
    try {
        if (Test-Path -LiteralPath $dst) {
            Remove-Item -LiteralPath $dst -Force -ErrorAction Stop
        }
        Move-Item -LiteralPath $src -Destination $dst -Force -ErrorAction Stop
        Write-GitMakeLog "replacement complete"
        exit 0
    } catch {
        Write-GitMakeLog ("replacement retry " + ($i + 1) + ": " + $_.Exception.Message)
        Start-Sleep -Milliseconds 250
    }
}

Write-GitMakeLog "replacement failed after retries"
exit 1
`, psQuote(spec.Source), psQuote(spec.Target), spec.ParentPID, psQuote(spec.LogPath))
}
