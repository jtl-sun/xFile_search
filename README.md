# xFile_search

A fast, lightweight Windows file-name search application focused on large file collections, responsive indexing, keyboard-first navigation, and rich previews.

Current version: **0.1.26**

> Korean documentation: [README_KO.md](README_KO.md)

## Highlights

- Fast indexed file-name/path search for large local file collections
- Search syntax such as `*.jpg`, `D:\\*.jpg`, and folder-scoped patterns
- Search-within-results workflow
- Background metadata loading so the result list remains responsive
- Result columns: **Name | Path | Size | Date** with header sorting
- Preview pane for images and Windows-supported documents
- Image preview: mouse-wheel zoom at cursor, drag-to-pan, **1:1**, and **Fit Window**
- Keyboard workflow: arrows to navigate, `Enter` to open, `Del` to move supported images to Recycle Bin, `Esc` to cancel
- Recent-search history stored locally
- Explorer-style Windows Shell context menu on right-click
- Portable index layout with visible `Index` folder


## Install (recommended)

For most Windows users, download **`xFile_search_Setup_v0.1.26_x64.exe`** from GitHub Releases and run it. It installs per-user without administrator privileges and preserves Index/search history during upgrades.

Portable users can download **`xFile_search_Portable_v0.1.26_x64.zip`**. See [INSTALL.md](INSTALL.md) for details.

## Requirements

- Windows 10/11 x64
- Go 1.23.2 or newer to build from source
- Office/PDF preview availability depends on Preview Handlers installed on Windows

## Build

From a Windows command prompt with Go installed:

```bat
build_windows.bat
```

The script runs tests and builds `xFile_search.exe` and `xFile_indexer.exe`.

Or build manually:

```bat
go test ./...
go build -trimpath -ldflags="-H=windowsgui" -o xFile_search.exe .
copy /Y xFile_search.exe xFile_indexer.exe
```

## Portable layout

At runtime xFile_search can keep its searchable index beside the executable:

```text
xFile_search/
├─ xFile_search.exe
├─ xFile_indexer.exe
├─ xFile_search.ini
├─ SearchHistory.txt
├─ Index/
├─ Logs/
└─ Backup/
```

Runtime data and generated indexes are intentionally excluded from Git by `.gitignore`.

## Security notes

xFile_search does not require PowerShell, `ExecutionPolicy Bypass`, hidden browser launching, or network access for its core search/preview workflow. See [docs/SECURITY_AUDIT.md](docs/SECURITY_AUDIT.md).

## Tests

The project includes unit tests for search, navigation, selection, sorting, preview geometry, metadata formatting, search history, and window/layout behavior. See [docs/TEST_REPORT.md](docs/TEST_REPORT.md).

## Changelog

See [CHANGELOG.md](CHANGELOG.md).

## License

No open-source license has been selected yet. Until a license is added, normal copyright restrictions apply.
