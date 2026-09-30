# xFile_search v0.1.31

Recommended download: **xFile_search_Setup_v0.1.31_x64.exe**.

## What changed

- Added a global **Ctrl+Alt+F** shortcut to bring xFile_search forward, focus the search box, and select the current query.
- Added a Windows **system tray** menu with Show, Hide to Tray, Clear Search / Scope, Reindex, Open Index Folder, Start with Windows, and Exit.
- Added **Start with Windows** as a per-user toggle; no administrator rights are required.
- Direct directory searches now show an explicit scope line such as `Scope: D:\Design\ | Subfolders: ON` so recursive search is visually clear.
- The productivity features run separately from the search/indexing core, and the indexer child process does not start tray/hotkey features.
- Existing v0.1.30 recursive directory search, v0.1.29 bold green **`INDEXING... xx%`**, removable-drive detection, and progressive background indexing remain intact.
- Index format remains **v3**, so no index migration is required.

The Setup installer preserves Index, Logs, Backup, SearchHistory.txt, and user configuration during upgrades.
