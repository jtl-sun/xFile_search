# xFile_search v0.1.32

Recommended download: **xFile_search_Setup_v0.1.32_x64.exe**.

## What changed

- Fixed the case where a recursive directory search found tens of thousands of matches but the first `MaxDisplayResults` entries were mostly folders, making descendant files appear to be missing.
- Direct directory scope searches in **All** mode now keep files visible by balancing the displayed rows (approximately 75% files and 25% folders when both are available).
- Selecting **Files** continues to search every sub-directory recursively and displays file results only.
- The status bar reports the recursive visible mix so it is clear that both file and folder results are present.
- The underlying search still scans the full indexed match set; the change only affects which rows are placed in the capped ListView.
- Index v3 is unchanged. No index migration is required.
- Existing v0.1.31 global hotkey/system tray/startup features, v0.1.30 recursive directory scope, and v0.1.29 bold green **`INDEXING... xx%`** remain intact.

The Setup installer preserves Index, Logs, Backup, SearchHistory.txt, and user configuration during upgrades.
