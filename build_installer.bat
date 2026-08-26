@echo off
setlocal
set VERSION=0.1.26
call build_windows.bat
if errorlevel 1 exit /b 1
if not exist installer\payload mkdir installer\payload
copy /Y xFile_search.exe installer\payload\xFile_search.exe >nul
copy /Y INSTALL_KO.md installer\payload\README_KO.txt >nul
copy /Y INSTALL.md installer\payload\README.txt >nul
pushd installer
go build -trimpath -ldflags="-H=windowsgui" -o ..\xFile_search_Setup_v%VERSION%_x64.exe .
set RC=%ERRORLEVEL%
popd
if not "%RC%"=="0" exit /b %RC%
echo Built xFile_search_Setup_v%VERSION%_x64.exe
