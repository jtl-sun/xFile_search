//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"
)

// Native Explorer-style context menus for xFile_search results.
// The menu is supplied by the Windows Shell itself, so installed shell
// extensions (Open with, antivirus scanners, editors, Send To, Properties,
// etc.) can participate just like they do in Explorer.

const (
	nmRClick = -5

	wmDrawItem      = 0x002B
	wmMeasureItem   = 0x002C
	wmInitMenuPopup = 0x0117
	wmMenuChar      = 0x0120

	cmfNormal = 0x00000000

	shellMenuFirstID = 1
	shellMenuLastID  = 0x7FFF

	rpcEChangedMode = 0x80010106
)

var (
	procSHParseDisplayName = shell32.NewProc("SHParseDisplayName")
	procSHBindToParent     = shell32.NewProc("SHBindToParent")
	procCoTaskMemFree      = ole32.NewProc("CoTaskMemFree")
)

var (
	iidIShellFolder  = guid{0x000214e6, 0x0000, 0x0000, [8]byte{0xc0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidIContextMenu  = guid{0x000214e4, 0x0000, 0x0000, [8]byte{0xc0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidIContextMenu2 = guid{0x000214f4, 0x0000, 0x0000, [8]byte{0xc0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidIContextMenu3 = guid{0xbcfce0a0, 0xec17, 0x11d0, [8]byte{0x8d, 0x10, 0x00, 0xa0, 0xc9, 0x0f, 0x27, 0x19}}
)

type nmItemActivate struct {
	Hdr       nmhdr
	IItem     int32
	ISubItem  int32
	UNewState uint32
	UOldState uint32
	UChanged  uint32
	PtAction  point
	LParam    uintptr
	UKeyFlags uint32
}

// CMINVOKECOMMANDINFO. lpVerb is an ordinal (MAKEINTRESOURCEA) rather than a
// string, so it is safe for Unicode file paths because the Shell already owns
// the selected PIDL and command binding.
type cmInvokeCommandInfo struct {
	CbSize       uint32
	FMask        uint32
	Hwnd         uintptr
	LpVerb       uintptr
	LpParameters uintptr
	LpDirectory  uintptr
	NShow        int32
	DwHotKey     uint32
	HIcon        uintptr
}

func (a *WindowsApp) selectListRow(row int) {
	if a == nil || a.list == 0 || row < 0 || row >= len(a.shownIDs) {
		return
	}
	clear := lvItem{State: 0, StateMask: lvisSelected | lvisFocused}
	sendMessage(a.list, lvmSetItemState, ^uintptr(0), uintptr(unsafe.Pointer(&clear)))
	state := lvItem{State: lvisSelected | lvisFocused, StateMask: lvisSelected | lvisFocused}
	sendMessage(a.list, lvmSetItemState, uintptr(row), uintptr(unsafe.Pointer(&state)))
	sendMessage(a.list, lvmEnsureVisible, uintptr(row), 0)
	procSetFocus.Call(a.list)
	a.schedulePreview()
}

func (a *WindowsApp) showSelectedShellContextMenu(screenPt point) {
	if a == nil || a.hwnd == 0 {
		return
	}
	e, ok := a.selectedEntry()
	if !ok || e.Path == "" {
		return
	}
	if err := a.showShellContextMenu(e.Path, screenPt); err != nil {
		a.showFallbackContextMenu(e.Path, screenPt)
		a.setStatus("Windows shell menu fallback: " + err.Error())
	}
}

func (a *WindowsApp) showShellContextMenu(path string, screenPt point) error {
	// The UI thread is STA. CoInitializeEx may return S_FALSE if already
	// initialized; that is still success and must be balanced by CoUninitialize.
	hr, _, _ := procCoInitializeEx.Call(0, coinitApartmentThreaded)
	didInit := !hresultFailed(hr)
	if hresultFailed(hr) && uint32(hr) != rpcEChangedMode {
		return fmt.Errorf("CoInitializeEx failed: 0x%08X", uint32(hr))
	}
	if didInit {
		defer procCoUninitialize.Call()
	}

	p := utf16Ptr(filepath.Clean(path))
	var absPidl uintptr
	hr, _, _ = procSHParseDisplayName.Call(
		uintptr(unsafe.Pointer(p)), 0, uintptr(unsafe.Pointer(&absPidl)), 0, 0,
	)
	if hresultFailed(hr) || absPidl == 0 {
		return fmt.Errorf("SHParseDisplayName failed: 0x%08X", uint32(hr))
	}
	defer procCoTaskMemFree.Call(absPidl)

	var parent uintptr
	var child uintptr
	hr, _, _ = procSHBindToParent.Call(
		absPidl,
		uintptr(unsafe.Pointer(&iidIShellFolder)),
		uintptr(unsafe.Pointer(&parent)),
		uintptr(unsafe.Pointer(&child)),
	)
	if hresultFailed(hr) || parent == 0 || child == 0 {
		return fmt.Errorf("SHBindToParent failed: 0x%08X", uint32(hr))
	}
	defer comRelease(parent)

	childArray := [1]uintptr{child}
	var ctxMenu uintptr
	hr = comCall(parent, 10, // IShellFolder::GetUIObjectOf
		a.hwnd,
		1,
		uintptr(unsafe.Pointer(&childArray[0])),
		uintptr(unsafe.Pointer(&iidIContextMenu)),
		0,
		uintptr(unsafe.Pointer(&ctxMenu)),
	)
	if hresultFailed(hr) || ctxMenu == 0 {
		return fmt.Errorf("GetUIObjectOf(IContextMenu) failed: 0x%08X", uint32(hr))
	}
	defer comRelease(ctxMenu)

	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return fmt.Errorf("CreatePopupMenu failed")
	}
	defer procDestroyMenu.Call(menu)

	hr = comCall(ctxMenu, 3, // IContextMenu::QueryContextMenu
		menu, 0, shellMenuFirstID, shellMenuLastID, cmfNormal,
	)
	if hresultFailed(hr) {
		return fmt.Errorf("QueryContextMenu failed: 0x%08X", uint32(hr))
	}

	// IContextMenu2/3 receive owner-draw and submenu messages while the popup
	// is live. Querying these interfaces dramatically improves compatibility
	// with third-party Explorer extensions.
	if m3, ok := comQueryInterface(ctxMenu, &iidIContextMenu3); ok {
		a.shellMenu3 = m3
		defer func() {
			a.shellMenu3 = 0
			comRelease(m3)
		}()
	}
	if a.shellMenu3 == 0 {
		if m2, ok := comQueryInterface(ctxMenu, &iidIContextMenu2); ok {
			a.shellMenu2 = m2
			defer func() {
				a.shellMenu2 = 0
				comRelease(m2)
			}()
		}
	}

	cmd, _, _ := procTrackPopupMenuEx.Call(
		menu,
		tpmLeftAlign|tpmTopAlign|tpmReturnCmd|tpmRightButton,
		uintptr(screenPt.X), uintptr(screenPt.Y), a.hwnd, 0,
	)
	if cmd == 0 {
		procSetFocus.Call(a.list)
		return nil
	}
	if cmd < shellMenuFirstID || cmd > shellMenuLastID {
		procSetFocus.Call(a.list)
		return nil
	}

	info := cmInvokeCommandInfo{
		CbSize: uint32(unsafe.Sizeof(cmInvokeCommandInfo{})),
		Hwnd:   a.hwnd,
		LpVerb: cmd - shellMenuFirstID,
		NShow:  swShow,
	}
	hr = comCall(ctxMenu, 4, uintptr(unsafe.Pointer(&info))) // InvokeCommand
	procSetFocus.Call(a.list)
	if hresultFailed(hr) {
		return fmt.Errorf("InvokeCommand failed: 0x%08X", uint32(hr))
	}

	// Commands such as Delete can remove the item outside xFile_search. Check
	// after the handler returns, and once more shortly afterward for async shell
	// extensions, then hide a vanished path from the current indexed results.
	a.refreshPathAfterShellAction(path)
	return nil
}

func (a *WindowsApp) forwardShellContextMenuMessage(message uint32, wParam, lParam uintptr) (uintptr, bool) {
	if a == nil {
		return 0, false
	}
	if a.shellMenu3 != 0 {
		var result uintptr
		hr := comCall(a.shellMenu3, 7, uintptr(message), wParam, lParam, uintptr(unsafe.Pointer(&result)))
		if !hresultFailed(hr) {
			return result, true
		}
	}
	if a.shellMenu2 != 0 {
		hr := comCall(a.shellMenu2, 6, uintptr(message), wParam, lParam)
		if !hresultFailed(hr) {
			return 0, true
		}
	}
	return 0, false
}

func (a *WindowsApp) refreshPathAfterShellAction(path string) {
	if a == nil || path == "" {
		return
	}
	check := func() {
		if _, err := os.Stat(path); err == nil {
			return
		}
		// Ignore a stale delayed check if another context menu has since
		// become the active one. UI state is touched only by the UI thread.
		if current, _ := a.shellGonePath.Load().(string); current != path {
			return
		}
		postMessage(a.hwnd, wmAppShellPathGone, 0, 0)
	}
	// Keep the path in a tiny one-shot slot. Context-menu commands are invoked
	// serially on the UI thread, so this cannot overlap with another popup.
	a.shellGonePath.Store(path)
	check()
	go func() {
		time.Sleep(700 * time.Millisecond)
		check()
	}()
}

func (a *WindowsApp) consumeShellGonePath() {
	if a == nil {
		return
	}
	v := a.shellGonePath.Load()
	path, _ := v.(string)
	if path == "" {
		return
	}
	if _, err := os.Stat(path); err == nil {
		return
	}
	a.shellGonePath.Store("")
	row := a.rowForPath(path)
	a.markSessionDeleted(path)
	a.refreshAfterDelete(row)
	a.setStatus("Shell action updated results: " + path)
}

func (a *WindowsApp) rowForPath(path string) int {
	if a == nil || path == "" {
		return -1
	}
	snap := a.lastSnap
	if len(a.steps) > 0 {
		snap = a.sessionSnap
	}
	if snap == nil {
		snap = a.snapshot.Load()
	}
	if snap == nil {
		return -1
	}
	clean := filepath.Clean(path)
	for row, id := range a.shownIDs {
		if e, ok := snap.EntryAt(id); ok && strings.EqualFold(filepath.Clean(e.Path), clean) {
			return row
		}
	}
	return -1
}

func (a *WindowsApp) showFallbackContextMenu(path string, pt point) {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)
	add := func(id uintptr, label string) {
		p := utf16Ptr(label)
		procAppendMenuW.Call(menu, mfString, id, uintptr(unsafe.Pointer(p)))
	}
	add(1, "Open")
	add(2, "Open path")
	add(3, "Copy full path")
	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		add(4, "Open with...")
	}
	cmd, _, _ := procTrackPopupMenuEx.Call(menu, tpmLeftAlign|tpmTopAlign|tpmReturnCmd|tpmRightButton, uintptr(pt.X), uintptr(pt.Y), a.hwnd, 0)
	switch cmd {
	case 1:
		a.openPathWithAssociatedApp(path)
	case 2:
		target := path
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			target = filepath.Dir(path)
		}
		shellOpen(a.hwnd, target)
	case 3:
		if copyTextToClipboard(a.hwnd, path) {
			a.setStatus("Path copied: " + path)
		}
	case 4:
		shellOpenWith(a.hwnd, path)
	}
	procSetFocus.Call(a.list)
}
