# xFile_search v0.1.31 verification notes

The v0.1.31 update keeps the Index v3 file format and search engine unchanged. The new productivity layer is isolated from indexing and uses a separate locked Win32 message-loop thread.

Checks required before merge/release:

- `go test ./...`
- `go vet -unsafeptr=false ./...`
- Windows x64 build
- Global hotkey code compiles only in the GUI process; `--indexer` exits before productivity startup
- Direct directory scope label regression tests pass
- Existing recursive directory search tests pass
- Existing index progress tests pass
- No PowerShell, network download, or hidden browser execution added

Manual smoke test after release:

1. Press `Ctrl+Alt+F` from another application; xFile_search should appear with the search field focused and selected.
2. Enter `D:\Test Folder\`; the scope line should show `Subfolders: ON` and results should include descendants.
3. Right-click the tray icon and test Show / Hide to Tray / Clear Search / Reindex / Open Index Folder.
4. Toggle Start with Windows, reopen the tray menu, and confirm the check mark changes.
5. Verify the green bold `INDEXING... xx%` status still appears during reindexing.
