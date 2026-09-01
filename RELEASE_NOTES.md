# xFile_search v0.1.28

Recommended download: **xFile_search_Setup_v0.1.28_x64.exe**.

## What changed

- **Drive swap detection:** if another USB/external/local volume is connected under the same letter such as **F:**, xFile_search compares the Windows volume fingerprint and automatically refreshes instead of continuing to trust the old F: index.
- **Live drive monitoring:** insertion, removal and replacement events are detected while xFile_search is open.
- **Changed drives are scanned first** when possible, so the newly connected drive becomes useful sooner.
- **Progressive background indexing:** xFile_search publishes an early partial index (normally after about 5,000 scanned entries, or when the first root finishes sooner). That partial index is immediately searchable while the full scan continues in the background.
- **Clear indexing feedback:** the title bar shows `INDEXING...`, the Reindex button changes to `Indexing...`, a marquee indicator appears at the bottom, and status text shows the current scan path, running item count and skipped count.
- Search result status also says when **INDEXING continues in background**, so a long scan is not mistaken for a frozen program.
- `Roots=auto` now includes accessible local fixed and removable drives. Empty removable/card-reader slots are skipped.
- The existing **Index v3** format is unchanged. On the first v0.1.28 launch, one background refresh may run to establish the new volume fingerprint baseline.
- Includes the v0.1.27 return-focus and Preview-click navigation improvements.

The Setup installer preserves Index, Logs, Backup, SearchHistory.txt, and user configuration during upgrades.
