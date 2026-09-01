package winreplace

import (
	"fmt"
	"strings"
)

// utf8BOM prefixes generated PowerShell scripts. Windows PowerShell 5.1 reads
// a BOM-less -File script using the system ANSI code page, which corrupts
// non-ASCII install paths before the script ever runs.
const utf8BOM = "\xef\xbb\xbf"

type scriptSpec struct {
	Source    string
	Target    string
	ParentPID int
	LogPath   string
}

func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// buildPowerShell renders the fallback replacement helper.
//
// The helper is only reached when the in-process rename-aside replacement in
// ReplaceExecutable could not complete. It follows the same non-destructive
// ordering: the current executable is renamed aside, never deleted first, so a
// failed attempt can always put it back. Deleting the target up front used to
// risk leaving the machine with no gitmake.exe at all.
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

# Logged before anything else so the launching process can confirm that this
# script really started. powershell.exe launched with DETACHED_PROCESS exits 0
# without running the script at all, which previously made staged replacement a
# silent no-op.
Write-GitMakeLog "replacement helper started"

# Detached self-upgrade helpers must not race the GitMake process that created
# them. Synchronous installer replacement passes parentPid=0 and skips this wait
# because the installer is running from a different executable path.
if ($parentPid -gt 0) {
    for ($i = 0; $i -lt 600; $i++) {
        if (-not (Get-Process -Id $parentPid -ErrorAction SilentlyContinue)) { break }
        Start-Sleep -Milliseconds 50
    }
}

$dstFull = [IO.Path]::GetFullPath($dst)
$backup = $dstFull + '.old-' + [DateTime]::UtcNow.ToString('yyyyMMddHHmmssfff')

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

# Renaming a running image is allowed on Windows even though deleting or
# overwriting it is not, so the target is moved aside rather than removed. A
# failed attempt restores it immediately; the install directory is never left
# without gitmake.exe.
for ($i = 0; $i -lt 240; $i++) {
    $movedAside = $false
    try {
        if (Test-Path -LiteralPath $dstFull) {
            Move-Item -LiteralPath $dstFull -Destination $backup -Force -ErrorAction Stop
            $movedAside = $true
        }
        Move-Item -LiteralPath $src -Destination $dstFull -Force -ErrorAction Stop
        if ($movedAside) {
            try { Remove-Item -LiteralPath $backup -Force -ErrorAction Stop } catch {
                Write-GitMakeLog ("previous executable is still running; left at " + $backup)
            }
        }
        Write-GitMakeLog "replacement complete"
        exit 0
    } catch {
        Write-GitMakeLog ("replacement retry " + ($i + 1) + ": " + $_.Exception.Message)
        if ($movedAside -and -not (Test-Path -LiteralPath $dstFull)) {
            try {
                Move-Item -LiteralPath $backup -Destination $dstFull -Force -ErrorAction Stop
                Write-GitMakeLog "restored the previous executable after a failed attempt"
            } catch {
                Write-GitMakeLog ("could not restore previous executable: " + $_.Exception.Message)
            }
        }
        # MCP hosts may automatically respawn a stdio server after it exits, so
        # eviction is retried on every attempt rather than only once. It runs
        # after a failure, never speculatively: rename-aside normally succeeds
        # without stopping anything.
        Stop-GitMakeAtTarget
        Start-Sleep -Milliseconds 250
    }
}

Write-GitMakeLog "replacement failed after retries"
exit 1
`, psQuote(spec.Source), psQuote(spec.Target), spec.ParentPID, psQuote(spec.LogPath))
}
