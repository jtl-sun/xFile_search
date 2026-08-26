@echo off
setlocal
go test ./...
if errorlevel 1 exit /b 1
go build -trimpath -ldflags="-H=windowsgui" -o xFile_search.exe .
if errorlevel 1 exit /b 1
copy /Y xFile_search.exe xFile_indexer.exe >nul
echo Built xFile_search.exe + xFile_indexer.exe
