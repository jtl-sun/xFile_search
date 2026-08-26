# xFile_search v0.1.25 Security Audit

This release keeps the security-clean architecture introduced in v0.1.12.

- No PowerShell execution.
- No `ExecutionPolicy Bypass`.
- No `rundll32.exe` fallback.
- No hidden browser process launch for preview.
- Delete-to-Recycle-Bin continues to use the Windows Shell API.
- No network access or executable download feature is added.

## v0.1.23 Size metadata

The new Size column uses ordinary local filesystem metadata (`os.Stat`) in the existing background metadata pass that already obtains modified Date. It does not open file contents, execute files, contact the network, or modify files. Header sorting is in-process only.

## v0.1.25 Search history

- Search history is stored locally as plain text in the app data/portable folder.
- No network transmission is used.
- No shell, PowerShell, script engine, or hidden external process is used for history.
- Writes use a local temporary file followed by rename.

## v0.1.25 Shell context menu
- Uses documented Windows Shell COM interfaces (IShellFolder/IContextMenu/IContextMenu2/IContextMenu3).
- Does not invoke PowerShell, cmd.exe, rundll32, script hosts, or ExecutionPolicy bypasses.
- Third-party context-menu extensions already installed/registered in Windows may be loaded by the Shell into the xFile_search process while the menu is open, matching normal Explorer-style behavior. Only install trusted shell extensions.
