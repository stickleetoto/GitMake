# GitMake v1.2.3 — Locked-Executable Recovery

GitMake v1.2.3 fixes the Windows executable-lock failure observed when installing or upgrading while another GitMake process — commonly an MCP stdio server — still has the installed executable open.

## Fixed

Previous Windows install logic staged `gitmake.exe.new`, attempted to remove the existing installed executable, and retried the rename. If another GitMake process held `C:\Users\<user>\AppData\Local\Programs\GitMake\gitmake.exe` open, Windows rejected the removal and the command ended with an unhelpful `Access is denied` error.

v1.2.3 now falls back to a detached replacement helper. The helper:

1. waits for the GitMake command that created it to exit;
2. enumerates GitMake processes;
3. stops **only** processes whose executable path exactly matches the installed target;
4. leaves GitMake copies running from Downloads or other directories untouched;
5. retries the file replacement for roughly one minute to tolerate short-lived antivirus/indexer handles;
6. writes a helper log in the system temp directory.

The same helper is now shared by `gitmake install` and Windows self-upgrade replacement.

## Installer UX

When an immediate replacement succeeds, installation behaves as before.

When the target is locked, GitMake now reports that replacement was staged and prints the helper log path instead of reporting a generic permission failure.

## Security boundary

The helper does not use a broad `taskkill /IM gitmake.exe`. Process termination is restricted to the exact normalized installed executable path, case-insensitively on Windows.

## Compatibility

No GitMake publishing interface, config schema, plan format, approval protocol, or MCP tool contract changes in v1.2.3.
