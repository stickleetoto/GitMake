# GitMake v1.2.4 — Respawn-Safe Replacement

GitMake v1.2.4 hardens the Windows locked-executable recovery introduced in v1.2.3.

## Fixed

v1.2.3 could stage `gitmake.exe.new` successfully but still leave the installed executable on the old version when an MCP host automatically restarted its stdio GitMake process. The replacement helper stopped matching processes only once. A host that respawned GitMake during the retry window could immediately lock the same installed `gitmake.exe` again.

v1.2.4 moves exact-path process eviction inside the replacement retry loop. Before every remove/move attempt, the helper:

1. enumerates running `gitmake` processes;
2. selects only processes whose executable path exactly matches the installed target;
3. force-stops those exact-target processes;
4. briefly waits for process termination so Windows can release image/file handles;
5. immediately retries the staged executable move.

This sequence repeats for the recovery window, so MCP auto-respawn no longer defeats later retries.

## Synchronous manual install

When `gitmake install` or `GitMake-Setup.exe` is running from a downloaded release rather than from the installed target itself, v1.2.4 performs the exact-path lock cleanup and replacement synchronously. The install command no longer returns a staged-success message while the installed binary can still be the old version. Detached replacement remains only for true self-upgrade cases where the currently running process is the installed executable that must replace itself after exit.

## Safety boundary

The helper still never uses broad image-name termination. A GitMake copy running from Downloads or another project directory is left untouched. Only the normalized installed target path is eligible for termination.

## Compatibility

No GitMake publishing interface, config schema, plan format, approval protocol, or MCP tool contract changes in v1.2.4.
