# Changelog

## 0.1.29
- Added a dedicated **bold green `INDEXING... xx%` status label** at the bottom of the main window so background indexing is visually unmistakable.
- The current indexing percentage is also mirrored in the window title.
- The percentage uses the previous completed index item count as a lightweight estimate, avoiding a full pre-count pass that would make indexing slower.
- Progress is capped at 99% until the worker finishes, then changes to 100%.
- Existing marquee progress, current scan path, running item count, skipped count, partial searchable index, and background indexing behavior remain unchanged.
- Added unit tests for percentage estimation/parsing and index-count hint loading.


## 0.1.28
- Detects replacement of fixed/removable volumes even when the same Windows drive letter is reused (for example F:) by comparing a persisted Windows volume fingerprint.
- Monitors Windows drive arrival/removal/change events and automatically starts a safe background refresh when the indexed volume set changes.
- `Roots=auto` now includes accessible local fixed and removable drives; empty removable/card-reader slots are skipped.
- Changed/swapped volumes are prioritized first during reindexing so useful results become available sooner.
- Added **progressive background indexing**: after an early checkpoint (or the first root completes), a valid partial v3 index is published and becomes searchable while the full scan continues.
- Added an obvious indexing state: `INDEXING...` in the window title, `Indexing...` on the Reindex button, a bottom marquee progress indicator, current scan path, running item count and skipped count.
- Search result status continues to show that indexing is running in the background.
- Captures the drive fingerprint at index-start time so a volume swapped during a long scan triggers another refresh instead of silently accepting a mixed/stale index.
- Existing Index v3 format remains compatible; this release does not require an index-format migration.
- Corrected Windows drive-scope search test fixtures to use real single-backslash Windows paths.
- Includes all v0.1.27 focus/Preview navigation improvements.


## 0.1.27
- Restores the result-list focus when xFile_search is activated again after switching to another application, so Up/Down immediately continues browsing from the current file.
- A normal click on an image Preview now scrolls the matching result row into view, gives it the focused selection highlight, and makes Up/Down immediately available.
- Image drag-to-pan and Preview double-click-to-open behavior remain unchanged.

## 0.1.26
- Added a per-user Windows Setup installer that requires no administrator privileges.
- Setup preserves Index, Logs, Backup, SearchHistory.txt and user configuration during upgrades.
- Added Desktop and Start Menu shortcuts and Windows Installed Apps uninstall registration.
- Added INSTALL.md and INSTALL_KO.md.
- Added automated installer / portable release packaging workflow support.

## 0.1.25

- Added persistent recent-search history for the main search box.
- Added a small history dropdown button beside the search field.
- Recent searches are stored locally in `SearchHistory.txt` in portable mode.
- Duplicate searches move to the top; up to 30 are remembered and the newest 15 are shown.
- Added `Clear Search History`.
- Added Explorer-style Windows Shell context menu on right-click of a result.
- Added `IContextMenu2` / `IContextMenu3` forwarding for Shell submenus and owner-drawn extensions.
- Right-click selects the clicked result before showing the context menu.
- Added safe fallback actions if the Shell context menu cannot be created.

## 0.1.23

- Changed file-list columns to `Name | Path | Size | Date`.
- Size/Date metadata loads in the background to keep search responsive.
- Size header sorts numerically; repeated clicks toggle ascending/descending.
- Preview resizing preserves Size/Date visibility by shrinking Name/Path first.

## 0.1.22

- Added mouse-drag panning for built-in image Preview.
- Mouse-wheel zoom now anchors at the current mouse position.
- Added **1:1** and **Fit Window** image Preview controls.
- Fit mode recalculates automatically when the Preview pane or main window is resized.

## 0.1.21

- Added sortable result headers.
- Repeated header clicks toggle ascending/descending order.
- Selected file and Preview are preserved after sorting.
- Date metadata updates are ID-based to avoid wrong-row updates after sorting.

## 0.1.20

- Added background Date metadata and automatic result-column width calculation.
- Added drag-to-copy for Name and Path cells.

## 0.1.19

- Added keyboard pane switching between File List and Preview with Right/Left Arrow.

## 0.1.18

- Main window opens centered on the primary Windows work area.

## 0.1.17 and earlier

- Added keyboard-first result navigation, Preview support, Enter-to-open, Recycle-Bin deletion for images, Esc cancel, resizable Preview pane, path-aware search syntax, background indexing, and portable index storage.
