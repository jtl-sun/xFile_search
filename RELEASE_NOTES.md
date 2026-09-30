# xFile_search v0.1.30

Recommended download: **xFile_search_Setup_v0.1.30_x64.exe**.

## What changed

- Directly entering a Windows directory path ending in `\` now searches that directory **recursively**, including all descendant files and folders.
- Directory paths containing spaces are preserved as a single search scope instead of being split into separate query terms.
- Recursive matching is constrained to the selected directory tree, so similarly named sibling folders are not included accidentally.
- The search continues to use the existing memory-mapped Index v3; no UI-thread disk scan and no index-format migration are required.
- Added regression coverage for the reported `_Request Samples\` directory case.
- The v0.1.29 bold green **`INDEXING... xx%`** indicator and progressive background indexing remain unchanged.

The Setup installer preserves Index, Logs, Backup, SearchHistory.txt, and user configuration during upgrades.
