# Installing xFile_search

## Recommended: Windows Setup

1. Download `xFile_search_Setup_v0.1.29_x64.exe` from GitHub **Releases**.
2. Double-click the setup file.
3. Click **Yes** in the install confirmation.
4. xFile_search launches automatically when installation finishes.

Default install location:

`%LOCALAPPDATA%\Programs\xFile_search`

No administrator privileges are required. Installing over an older version updates the program files while preserving `Index`, `Logs`, `Backup`, and `SearchHistory.txt`.

## Portable

Extract `xFile_search_Portable_v0.1.29_x64.zip` and run `xFile_search.exe`.

## Uninstall

Use **Settings > Apps > Installed apps > xFile_search > Uninstall**.


## First v0.1.29 launch

When upgrading from v0.1.27 or earlier, xFile_search may perform **one automatic background reindex** to establish volume fingerprints for fixed/removable drives.

The app stays usable while indexing. Once the first partial checkpoint is ready, searches use that partial index while the full index continues in the background. Indexing activity is visible with a bold green `INDEXING... xx%` label, in the title bar, on the Reindex button, in the bottom marquee indicator, and in the status text. The percentage is a lightweight estimate based on the previous complete index item count and is capped at 99% until completion, avoiding a slow double-scan just to calculate an exact percentage.
