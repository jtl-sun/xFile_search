//go:build windows

package main

import (
	"os"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

// v0.1.31 productivity features are intentionally isolated from the search and
// indexing core. They run on their own locked UI thread so the existing main
// Win32 message loop, memory-mapped index, and background indexer stay unchanged.

const (
	productivityClassName = "xFileSearchProductivityWindowClass"
	productivityHotkeyID  = 0x5831
	productivityTrayID    = 1
	productivityTimerID   = 1

	wmHotKey      = 0x0312
	wmTimer       = 0x0113
	wmContextMenu = 0x007B
	wmRButtonUp   = 0x0205
	wmProductTray = wmApp + 40

	modAlt      = 0x0001
	modControl  = 0x0002
	modNoRepeat = 0x4000
	vkF         = 0x46

	nimAdd    = 0x00000000
	nimDelete = 0x00000002
	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	idiApplication = 32512
	swRestore      = 9

	trayCmdShow      = 41001
	trayCmdHide      = 41002
	trayCmdClear     = 41003
	trayCmdReindex   = 41004
	trayCmdIndexDir  = 41005
	trayCmdStartup   = 41006
	trayCmdExit      = 41007

	mfChecked = 0x00000008

	emSetSel = 0x00B1

	hkeyCurrentUser = 0x80000001
	keyQueryValue   = 0x0001
	keySetValue     = 0x0002
	regSz           = 1
)

var (
	procRegisterHotKey          = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey        = user32.NewProc("UnregisterHotKey")
	procSetForegroundWindow     = user32.NewProc("SetForegroundWindow")
	procAttachThreadInput       = user32.NewProc("AttachThreadInput")
	procGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	procFindWindowW             = user32.NewProc("FindWindowW")
	procGetDlgItem              = user32.NewProc("GetDlgItem")
	procLoadIconW               = user32.NewProc("LoadIconW")
	procSetTimer                = user32.NewProc("SetTimer")
	procKillTimer               = user32.NewProc("KillTimer")
	procGetCurrentThreadID      = kernel32.NewProc("GetCurrentThreadId")
	procShellNotifyIconW        = shell32.NewProc("Shell_NotifyIconW")

	advapi32                 = syscall.NewLazyDLL("advapi32.dll")
	procRegCreateKeyExW      = advapi32.NewProc("RegCreateKeyExW")
	procRegOpenKeyExW        = advapi32.NewProc("RegOpenKeyExW")
	procRegSetValueExW       = advapi32.NewProc("RegSetValueExW")
	procRegQueryValueExW     = advapi32.NewProc("RegQueryValueExW")
	procRegDeleteValueW      = advapi32.NewProc("RegDeleteValueW")
	procRegCloseKey          = advapi32.NewProc("RegCloseKey")
)

type notifyIconData struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UTimeoutVersion  uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     uintptr
}

var productivitySawMain bool

func startProductivityFeatures() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hinst, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	className := utf16Ptr(productivityClassName)
	wc := wndClassEx{
		CbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		LpfnWndProc:   syscall.NewCallback(productivityWndProc),
		HInstance:     hinst,
		LpszClassName: className,
	}
	if r, _, _ := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		logf("productivity window class registration failed")
		return
	}

	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(utf16Ptr(appName+" Productivity"))),
		0,
		0, 0, 0, 0,
		0, 0, hinst, 0,
	)
	if hwnd == 0 {
		logf("productivity hidden window creation failed")
		return
	}

	hotkeyOK, _, _ := procRegisterHotKey.Call(hwnd, productivityHotkeyID, modControl|modAlt|modNoRepeat, vkF)
	addProductivityTrayIcon(hwnd, hotkeyOK != 0)
	procSetTimer.Call(hwnd, productivityTimerID, 300, 0)

	defer func() {
		procKillTimer.Call(hwnd, productivityTimerID)
		if hotkeyOK != 0 {
			procUnregisterHotKey.Call(hwnd, productivityHotkeyID)
		}
		removeProductivityTrayIcon(hwnd)
		procDestroyWindow.Call(hwnd)
	}()

	var m msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func productivityWndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case wmHotKey:
		if wParam == productivityHotkeyID {
			showAndFocusMainWindow()
			return 0
		}
	case wmProductTray:
		switch uint32(lParam) {
		case wmLButtonUp, wmLButtonDblClk:
			showAndFocusMainWindow()
			return 0
		case wmRButtonUp, wmContextMenu:
			showProductivityTrayMenu(hwnd)
			return 0
		}
	case wmTimer:
		if wParam == productivityTimerID {
			main := findMainWindow()
			if main != 0 {
				productivitySawMain = true
				refreshDirectoryScopeIndicator(main)
			} else if productivitySawMain {
				procDestroyWindow.Call(hwnd)
			}
			return 0
		}
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
	return r
}

func findMainWindow() uintptr {
	className := utf16Ptr("xFileSearchWindowClass")
	hwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(className)), 0)
	return hwnd
}

func showAndFocusMainWindow() {
	main := findMainWindow()
	if main == 0 {
		return
	}
	procShowWindow.Call(main, swRestore)
	procSetForegroundWindow.Call(main)

	search, _, _ := procGetDlgItem.Call(main, idSearch)
	if search == 0 {
		return
	}
	mainThread, _, _ := procGetWindowThreadProcessID.Call(main, 0)
	thisThread, _, _ := procGetCurrentThreadID.Call()
	attached := false
	if mainThread != 0 && thisThread != 0 && mainThread != thisThread {
		if ok, _, _ := procAttachThreadInput.Call(thisThread, mainThread, 1); ok != 0 {
			attached = true
		}
	}
	procSetForegroundWindow.Call(main)
	procSetFocus.Call(search)
	sendMessage(search, emSetSel, 0, ^uintptr(0))
	if attached {
		procAttachThreadInput.Call(thisThread, mainThread, 0)
	}
}

func hideMainWindow() {
	if main := findMainWindow(); main != 0 {
		procShowWindow.Call(main, swHide)
	}
}

func postMainCommand(id int) {
	if main := findMainWindow(); main != 0 {
		postMessage(main, wmCommand, uintptr(id), 0)
	}
}

func exitMainWindow() {
	if main := findMainWindow(); main != 0 {
		postMessage(main, wmClose, 0, 0)
	}
}

func showProductivityTrayMenu(hwnd uintptr) {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)

	appendTrayMenuItem(menu, mfString, trayCmdShow, "Show xFile_search    Ctrl+Alt+F")
	appendTrayMenuItem(menu, mfString, trayCmdHide, "Hide to Tray")
	appendTrayMenuItem(menu, mfSeparator, 0, "")
	appendTrayMenuItem(menu, mfString, trayCmdClear, "Clear Search / Scope")
	appendTrayMenuItem(menu, mfString, trayCmdReindex, "Reindex")
	appendTrayMenuItem(menu, mfString, trayCmdIndexDir, "Open Index Folder")
	appendTrayMenuItem(menu, mfSeparator, 0, "")
	startupFlags := uint32(mfString)
	if startupEnabled() {
		startupFlags |= mfChecked
	}
	appendTrayMenuItem(menu, startupFlags, trayCmdStartup, "Start with Windows")
	appendTrayMenuItem(menu, mfSeparator, 0, "")
	appendTrayMenuItem(menu, mfString, trayCmdExit, "Exit")

	var pt point
	if ok, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt))); ok == 0 {
		return
	}
	procSetForegroundWindow.Call(hwnd)
	cmd, _, _ := procTrackPopupMenuEx.Call(menu, tpmLeftAlign|tpmTopAlign|tpmReturnCmd|tpmRightButton, uintptr(pt.X), uintptr(pt.Y), hwnd, 0)
	switch int(cmd) {
	case trayCmdShow:
		showAndFocusMainWindow()
	case trayCmdHide:
		hideMainWindow()
	case trayCmdClear:
		postMainCommand(idClear)
		showAndFocusMainWindow()
	case trayCmdReindex:
		postMainCommand(idReindex)
	case trayCmdIndexDir:
		postMainCommand(idIndexDir)
	case trayCmdStartup:
		_ = setStartupEnabled(!startupEnabled())
	case trayCmdExit:
		exitMainWindow()
	}
}

func appendTrayMenuItem(menu uintptr, flags uint32, id int, text string) {
	if flags&mfSeparator != 0 {
		procAppendMenuW.Call(menu, uintptr(flags), 0, 0)
		return
	}
	p := utf16Ptr(text)
	procAppendMenuW.Call(menu, uintptr(flags), uintptr(id), uintptr(unsafe.Pointer(p)))
}

func addProductivityTrayIcon(hwnd uintptr, hotkeyOK bool) {
	nid := notifyIconData{CbSize: uint32(unsafe.Sizeof(notifyIconData{})), HWnd: hwnd, UID: productivityTrayID}
	nid.UFlags = nifMessage | nifIcon | nifTip
	nid.UCallbackMessage = wmProductTray
	icon, _, _ := procLoadIconW.Call(0, idiApplication)
	nid.HIcon = icon
	tip := appName + " " + appVersion + " · Ctrl+Alt+F"
	if !hotkeyOK {
		tip = appName + " " + appVersion + " · hotkey unavailable"
	}
	copyUTF16Fixed(nid.SzTip[:], tip)
	procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
}

func removeProductivityTrayIcon(hwnd uintptr) {
	nid := notifyIconData{CbSize: uint32(unsafe.Sizeof(notifyIconData{})), HWnd: hwnd, UID: productivityTrayID}
	procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
}

func copyUTF16Fixed(dst []uint16, s string) {
	u, _ := syscall.UTF16FromString(s)
	if len(u) > len(dst) {
		u = u[:len(dst)]
		if len(u) > 0 {
			u[len(u)-1] = 0
		}
	}
	copy(dst, u)
}

func refreshDirectoryScopeIndicator(main uintptr) {
	search, _, _ := procGetDlgItem.Call(main, idSearch)
	bread, _, _ := procGetDlgItem.Call(main, idBread)
	if search == 0 || bread == 0 {
		return
	}
	raw := strings.TrimSpace(getWindowText(search))
	current := getWindowText(bread)
	if label, ok := directoryScopeLabel(raw); ok {
		if current != label {
			setWindowText(bread, label)
		}
		return
	}
	if strings.HasPrefix(current, "Scope: ") {
		setWindowText(bread, "Search → Search Within → Search Within  (results get narrower and faster)")
	}
}

func directoryScopeLabel(raw string) (string, bool) {
	if _, ok := wholeDirectoryPrefix(strings.TrimSpace(raw)); !ok {
		return "", false
	}
	display := strings.TrimSpace(strings.Trim(strings.TrimSpace(raw), `"`))
	display = normalizePathToken(display)
	return "Scope: " + display + "   |   Subfolders: ON   |   Clear: Clear button", true
}

const startupRegistryPath = `Software\Microsoft\Windows\CurrentVersion\Run`
const startupValueName = "xFile_search"

func startupEnabled() bool {
	var key uintptr
	subkey := utf16Ptr(startupRegistryPath)
	r, _, _ := procRegOpenKeyExW.Call(hkeyCurrentUser, uintptr(unsafe.Pointer(subkey)), 0, keyQueryValue, uintptr(unsafe.Pointer(&key)))
	if r != 0 {
		return false
	}
	defer procRegCloseKey.Call(key)
	name := utf16Ptr(startupValueName)
	var typ uint32
	var size uint32
	r, _, _ = procRegQueryValueExW.Call(key, uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&typ)), 0, uintptr(unsafe.Pointer(&size)))
	return r == 0 && typ == regSz && size > 2
}

func setStartupEnabled(enable bool) bool {
	name := utf16Ptr(startupValueName)
	if !enable {
		var key uintptr
		subkey := utf16Ptr(startupRegistryPath)
		r, _, _ := procRegOpenKeyExW.Call(hkeyCurrentUser, uintptr(unsafe.Pointer(subkey)), 0, keySetValue, uintptr(unsafe.Pointer(&key)))
		if r != 0 {
			return r == 2 // already absent
		}
		defer procRegCloseKey.Call(key)
		r, _, _ = procRegDeleteValueW.Call(key, uintptr(unsafe.Pointer(name)))
		return r == 0 || r == 2
	}

	exe, err := os.Executable()
	if err != nil || exe == "" {
		return false
	}
	command := `"` + exe + `"`
	data, err := syscall.UTF16FromString(command)
	if err != nil {
		return false
	}
	var key uintptr
	var disposition uint32
	subkey := utf16Ptr(startupRegistryPath)
	r, _, _ := procRegCreateKeyExW.Call(
		hkeyCurrentUser,
		uintptr(unsafe.Pointer(subkey)),
		0, 0, 0,
		keyQueryValue|keySetValue,
		0,
		uintptr(unsafe.Pointer(&key)),
		uintptr(unsafe.Pointer(&disposition)),
	)
	if r != 0 {
		return false
	}
	defer procRegCloseKey.Call(key)
	r, _, _ = procRegSetValueExW.Call(
		key,
		uintptr(unsafe.Pointer(name)),
		0,
		regSz,
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(len(data)*2),
	)
	return r == 0
}
