//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	comctl32 = syscall.NewLazyDLL("comctl32.dll")

	procRegisterClassExW      = user32.NewProc("RegisterClassExW")
	procCreateWindowExW       = user32.NewProc("CreateWindowExW")
	procDefWindowProcW        = user32.NewProc("DefWindowProcW")
	procShowWindow            = user32.NewProc("ShowWindow")
	procUpdateWindow          = user32.NewProc("UpdateWindow")
	procGetMessageW           = user32.NewProc("GetMessageW")
	procPeekMessageW          = user32.NewProc("PeekMessageW")
	procTranslateMessage      = user32.NewProc("TranslateMessage")
	procDispatchMessageW      = user32.NewProc("DispatchMessageW")
	procPostQuitMessage       = user32.NewProc("PostQuitMessage")
	procDestroyWindow         = user32.NewProc("DestroyWindow")
	procSendMessageW          = user32.NewProc("SendMessageW")
	procPostMessageW          = user32.NewProc("PostMessageW")
	procSetWindowPos          = user32.NewProc("SetWindowPos")
	procGetClientRect         = user32.NewProc("GetClientRect")
	procGetWindowRect         = user32.NewProc("GetWindowRect")
	procGetDoubleClickTime    = user32.NewProc("GetDoubleClickTime")
	procGetWindowTextLengthW  = user32.NewProc("GetWindowTextLengthW")
	procGetWindowTextW        = user32.NewProc("GetWindowTextW")
	procSetWindowTextW        = user32.NewProc("SetWindowTextW")
	procSetFocus              = user32.NewProc("SetFocus")
	procLoadCursorW           = user32.NewProc("LoadCursorW")
	procMessageBoxW           = user32.NewProc("MessageBoxW")
	procInvalidateRect        = user32.NewProc("InvalidateRect")
	procOpenClipboard         = user32.NewProc("OpenClipboard")
	procEmptyClipboard        = user32.NewProc("EmptyClipboard")
	procSetClipboardData      = user32.NewProc("SetClipboardData")
	procCloseClipboard        = user32.NewProc("CloseClipboard")
	procSetCapture            = user32.NewProc("SetCapture")
	procReleaseCapture        = user32.NewProc("ReleaseCapture")
	procGetCursorPos          = user32.NewProc("GetCursorPos")
	procScreenToClient        = user32.NewProc("ScreenToClient")
	procGetWindow             = user32.NewProc("GetWindow")
	procIsWindowVisible       = user32.NewProc("IsWindowVisible")
	procIsChild               = user32.NewProc("IsChild")
	procSendMessageTimeoutW   = user32.NewProc("SendMessageTimeoutW")
	procSystemParametersInfoW = user32.NewProc("SystemParametersInfoW")
	procGetSystemMetrics      = user32.NewProc("GetSystemMetrics")
	procCreatePopupMenu       = user32.NewProc("CreatePopupMenu")
	procAppendMenuW           = user32.NewProc("AppendMenuW")
	procTrackPopupMenuEx      = user32.NewProc("TrackPopupMenuEx")
	procDestroyMenu           = user32.NewProc("DestroyMenu")

	procGetStockObject       = gdi32.NewProc("GetStockObject")
	procDeleteObject         = gdi32.NewProc("DeleteObject")
	procGetObjectW           = gdi32.NewProc("GetObjectW")
	procShellExecuteW        = shell32.NewProc("ShellExecuteW")
	procSHFileOperationW     = shell32.NewProc("SHFileOperationW")
	procInitCommonControlsEx = comctl32.NewProc("InitCommonControlsEx")

	procGlobalAlloc  = kernel32.NewProc("GlobalAlloc")
	procGlobalLock   = kernel32.NewProc("GlobalLock")
	procGlobalUnlock = kernel32.NewProc("GlobalUnlock")
)

const (
	cwUseDefault = 0x80000000

	spiGetWorkArea = 0x0030
	smCxScreen     = 0
	smCyScreen     = 1

	wsOverlappedWindow = 0x00CF0000
	wsChild            = 0x40000000
	wsVisible          = 0x10000000
	wsTabStop          = 0x00010000
	wsBorder           = 0x00800000
	wsVScroll          = 0x00200000
	wsHScroll          = 0x00100000
	wsClipChildren     = 0x02000000

	wsExClientEdge = 0x00000200

	esAutoHScroll = 0x0080
	esMultiline   = 0x0004
	esAutoVScroll = 0x0040
	esReadOnly    = 0x0800
	esWantReturn  = 0x1000

	bsPushButton = 0x00000000

	mfString    = 0x00000000
	mfGrayEd    = 0x00000001
	mfSeparator = 0x00000800

	tpmLeftAlign   = 0x0000
	tpmTopAlign    = 0x0000
	tpmRightButton = 0x0002
	tpmReturnCmd   = 0x0100
	bsAutoCheckBox = 0x00000003

	ssBitmap      = 0x0000000E
	ssCenterImage = 0x00000200

	cbsDropDownList = 0x0003

	lvsReport        = 0x0001
	lvsShowSelAlways = 0x0008

	wmCreate           = 0x0001
	wmDestroy          = 0x0002
	wmSize             = 0x0005
	wmClose            = 0x0010
	wmQuit             = 0x0012
	wmSetFont          = 0x0030
	wmCommand          = 0x0111
	wmNotify           = 0x004E
	wmSetRedraw        = 0x000B
	wmSetCursor        = 0x0020
	wmLButtonDown      = 0x0201
	wmLButtonUp        = 0x0202
	wmMouseMove        = 0x0200
	wmMouseWheel       = 0x020A
	wmKeyDown          = 0x0100
	wmLButtonDblClk    = 0x0203
	wmApp              = 0x8000
	wmAppSearch        = wmApp + 1
	wmAppResults       = wmApp + 2
	wmAppStatus        = wmApp + 3
	wmAppSnapshot      = wmApp + 4
	wmAppPreview       = wmApp + 5
	wmAppFocusList     = wmApp + 6
	wmAppDates         = wmApp + 7
	wmAppShellPathGone = wmApp + 8

	pmRemove = 0x0001

	enChange     = 0x0300
	cbnSelChange = 1
	bnClicked    = 0

	cbAddString = 0x0143
	cbGetCurSel = 0x0147
	cbSetCurSel = 0x014E

	lvmFirst                    = 0x1000
	lvmDeleteAllItems           = lvmFirst + 9
	lvmGetNextItem              = lvmFirst + 12
	lvmEnsureVisible            = lvmFirst + 19
	lvmSetItemState             = lvmFirst + 43
	lvmSetExtendedListViewStyle = lvmFirst + 54
	lvmInsertItemW              = lvmFirst + 77
	lvmSetItemW                 = lvmFirst + 76
	lvmInsertColumnW            = lvmFirst + 97
	lvmSetColumnWidth           = lvmFirst + 30
	lvmSubItemHitTest           = lvmFirst + 57

	lvifText  = 0x0001
	lvifState = 0x0008
	lvcfFmt   = 0x0001
	lvcfWidth = 0x0002
	lvcfText  = 0x0004

	lvsExFullRowSelect = 0x00000020
	lvsExDoubleBuffer  = 0x00010000

	lvniSelected = 0x0002
	lvisFocused  = 0x0001
	lvisSelected = 0x0002

	nmDblClk       = -3
	lvnItemChanged = -101
	lvnColumnClick = -108
	lvnKeyDown     = -155

	swShow = 5
	swHide = 0

	colorWindow     = 5
	colorButtonFace = 15
	idcArrow        = 32512
	idcSizeWE       = 32644

	defaultGuiFont = 17

	mbOK           = 0x00000000
	mbYesNo        = 0x00000004
	mbYesNoCancel  = 0x00000003
	mbIconInfo     = 0x00000040
	mbIconQuestion = 0x00000020
	mbIconError    = 0x00000010
	idYes          = 6
	idCancel       = 2

	vkReturn = 0x0D
	vkLeft   = 0x25
	vkUp     = 0x26
	vkRight  = 0x27
	vkDown   = 0x28
	vkDelete = 0x2E

	foDelete          = 0x0003
	fofSilent         = 0x0004
	fofNoConfirmation = 0x0010
	fofAllowUndo      = 0x0040
	fofNoErrorUI      = 0x0400

	cfUnicodeText = 13
	gmemMoveable  = 0x0002

	bmGetCheck   = 0x00F0
	bmSetCheck   = 0x00F1
	bstUnchecked = 0x0000
	bstChecked   = 0x0001

	stmSetImage = 0x0172
	imageBitmap = 0

	iccListViewClasses = 0x00000001
)

type bitmapObject struct {
	Type       int32
	Width      int32
	Height     int32
	WidthBytes int32
	Planes     uint16
	BitsPixel  uint16
	Bits       uintptr
}

type point struct{ X, Y int32 }
type rect struct{ Left, Top, Right, Bottom int32 }

type msg struct {
	Hwnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       point
	LPrivate uint32
}

type wndClassEx struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type initCommonControlsEx struct {
	DwSize uint32
	DwICC  uint32
}

type nmhdr struct {
	HwndFrom uintptr
	IdFrom   uintptr
	Code     int32
}

type nmListView struct {
	Hdr       nmhdr
	IItem     int32
	ISubItem  int32
	UNewState uint32
	UOldState uint32
	UChanged  uint32
	PtAction  point
	LParam    uintptr
}

type nmLVKeyDown struct {
	Hdr   nmhdr
	WVKey uint16
	Flags uint32
}

type lvHitTestInfo struct {
	Pt       point
	Flags    uint32
	IItem    int32
	ISubItem int32
	IGroup   int32
}

type shFileOpStruct struct {
	Hwnd                  uintptr
	WFunc                 uint32
	PFrom                 *uint16
	PTo                   *uint16
	FFlags                uint16
	_                     uint16
	FAnyOperationsAborted int32
	HNameMappings         uintptr
	LpszProgressTitle     *uint16
}

type lvColumn struct {
	Mask       uint32
	Fmt        int32
	Cx         int32
	PszText    *uint16
	CchTextMax int32
	ISubItem   int32
	IImage     int32
	IOrder     int32
	CxMin      int32
	CxDefault  int32
	CxIdeal    int32
}

type lvItem struct {
	Mask       uint32
	IItem      int32
	ISubItem   int32
	State      uint32
	StateMask  uint32
	PszText    *uint16
	CchTextMax int32
	IImage     int32
	LParam     uintptr
	IIndent    int32
	IGroupID   int32
	CColumns   uint32
	PuColumns  *uint32
	PiColFmt   *int32
	IGroup     int32
}

func loword(v uintptr) uint16 { return uint16(v & 0xffff) }
func getPrimaryWorkArea() rect {
	var r rect
	if ok, _, _ := procSystemParametersInfoW.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&r)), 0); ok != 0 && r.Right > r.Left && r.Bottom > r.Top {
		return r
	}
	w, _, _ := procGetSystemMetrics.Call(smCxScreen)
	h, _, _ := procGetSystemMetrics.Call(smCyScreen)
	if w > 0 && h > 0 {
		return rect{Left: 0, Top: 0, Right: int32(w), Bottom: int32(h)}
	}
	return rect{Left: 0, Top: 0, Right: 1200, Bottom: 720}
}

func hiword(v uintptr) uint16 { return uint16((v >> 16) & 0xffff) }

func utf16Ptr(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func setWindowText(hwnd uintptr, s string) {
	p := utf16Ptr(s)
	procSetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(p)))
}

func getWindowText(hwnd uintptr) string {
	n, _, _ := procGetWindowTextLengthW.Call(hwnd)
	buf := make([]uint16, int(n)+1)
	if len(buf) == 0 {
		return ""
	}
	procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf)
}

func sendMessage(hwnd uintptr, m uint32, w, l uintptr) uintptr {
	r, _, _ := procSendMessageW.Call(hwnd, uintptr(m), w, l)
	return r
}

func postMessage(hwnd uintptr, m uint32, w, l uintptr) {
	procPostMessageW.Call(hwnd, uintptr(m), w, l)
}

func setPos(hwnd uintptr, x, y, w, h int32) {
	const swpNoZOrder = 0x0004
	procSetWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(h), swpNoZOrder)
}

func showWindow(hwnd uintptr, show bool) {
	if hwnd == 0 {
		return
	}
	cmd := uintptr(swHide)
	if show {
		cmd = uintptr(swShow)
	}
	procShowWindow.Call(hwnd, cmd)
}

func invalidateWindow(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	procInvalidateRect.Call(hwnd, 0, 1)
}

func shellOpen(owner uintptr, path string) uintptr {
	verb := utf16Ptr("open")
	p := utf16Ptr(path)
	r, _, _ := procShellExecuteW.Call(owner, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(p)), 0, 0, swShow)
	return r
}

func shellOpenWith(owner uintptr, path string) uintptr {
	verb := utf16Ptr("openas")
	p := utf16Ptr(path)
	r, _, _ := procShellExecuteW.Call(owner, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(p)), 0, 0, swShow)
	return r
}

func pointInWindow(hwnd uintptr, pt point) bool {
	if hwnd == 0 {
		return false
	}
	var r rect
	ok, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	if ok == 0 {
		return false
	}
	return pt.X >= r.Left && pt.X < r.Right && pt.Y >= r.Top && pt.Y < r.Bottom
}

func copyTextToClipboard(owner uintptr, text string) bool {
	ok, _, _ := procOpenClipboard.Call(owner)
	if ok == 0 {
		return false
	}
	defer procCloseClipboard.Call()
	procEmptyClipboard.Call()
	u, _ := syscall.UTF16FromString(text)
	bytes := uintptr(len(u) * 2)
	h, _, _ := procGlobalAlloc.Call(gmemMoveable, bytes)
	if h == 0 {
		return false
	}
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		return false
	}
	dst := unsafe.Slice((*uint16)(unsafe.Pointer(p)), len(u))
	copy(dst, u)
	procGlobalUnlock.Call(h)
	r, _, _ := procSetClipboardData.Call(cfUnicodeText, h)
	return r != 0
}
