//go:build windows

package main

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	appName    = "xFile_search"
	appVersion = "0.1.27"
)

//go:embed payload/xFile_search.exe payload/README_KO.txt payload/README.txt
var payload embed.FS

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")
	advapi32 = syscall.NewLazyDLL("advapi32.dll")

	procMessageBoxW      = user32.NewProc("MessageBoxW")
	procCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	procCoUninitialize   = ole32.NewProc("CoUninitialize")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
	procRegCreateKeyExW  = advapi32.NewProc("RegCreateKeyExW")
	procRegSetValueExW   = advapi32.NewProc("RegSetValueExW")
	procRegCloseKey      = advapi32.NewProc("RegCloseKey")
	procRegDeleteTreeW   = advapi32.NewProc("RegDeleteTreeW")
)

const (
	mbOK              = 0x00000000
	mbYesNo           = 0x00000004
	mbIconInformation = 0x00000040
	mbIconQuestion    = 0x00000020
	mbIconError       = 0x00000010
	idYes             = 6

	clsctxInprocServer = 0x1
	coinitApartment    = 0x2

	hkeyCurrentUser = uintptr(0x80000001)
	keySetValue     = 0x0002
	regSZ           = 1
	regDWORD        = 4
)

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	clsidShellLink = guid{0x00021401, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidShellLinkW  = guid{0x000214F9, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidPersistFile = guid{0x0000010B, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
)

type iUnknownVtbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
}

type iUnknown struct{ Vtbl *iUnknownVtbl }

type iShellLinkWVtbl struct {
	QueryInterface, AddRef, Release          uintptr
	GetPath, GetIDList, SetIDList            uintptr
	GetDescription, SetDescription           uintptr
	GetWorkingDirectory, SetWorkingDirectory uintptr
	GetArguments, SetArguments               uintptr
	GetHotkey, SetHotkey                     uintptr
	GetShowCmd, SetShowCmd                   uintptr
	GetIconLocation, SetIconLocation         uintptr
	SetRelativePath, Resolve, SetPath        uintptr
}

type iShellLinkW struct{ Vtbl *iShellLinkWVtbl }

type iPersistFileVtbl struct {
	QueryInterface, AddRef, Release                            uintptr
	GetClassID, IsDirty, Load, Save, SaveCompleted, GetCurFile uintptr
}

type iPersistFile struct{ Vtbl *iPersistFileVtbl }

func main() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if len(os.Args) > 1 && os.Args[1] == "--uninstall" {
		uninstall(false)
		return
	}
	if len(os.Args) > 2 && os.Args[1] == "--uninstall-final" {
		time.Sleep(600 * time.Millisecond)
		uninstallFinal(os.Args[2])
		return
	}
	if err := install(); err != nil {
		message("xFile_search Setup", "Installation failed:\n\n"+err.Error(), mbOK|mbIconError)
	}
}

func install() error {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return errors.New("LOCALAPPDATA is unavailable")
	}
	dir := filepath.Join(local, "Programs", appName)
	exe := filepath.Join(dir, "xFile_search.exe")

	text := fmt.Sprintf("Install %s %s?\n\nLocation:\n%s\n\nExisting Index and search history will be preserved.", appName, appVersion, dir)
	if message("xFile_search Setup", text, mbYesNo|mbIconQuestion) != idYes {
		return nil
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	for _, d := range []string{"Index", "Logs", "Backup"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0755); err != nil {
			return err
		}
	}

	appBytes, err := payload.ReadFile("payload/xFile_search.exe")
	if err != nil {
		return err
	}
	if err := writeAtomic(exe, appBytes, 0755); err != nil {
		return fmt.Errorf("could not update xFile_search.exe. Close xFile_search and try again: %w", err)
	}
	if err := writeAtomic(filepath.Join(dir, "xFile_indexer.exe"), appBytes, 0755); err != nil {
		return err
	}

	if b, e := payload.ReadFile("payload/README_KO.txt"); e == nil {
		_ = os.WriteFile(filepath.Join(dir, "README_KO.txt"), b, 0644)
	}
	if b, e := payload.ReadFile("payload/README.txt"); e == nil {
		_ = os.WriteFile(filepath.Join(dir, "README.txt"), b, 0644)
	}

	self, err := os.Executable()
	if err == nil {
		if b, e := os.ReadFile(self); e == nil {
			_ = writeAtomic(filepath.Join(dir, "Uninstall_xFile_search.exe"), b, 0755)
		}
	}

	_ = createShortcuts(exe, dir)
	_ = registerUninstall(dir, exe)

	message("xFile_search Setup", "Installation completed.\n\nxFile_search will now start.", mbOK|mbIconInformation)
	_ = exec.Command(exe).Start()
	return nil
}

func uninstall(fromTemp bool) {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return
	}
	dir := filepath.Join(local, "Programs", appName)
	if !fromTemp {
		if message("Uninstall xFile_search", "Remove xFile_search?\n\nIndex, Logs, Backup, and SearchHistory.txt will be kept so they can be reused later.", mbYesNo|mbIconQuestion) != idYes {
			return
		}
		self, err := os.Executable()
		if err == nil {
			tmp := filepath.Join(os.TempDir(), fmt.Sprintf("xFile_search_uninstall_%d.exe", os.Getpid()))
			if b, e := os.ReadFile(self); e == nil && os.WriteFile(tmp, b, 0755) == nil {
				_ = exec.Command(tmp, "--uninstall-final", dir).Start()
				return
			}
		}
	}
	uninstallFinal(dir)
}

func uninstallFinal(dir string) {
	removeShortcuts()
	_ = deleteUninstallReg()
	keep := map[string]bool{"Index": true, "Logs": true, "Backup": true, "SearchHistory.txt": true, "xFile_search.ini": true}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if keep[e.Name()] {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if e.IsDir() {
			_ = os.RemoveAll(p)
		} else {
			_ = os.Remove(p)
		}
	}
	message("Uninstall xFile_search", "xFile_search was removed.\n\nYour Index and search history were preserved in:\n"+dir, mbOK|mbIconInformation)
}

func writeAtomic(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".new"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	_ = os.Remove(path + ".old")
	if _, err := os.Stat(path); err == nil {
		if err := os.Rename(path, path+".old"); err != nil {
			_ = os.Remove(tmp)
			return err
		}
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	_ = os.Remove(path + ".old")
	return nil
}

func message(title, text string, flags uintptr) int {
	tp, _ := syscall.UTF16PtrFromString(title)
	xp, _ := syscall.UTF16PtrFromString(text)
	r, _, _ := procMessageBoxW.Call(0, uintptr(unsafe.Pointer(xp)), uintptr(unsafe.Pointer(tp)), flags)
	return int(r)
}

func createShortcuts(exe, workDir string) error {
	desktop := filepath.Join(os.Getenv("USERPROFILE"), "Desktop", appName+".lnk")
	appdata := os.Getenv("APPDATA")
	startDir := filepath.Join(appdata, "Microsoft", "Windows", "Start Menu", "Programs", appName)
	_ = os.MkdirAll(startDir, 0755)
	start := filepath.Join(startDir, appName+".lnk")
	if err := createShortcut(desktop, exe, workDir); err != nil {
		return err
	}
	return createShortcut(start, exe, workDir)
}

func removeShortcuts() {
	_ = os.Remove(filepath.Join(os.Getenv("USERPROFILE"), "Desktop", appName+".lnk"))
	startDir := filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", appName)
	_ = os.RemoveAll(startDir)
}

func createShortcut(shortcut, target, workDir string) error {
	r, _, _ := procCoInitializeEx.Call(0, coinitApartment)
	if r != 0 && r != 1 {
		return fmt.Errorf("CoInitializeEx: 0x%x", r)
	}
	defer procCoUninitialize.Call()

	var sl *iShellLinkW
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidShellLink)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidShellLinkW)), uintptr(unsafe.Pointer(&sl)),
	)
	if hr != 0 || sl == nil {
		return fmt.Errorf("CoCreateInstance: 0x%x", hr)
	}
	defer syscall.SyscallN(sl.Vtbl.Release, uintptr(unsafe.Pointer(sl)))

	t, _ := syscall.UTF16PtrFromString(target)
	w, _ := syscall.UTF16PtrFromString(workDir)
	d, _ := syscall.UTF16PtrFromString("Fast Windows file search")
	if hr, _, _ = syscall.SyscallN(sl.Vtbl.SetPath, uintptr(unsafe.Pointer(sl)), uintptr(unsafe.Pointer(t))); hr != 0 {
		return fmt.Errorf("SetPath: 0x%x", hr)
	}
	_, _, _ = syscall.SyscallN(sl.Vtbl.SetWorkingDirectory, uintptr(unsafe.Pointer(sl)), uintptr(unsafe.Pointer(w)))
	_, _, _ = syscall.SyscallN(sl.Vtbl.SetDescription, uintptr(unsafe.Pointer(sl)), uintptr(unsafe.Pointer(d)))
	_, _, _ = syscall.SyscallN(sl.Vtbl.SetIconLocation, uintptr(unsafe.Pointer(sl)), uintptr(unsafe.Pointer(t)), 0)
	_, _, _ = syscall.SyscallN(sl.Vtbl.SetShowCmd, uintptr(unsafe.Pointer(sl)), 1)

	var pf *iPersistFile
	hr, _, _ = syscall.SyscallN(sl.Vtbl.QueryInterface, uintptr(unsafe.Pointer(sl)), uintptr(unsafe.Pointer(&iidPersistFile)), uintptr(unsafe.Pointer(&pf)))
	if hr != 0 || pf == nil {
		return fmt.Errorf("QueryInterface(IPersistFile): 0x%x", hr)
	}
	defer syscall.SyscallN(pf.Vtbl.Release, uintptr(unsafe.Pointer(pf)))
	s, _ := syscall.UTF16PtrFromString(shortcut)
	hr, _, _ = syscall.SyscallN(pf.Vtbl.Save, uintptr(unsafe.Pointer(pf)), uintptr(unsafe.Pointer(s)), 1)
	if hr != 0 {
		return fmt.Errorf("Save shortcut: 0x%x", hr)
	}
	return nil
}

func registerUninstall(dir, exe string) error {
	key, err := openUninstallKey()
	if err != nil {
		return err
	}
	defer procRegCloseKey.Call(key)
	uninstaller := filepath.Join(dir, "Uninstall_xFile_search.exe")
	values := map[string]string{
		"DisplayName":          appName,
		"DisplayVersion":       appVersion,
		"Publisher":            "jtl-sun",
		"InstallLocation":      dir,
		"DisplayIcon":          exe,
		"UninstallString":      fmt.Sprintf("\"%s\" --uninstall", uninstaller),
		"QuietUninstallString": fmt.Sprintf("\"%s\" --uninstall", uninstaller),
	}
	for n, v := range values {
		if err := regSetString(key, n, v); err != nil {
			return err
		}
	}
	_ = regSetDWORD(key, "NoModify", 1)
	_ = regSetDWORD(key, "NoRepair", 1)
	return nil
}

func openUninstallKey() (uintptr, error) {
	sub := `Software\Microsoft\Windows\CurrentVersion\Uninstall\xFile_search`
	sp, _ := syscall.UTF16PtrFromString(sub)
	var key uintptr
	r, _, _ := procRegCreateKeyExW.Call(hkeyCurrentUser, uintptr(unsafe.Pointer(sp)), 0, 0, 0, keySetValue, 0, uintptr(unsafe.Pointer(&key)), 0)
	if r != 0 {
		return 0, fmt.Errorf("RegCreateKeyExW: %d", r)
	}
	return key, nil
}

func regSetString(key uintptr, name, value string) error {
	np, _ := syscall.UTF16PtrFromString(name)
	vp, _ := syscall.UTF16FromString(value)
	r, _, _ := procRegSetValueExW.Call(key, uintptr(unsafe.Pointer(np)), 0, regSZ, uintptr(unsafe.Pointer(&vp[0])), uintptr(len(vp)*2))
	if r != 0 {
		return fmt.Errorf("RegSetValueExW(%s): %d", name, r)
	}
	return nil
}

func regSetDWORD(key uintptr, name string, value uint32) error {
	np, _ := syscall.UTF16PtrFromString(name)
	r, _, _ := procRegSetValueExW.Call(key, uintptr(unsafe.Pointer(np)), 0, regDWORD, uintptr(unsafe.Pointer(&value)), 4)
	if r != 0 {
		return fmt.Errorf("RegSetValueExW(%s): %d", name, r)
	}
	return nil
}

func deleteUninstallReg() error {
	sub := `Software\Microsoft\Windows\CurrentVersion\Uninstall\xFile_search`
	sp, _ := syscall.UTF16PtrFromString(sub)
	r, _, _ := procRegDeleteTreeW.Call(hkeyCurrentUser, uintptr(unsafe.Pointer(sp)))
	if r != 0 && r != 2 {
		return fmt.Errorf("RegDeleteTreeW: %d", r)
	}
	return nil
}

var _ = io.Copy
var _ = strings.TrimSpace
