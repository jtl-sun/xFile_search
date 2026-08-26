# xFile_search Setup builder

The setup program is a small native Go executable. It embeds the Windows x64 xFile_search binary, installs per-user under LocalAppData, creates Desktop and Start Menu shortcuts, registers an uninstaller under HKCU, and does not require administrator privileges.

It does not use PowerShell, cmd.exe, network downloads, or elevation.
