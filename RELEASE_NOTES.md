# GitMake v0.3.0

GitMake is now an installable Windows CLI instead of something you need to keep copying beside every project.

## Highlights

- `GitMake-Setup.exe` and `gitmake install`
- Per-user installation with no admin requirement
- Automatic user PATH registration
- `gitmake doctor` environment diagnostics
- `gitmake Project.zip` explicit source selection
- `gitmake init [Project.zip]`
- Cleaner compact CREATE/UPDATE/Release output
- Actionable dependency/auth/Git-identity errors
- `gitmake upgrade` self-update flow from GitHub Releases
- Existing snapshot mirroring, safe ZIP validation, dry-run, and Release automation retained

The normal workflow after installation is simply:

```powershell
gitmake
```
