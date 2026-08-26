# Security audit notes

## Scope

This note documents security-relevant behavior in xFile_search v0.1.25.

## Removed / avoided behaviors

The current source does **not** require or intentionally use:

- PowerShell child processes
- `ExecutionPolicy Bypass`
- `rundll32.exe` preview fallbacks
- hidden Edge/Chrome headless rendering
- arbitrary network downloads
- remote command/control behavior

These patterns were deliberately avoided after an earlier development build triggered a heuristic antivirus detection.

## Windows integration that remains

xFile_search uses normal local Windows APIs for desktop integration, including:

- memory-mapped index files
- Windows Preview Handlers for supported document previews
- Shell APIs for opening files and moving supported images to the Recycle Bin
- Explorer Shell context-menu COM interfaces (`IShellFolder` / `IContextMenu`) on right-click
- Shell image/thumbnail support for image previews
- filesystem metadata reads for Size and Date

These operations are local to the machine and do not require network access.

## Runtime files

The application can create local runtime data such as:

- `Index/`
- `Logs/`
- `Backup/`
- `SearchHistory.txt`
- `xFile_search.ini`

These are excluded from the source repository by `.gitignore`.

## Antivirus note

Development binaries are unsigned. Reputation/heuristic products may therefore treat new binaries more cautiously. Users should scan released binaries before use. If a security product reports a specific detection, investigate the binary and source rather than automatically adding a broad antivirus exclusion.
