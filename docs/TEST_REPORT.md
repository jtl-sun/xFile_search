# Test report

Source snapshot: xFile_search v0.1.25

The source package was checked before the GitHub-ready bundle was prepared.

## Checks

- `go test ./...` — PASS
- `go test -race ./...` — PASS on the Linux test host for cross-platform logic
- `go vet ./...` — PASS
- Windows amd64 cross-build — PASS

## Covered pure-logic areas

Automated tests include search/query behavior, mapped-index loading, result selection, keyboard navigation, pane switching, Preview layout, image zoom/pan geometry, Size/Date formatting, result-column sizing, visible-result sorting, search-history management, and window-position calculations.

## Windows runtime note

COM Preview Handlers, Explorer Shell extensions, installed Office/PDF preview components, antivirus products, and per-machine file associations can only be fully validated on the target Windows system. The program therefore still requires practical Windows smoke testing after release builds are produced.
