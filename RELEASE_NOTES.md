# xFile_search v0.1.29

Recommended download: **xFile_search_Setup_v0.1.29_x64.exe**.

## What changed

- Background indexing now shows a dedicated **bold green `INDEXING... xx%` label** at the bottom of the main window.
- The same percentage is shown in the window title, while the Reindex button continues to show `Indexing...`.
- The existing marquee animation, current scan path, running item count, skipped count, and progressive partial-index search remain active.
- To avoid slowing indexing with a second full filesystem walk, the displayed percentage is a lightweight estimate based on the previous completed index item count. It stays below 100% while scanning and changes to **100% only when indexing completes**.
- All v0.1.28 removable-drive swap detection and progressive background indexing behavior is retained.
- Existing Index v3 files remain compatible; no reindex-format migration is required.

The Setup installer preserves Index, Logs, Backup, SearchHistory.txt, and user configuration during upgrades.
