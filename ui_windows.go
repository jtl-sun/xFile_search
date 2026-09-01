//go:build windows

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

const (
	idSearch        = 1001
	idFilter        = 1002
	idNarrow        = 1003
	idBack          = 1004
	idClear         = 1005
	idReindex       = 1006
	idCopyPath      = 1007
	idIndexDir      = 1008
	idPreview       = 1009
	idList          = 1100
	idBread         = 1200
	idStatus        = 1201
	idIndexProgress = 1202
	idPreviewHeader = 1300
	idPreviewHost   = 1301
	idPreviewText   = 1302
	idPreviewImage  = 1303
	idSplitter      = 1304
	idPreviewOne    = 1305
	idPreviewFit    = 1306
	idSearchHistory = 1307
)

type searchPayload struct {
	Result SearchResult
	Snap   *IndexSnapshot
	Mode   FilterMode
}

type snapshotPayload struct {
	Snap   *IndexSnapshot
	Reason string
}

type dateCell struct {
	ID        uint32
	DateText  string
	SizeText  string
	SizeBytes int64
	SizeKnown bool
}

type datePayload struct {
	Seq   uint64
	Cells []dateCell
}

type WindowsApp struct {
	hwnd             uintptr
	searchEdit       uintptr
	searchHistoryBtn uintptr
	filterBox        uintptr
	narrowBtn        uintptr
	backBtn          uintptr
	clearBtn         uintptr
	reindexBtn       uintptr
	copyBtn          uintptr
	indexDirBtn      uintptr
	previewCheck     uintptr
	list             uintptr
	bread            uintptr
	status           uintptr
	indexProgress    uintptr
	previewHeader    uintptr
	previewHost      uintptr
	previewText      uintptr
	previewImage     uintptr
	previewOneBtn    uintptr
	previewFitBtn    uintptr
	splitter         uintptr
	previewMgr       *previewManager
	previewEnabled   bool
	previewPercent   int
	previewDragWidth int32
	splitterDragging bool
	previewSeq       atomic.Uint64
	previewPath      atomic.Value // string: file currently shown in Preview
	previewClickMu   sync.Mutex
	previewClickTime uint32
	previewClickPt   point
	previewPanning   bool
	previewPanMoved  bool
	previewPanStart  point
	previewPanLast   point

	listDragActive bool
	listDragRow    int32
	listDragSub    int32
	listDragStart  point

	snapshot atomic.Pointer[IndexSnapshot]

	cfg        Config
	indexPath  string
	configPath string

	steps             []SearchStep
	sessionSnap       *IndexSnapshot
	lastSnap          *IndexSnapshot
	lastResult        SearchResult
	shownIDs          []uint32
	dateTextByID      map[uint32]string
	sizeTextByID      map[uint32]string
	sizeBytesByID     map[uint32]int64
	sortColumn        int
	sortAscending     bool
	deletedPaths      map[string]struct{} // tombstones for files moved to Recycle Bin
	deletedPathFile   string
	searchHistory     []string
	searchHistoryPath string
	historyMu         sync.Mutex
	historySeq        atomic.Uint64
	driveChangeSeq    atomic.Uint64

	pendingListNav      int
	pendingListNavQuery string

	searchMu     sync.Mutex
	searchCancel context.CancelFunc
	searchSeq    atomic.Uint64
	dateSeq      atomic.Uint64

	resultCh   chan searchPayload
	statusCh   chan string
	snapshotCh chan snapshotPayload
	dateCh     chan datePayload

	indexing  atomic.Bool
	loading   atomic.Bool
	workerMu  sync.Mutex
	workerCmd *exec.Cmd

	// Active Windows Shell context-menu interfaces. These are set only while
	// TrackPopupMenuEx is running so owner-draw / submenu messages can be
	// forwarded to third-party shell extensions (antivirus, editors, Send To, etc.).
	shellMenu2    uintptr
	shellMenu3    uintptr
	shellGonePath atomic.Value // string: last path possibly removed by a shell command
}

var winApp *WindowsApp

func NewWindowsApp() *WindowsApp {
	dataDir, idx, cfgPath, logPath := DataPaths()
	setLogPath(logPath)
	// Preserve any custom Roots/settings from v0.1.4 and earlier. The file is
	// tiny, so this migration is safe to do before the window is shown.
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		if oldCfg := legacyConfigPath(); oldCfg != "" && !strings.EqualFold(filepath.Clean(oldCfg), filepath.Clean(cfgPath)) {
			if b, rerr := os.ReadFile(oldCfg); rerr == nil {
				_ = os.WriteFile(cfgPath, b, 0o644)
			}
		}
	}
	_ = EnsureConfig(cfgPath)
	cfg := LoadConfig(cfgPath)
	a := &WindowsApp{
		cfg: cfg, indexPath: idx, configPath: cfgPath, previewPercent: cfg.PreviewWidthPercent,
		resultCh:          make(chan searchPayload, 1),
		statusCh:          make(chan string, 1),
		deletedPaths:      make(map[string]struct{}),
		dateTextByID:      make(map[uint32]string),
		sizeTextByID:      make(map[uint32]string),
		sizeBytesByID:     make(map[uint32]int64),
		sortColumn:        -1,
		deletedPathFile:   filepath.Join(filepath.Dir(idx), "deleted_paths.txt"),
		searchHistoryPath: filepath.Join(dataDir, "SearchHistory.txt"),
		snapshotCh:        make(chan snapshotPayload, 1),
		dateCh:            make(chan datePayload, 1),
	}
	a.loadDeletedPaths()
	a.searchHistory = loadSearchHistory(a.searchHistoryPath, maxSearchHistory)
	a.previewPath.Store("")
	a.shellGonePath.Store("")
	return a
}

func (a *WindowsApp) Run() error {
	// A Win32 window and its message queue are thread-affine. Go goroutines may
	// migrate between OS threads unless explicitly locked. If the UI goroutine
	// moves after CreateWindowExW, Windows messages can remain on the original
	// thread queue and the window is reported as "Not responding". Keep the
	// complete GUI lifetime on one OS thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	winApp = a
	icc := initCommonControlsEx{DwSize: uint32(unsafe.Sizeof(initCommonControlsEx{})), DwICC: iccListViewClasses | iccProgressClass}
	procInitCommonControlsEx.Call(uintptr(unsafe.Pointer(&icc)))

	className := utf16Ptr("xFileSearchWindowClass")
	title := utf16Ptr(appName + " " + appVersion)
	hinst, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
	wc := wndClassEx{
		CbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		Style:         0x0002 | 0x0001,
		LpfnWndProc:   syscall.NewCallback(mainWndProc),
		HInstance:     hinst,
		HCursor:       cursor,
		HbrBackground: colorWindow + 1,
		LpszClassName: className,
	}
	if r, _, e := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return fmt.Errorf("RegisterClassExW failed: %v", e)
	}

	if err := registerSplitterClass(hinst); err != nil {
		return err
	}

	const initialWidth int32 = 1200
	const initialHeight int32 = 720
	work := getPrimaryWorkArea()
	startX, startY := centeredWindowPosition(work.Left, work.Top, work.Right, work.Bottom, initialWidth, initialHeight)

	hwnd, _, e := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		wsOverlappedWindow|wsClipChildren,
		uintptr(startX), uintptr(startY), uintptr(initialWidth), uintptr(initialHeight),
		0, 0, hinst, 0,
	)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowExW failed: %v", e)
	}
	a.hwnd = hwnd
	a.createControls(hinst)
	a.previewMgr = newPreviewManager(a.previewHost, a.previewText, a.previewImage)
	a.layout()
	procShowWindow.Call(hwnd, swShow)
	procUpdateWindow.Call(hwnd)
	procSetFocus.Call(a.searchEdit)

	a.postStatus("Ready · loading index in background...")
	a.startBackgroundLoad()

	var m msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		a.handleListCellDragMessage(&m)
		if a.handlePaneArrowKeyMessage(&m) {
			continue
		}
		if a.handleSearchArrowKeyMessage(&m) {
			continue
		}
		if a.handleDeleteKeyMessage(&m) {
			continue
		}
		if handlePreviewInputMessage(&m) {
			continue
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	a.cancelSearch()
	return nil
}

func (a *WindowsApp) createControls(hinst uintptr) {
	a.searchEdit = createControl(wsExClientEdge, "EDIT", "", wsChild|wsVisible|wsTabStop|esAutoHScroll, idSearch, a.hwnd, hinst)
	a.searchHistoryBtn = createControl(0, "BUTTON", "▼", wsChild|wsVisible|wsTabStop|bsPushButton, idSearchHistory, a.hwnd, hinst)
	a.filterBox = createControl(0, "COMBOBOX", "", wsChild|wsVisible|wsTabStop|cbsDropDownList|wsVScroll, idFilter, a.hwnd, hinst)
	a.narrowBtn = createControl(0, "BUTTON", "Search Within", wsChild|wsVisible|wsTabStop|bsPushButton, idNarrow, a.hwnd, hinst)
	a.backBtn = createControl(0, "BUTTON", "Back", wsChild|wsVisible|wsTabStop|bsPushButton, idBack, a.hwnd, hinst)
	a.clearBtn = createControl(0, "BUTTON", "Clear", wsChild|wsVisible|wsTabStop|bsPushButton, idClear, a.hwnd, hinst)
	a.reindexBtn = createControl(0, "BUTTON", "Reindex", wsChild|wsVisible|wsTabStop|bsPushButton, idReindex, a.hwnd, hinst)
	a.copyBtn = createControl(0, "BUTTON", "Copy Path", wsChild|wsVisible|wsTabStop|bsPushButton, idCopyPath, a.hwnd, hinst)
	a.indexDirBtn = createControl(0, "BUTTON", "Index Folder", wsChild|wsVisible|wsTabStop|bsPushButton, idIndexDir, a.hwnd, hinst)
	a.previewCheck = createControl(0, "BUTTON", "Preview", wsChild|wsVisible|wsTabStop|bsAutoCheckBox, idPreview, a.hwnd, hinst)
	a.bread = createControl(0, "STATIC", "", wsChild|wsVisible, idBread, a.hwnd, hinst)
	a.status = createControl(0, "STATIC", "Ready", wsChild|wsVisible, idStatus, a.hwnd, hinst)
	a.indexProgress = createControl(0, "msctls_progress32", "", wsChild|pbsMarquee, idIndexProgress, a.hwnd, hinst)
	a.list = createControl(wsExClientEdge, "SysListView32", "", wsChild|wsVisible|wsTabStop|lvsReport|lvsShowSelAlways, idList, a.hwnd, hinst)
	a.splitter = createControl(0, "xFileSearchSplitterClass", "", wsChild|wsVisible, idSplitter, a.hwnd, hinst)
	a.previewHeader = createControl(0, "STATIC", "Preview", wsChild|wsVisible, idPreviewHeader, a.hwnd, hinst)
	a.previewHost = createControl(wsExClientEdge, "STATIC", "", wsChild|wsVisible|wsClipChildren, idPreviewHost, a.hwnd, hinst)
	a.previewText = createControl(0, "EDIT", "Select a file to preview.", wsChild|wsVisible|wsVScroll|wsHScroll|esMultiline|esAutoVScroll|esReadOnly|esWantReturn, idPreviewText, a.previewHost, hinst)
	a.previewImage = createControl(0, "STATIC", "", wsChild|ssBitmap, idPreviewImage, a.previewHost, hinst)
	a.previewOneBtn = createControl(0, "BUTTON", "1:1", wsChild|wsVisible|wsTabStop|bsPushButton, idPreviewOne, a.hwnd, hinst)
	a.previewFitBtn = createControl(0, "BUTTON", "Fit Window", wsChild|wsVisible|wsTabStop|bsPushButton, idPreviewFit, a.hwnd, hinst)
	a.previewEnabled = true
	sendMessage(a.previewCheck, bmSetCheck, bstChecked, 0)

	font, _, _ := procGetStockObject.Call(defaultGuiFont)
	controls := []uintptr{a.searchEdit, a.searchHistoryBtn, a.filterBox, a.narrowBtn, a.backBtn, a.clearBtn, a.reindexBtn, a.copyBtn, a.indexDirBtn, a.previewCheck, a.bread, a.status, a.indexProgress, a.list, a.previewHeader, a.previewText, a.previewImage, a.previewOneBtn, a.previewFitBtn}
	for _, h := range controls {
		sendMessage(h, wmSetFont, font, 1)
	}

	for _, s := range []string{"All", "Files", "Folders"} {
		p := utf16Ptr(s)
		sendMessage(a.filterBox, cbAddString, 0, uintptr(unsafe.Pointer(p)))
	}
	sendMessage(a.filterBox, cbSetCurSel, 0, 0)

	sendMessage(a.list, lvmSetExtendedListViewStyle, 0, lvsExFullRowSelect|lvsExDoubleBuffer)
	a.addColumn(0, "Name", 260)
	a.addColumn(1, "Path", 420)
	a.addColumn(2, "Size", 86)
	a.addColumn(3, "Date", 128)
}

func createControl(ex uint32, class, text string, style uint32, id int, parent, hinst uintptr) uintptr {
	c := utf16Ptr(class)
	t := utf16Ptr(text)
	h, _, _ := procCreateWindowExW.Call(
		uintptr(ex), uintptr(unsafe.Pointer(c)), uintptr(unsafe.Pointer(t)), uintptr(style),
		0, 0, 100, 24, parent, uintptr(id), hinst, 0,
	)
	return h
}

func (a *WindowsApp) addColumn(index int32, title string, width int32) {
	t := utf16Ptr(title)
	col := lvColumn{Mask: lvcfText | lvcfWidth | lvcfFmt, Cx: width, PszText: t, ISubItem: index}
	sendMessage(a.list, lvmInsertColumnW, uintptr(index), uintptr(unsafe.Pointer(&col)))
}

func (a *WindowsApp) layout() {
	if a.hwnd == 0 {
		return
	}
	var r rect
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&r)))
	w := r.Right - r.Left
	h := r.Bottom - r.Top
	margin := int32(10)
	top := int32(10)
	buttonH := int32(28)
	fixed := int32(95 + 100 + 72 + 72 + 88 + 82 + 96 + 82)
	searchW := w - fixed - 8*8 - margin*2
	if searchW < 220 {
		searchW = 220
	}
	x := margin
	const historyW int32 = 30
	editW := searchW - historyW
	if editW < 180 {
		editW = 180
	}
	setPos(a.searchEdit, x, top, editW, buttonH)
	setPos(a.searchHistoryBtn, x+editW, top, historyW, buttonH)
	x += searchW + 8
	setPos(a.filterBox, x, top, 95, 240)
	x += 103
	setPos(a.narrowBtn, x, top, 100, buttonH)
	x += 108
	setPos(a.backBtn, x, top, 72, buttonH)
	x += 80
	setPos(a.clearBtn, x, top, 72, buttonH)
	x += 80
	setPos(a.copyBtn, x, top, 88, buttonH)
	x += 96
	setPos(a.reindexBtn, x, top, 82, buttonH)
	x += 90
	setPos(a.indexDirBtn, x, top, 96, buttonH)
	x += 104
	setPos(a.previewCheck, x, top+3, 82, buttonH-2)

	setPos(a.bread, margin+2, 46, w-margin*2-4, 22)
	statusH := int32(24)
	listTop := int32(70)
	listH := h - listTop - statusH - 6
	if listH < 100 {
		listH = 100
	}
	contentW := w - margin*2
	if a.previewEnabled && contentW >= 760 {
		splitterW := previewSplitterWidth
		listW, previewW := previewPaneWidths(contentW, a.previewPercent, a.previewDragWidth)
		setPos(a.list, margin, listTop, listW, listH)
		a.fitListColumns(listW)
		splitterX := margin + listW
		setPos(a.splitter, splitterX, listTop, splitterW, listH)
		previewX := splitterX + splitterW
		const oneW int32 = 46
		const fitW int32 = 84
		const headerGap int32 = 4
		buttonsW := oneW + fitW + headerGap
		headerW := previewW - buttonsW - 10
		if headerW < 60 {
			headerW = 60
		}
		setPos(a.previewHeader, previewX+2, listTop, headerW, 22)
		buttonsX := previewX + previewW - buttonsW - 2
		setPos(a.previewOneBtn, buttonsX, listTop, oneW, 22)
		setPos(a.previewFitBtn, buttonsX+oneW+headerGap, listTop, fitW, 22)
		hostTop := listTop + 24
		hostH := listH - 24
		if hostH < 40 {
			hostH = 40
		}
		setPos(a.previewHost, previewX, hostTop, previewW, hostH)
		setPos(a.previewText, 0, 0, previewW-4, hostH-4)
		if a.previewMgr == nil {
			setPos(a.previewImage, 0, 0, previewW-4, hostH-4)
		}
		showWindow(a.splitter, true)
		showWindow(a.previewHeader, true)
		showWindow(a.previewOneBtn, true)
		showWindow(a.previewFitBtn, true)
		showWindow(a.previewHost, true)
		if a.previewMgr != nil {
			a.previewMgr.Resize(previewW-4, hostH-4)
		}
	} else {
		setPos(a.list, margin, listTop, contentW, listH)
		a.fitListColumns(contentW)
		showWindow(a.splitter, false)
		showWindow(a.previewHeader, false)
		showWindow(a.previewOneBtn, false)
		showWindow(a.previewFitBtn, false)
		showWindow(a.previewHost, false)
	}
	if a.indexing.Load() && a.indexProgress != 0 {
		const progressW int32 = 220
		const progressGap int32 = 8
		statusW := w - margin*2 - progressW - progressGap - 4
		if statusW < 120 {
			statusW = 120
		}
		setPos(a.status, margin+2, h-statusH, statusW, statusH)
		setPos(a.indexProgress, w-margin-progressW, h-statusH+4, progressW, statusH-8)
		showWindow(a.indexProgress, true)
	} else {
		setPos(a.status, margin+2, h-statusH, w-margin*2-4, statusH)
		showWindow(a.indexProgress, false)
	}
}

func mainWndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	a := winApp
	switch message {
	case wmInitMenuPopup, wmDrawItem, wmMeasureItem, wmMenuChar:
		if a != nil {
			if result, handled := a.forwardShellContextMenuMessage(message, wParam, lParam); handled {
				return result
			}
		}
	case wmDeviceChange:
		if a != nil && (wParam == dbtDeviceArrival || wParam == dbtDeviceRemoveComplete || wParam == dbtDevNodesChanged) {
			a.scheduleDriveRefreshCheck()
			return 1
		}
	case wmActivateApp:
		// When the user returns from another application, restore the result list
		// as the keyboard navigation target whenever a result is already selected.
		// Post the request so Windows can finish its activation/focus transition
		// first; Up/Down then continues image browsing immediately without a click.
		if a != nil && wParam != 0 && a.list != 0 && a.selectedRowIndex() >= 0 {
			postMessage(a.hwnd, wmAppFocusList, 0, 0)
		}
		return 0
	case wmSize:
		if a != nil {
			a.layout()
		}
		return 0
	case wmCommand:
		if a == nil {
			break
		}
		id := int(loword(wParam))
		code := hiword(wParam)
		switch id {
		case idSearch:
			if code == enChange {
				// Any text edit cancels a pending arrow-to-results handoff.
				// A new arrow press will capture the new query and force an immediate search.
				a.pendingListNav = 0
				a.pendingListNavQuery = ""
				a.historySeq.Add(1)
				a.scheduleSearch()
			}
		case idSearchHistory:
			if code == bnClicked {
				a.showSearchHistoryMenu()
			}
		case idFilter:
			if code == cbnSelChange {
				a.scheduleSearch()
			}
		case idNarrow:
			if code == bnClicked {
				a.narrowCurrent()
			}
		case idBack:
			if code == bnClicked {
				a.goBack()
			}
		case idClear:
			if code == bnClicked {
				a.clearSession()
			}
		case idReindex:
			if code == bnClicked {
				a.startReindex(true)
			}
		case idCopyPath:
			if code == bnClicked {
				a.copySelectedPath()
			}
		case idIndexDir:
			if code == bnClicked {
				a.openIndexFolder()
			}
		case idPreview:
			if code == bnClicked {
				a.previewEnabled = sendMessage(a.previewCheck, bmGetCheck, 0, 0) == bstChecked
				if !a.previewEnabled && a.previewMgr != nil {
					a.previewMgr.Clear()
				}
				a.layout()
				if a.previewEnabled {
					a.schedulePreview()
				}
			}
		case idPreviewOne:
			if code == bnClicked && a.previewMgr != nil {
				a.previewMgr.OneToOne()
			}
		case idPreviewFit:
			if code == bnClicked && a.previewMgr != nil {
				a.previewMgr.FitWindow()
			}
		}
		return 0
	case wmNotify:
		if a != nil && lParam != 0 {
			hdr := (*nmhdr)(unsafe.Pointer(lParam))
			if int(hdr.IdFrom) == idList {
				if hdr.Code == nmRClick {
					nmia := (*nmItemActivate)(unsafe.Pointer(lParam))
					if nmia.IItem >= 0 {
						a.selectListRow(int(nmia.IItem))
						var pt point
						if ok, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt))); ok != 0 {
							a.showSelectedShellContextMenu(pt)
						}
					}
					return 0
				}
				if hdr.Code == lvnColumnClick {
					nmlv := (*nmListView)(unsafe.Pointer(lParam))
					a.sortByColumn(int(nmlv.ISubItem))
					return 0
				}
				if hdr.Code == nmDblClk {
					a.openSelected()
					return 0
				}
				if hdr.Code == lvnKeyDown {
					nmk := (*nmLVKeyDown)(unsafe.Pointer(lParam))
					switch nmk.WVKey {
					case vkReturn:
						// Enter opens the currently selected result with the Windows
						// default/associated application, matching double-click.
						a.openSelected()
						return 0
					case vkDelete:
						a.deleteSelectedImage()
						return 0
					}
				}
				if hdr.Code == lvnItemChanged {
					nmlv := (*nmListView)(unsafe.Pointer(lParam))
					if nmlv.UChanged&lvifState != 0 && nmlv.UNewState&lvisSelected != 0 {
						a.schedulePreview()
					}
				}
			}
		}
	case wmAppSearch:
		if a != nil {
			a.beginSearchFromUI()
		}
		return 0
	case wmAppResults:
		if a != nil {
			a.consumeResult()
		}
		return 0
	case wmAppStatus:
		if a != nil {
			a.consumeStatus()
		}
		return 0
	case wmAppSnapshot:
		if a != nil {
			a.consumeSnapshot()
		}
		return 0
	case wmAppPreview:
		if a != nil {
			a.beginPreviewFromUI()
		}
		return 0
	case wmAppFocusList:
		if a != nil {
			a.focusFileListPane()
		}
		return 0
	case wmAppDates:
		if a != nil {
			a.consumeVisibleDates()
		}
		return 0
	case wmAppShellPathGone:
		if a != nil {
			a.consumeShellGonePath()
		}
		return 0
	case wmAppDriveCheck:
		if a != nil {
			a.handleDriveRefreshCheck()
		}
		return 0
	case wmAppIndexingState:
		if a != nil {
			a.applyIndexingVisual(wParam != 0)
		}
		return 0
	case wmClose:
		if a != nil {
			_ = SavePreviewWidthPercent(a.configPath, a.previewPercent)
			a.stopIndexer()
			if a.previewMgr != nil {
				a.previewMgr.Close()
			}
		}
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
	return r
}

func registerSplitterClass(hinst uintptr) error {
	className := utf16Ptr("xFileSearchSplitterClass")
	cursor, _, _ := procLoadCursorW.Call(0, idcSizeWE)
	wc := wndClassEx{
		CbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		Style:         0x0008, // CS_DBLCLKS
		LpfnWndProc:   syscall.NewCallback(splitterWndProc),
		HInstance:     hinst,
		HCursor:       cursor,
		HbrBackground: colorButtonFace + 1,
		LpszClassName: className,
	}
	if r, _, e := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return fmt.Errorf("Register splitter class failed: %v", e)
	}
	return nil
}

func splitterWndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	a := winApp
	switch message {
	case wmLButtonDown:
		if a != nil && hwnd == a.splitter {
			a.splitterDragging = true
			procSetCapture.Call(hwnd)
			a.resizePreviewFromCursor()
		}
		return 0
	case wmMouseMove:
		if a != nil && a.splitterDragging && hwnd == a.splitter {
			a.resizePreviewFromCursor()
		}
		return 0
	case wmLButtonUp:
		if a != nil && a.splitterDragging && hwnd == a.splitter {
			a.resizePreviewFromCursor()
			a.splitterDragging = false
			procReleaseCapture.Call()
			// Convert the exact drag width to a saved percentage only after drag
			// finishes. During drag, layout uses pixels so the pane cannot jitter.
			var rc rect
			procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
			contentW := rc.Right - rc.Left - 20
			if a.previewDragWidth > 0 {
				a.previewPercent = previewPercentForWidth(contentW, a.previewDragWidth)
			}
			a.previewDragWidth = 0
			a.layout()
			_ = SavePreviewWidthPercent(a.configPath, a.previewPercent)
		}
		return 0
	case wmLButtonDblClk:
		if a != nil && hwnd == a.splitter {
			a.previewDragWidth = 0
			a.previewPercent = previewDefaultPercent
			a.layout()
			_ = SavePreviewWidthPercent(a.configPath, a.previewPercent)
		}
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
	return r
}

func (a *WindowsApp) resizePreviewFromCursor() {
	if a == nil || a.hwnd == 0 || !a.previewEnabled {
		return
	}
	var pt point
	if r, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt))); r == 0 {
		return
	}
	procScreenToClient.Call(a.hwnd, uintptr(unsafe.Pointer(&pt)))
	var rc rect
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
	margin := int32(10)
	contentW := rc.Right - rc.Left - margin*2
	if contentW <= 0 {
		return
	}
	listW := pt.X - margin
	if listW < previewMinListWidth {
		listW = previewMinListWidth
	}
	maxListW := contentW - previewSplitterWidth - previewMinWidth
	if listW > maxListW {
		listW = maxListW
	}
	previewW := contentW - previewSplitterWidth - listW
	a.previewDragWidth = previewW
	a.layout()
}

func (a *WindowsApp) fitListColumns(listWidth int32) {
	if a == nil || a.list == 0 {
		return
	}
	nameW, pathW, sizeW, dateW := resultColumnWidths(listWidth)
	for col, width := range []int32{nameW, pathW, sizeW, dateW} {
		sendMessage(a.list, lvmSetColumnWidth, uintptr(col), uintptr(width))
	}
}

func (a *WindowsApp) scheduleVisibleDates(ids []uint32, paths []string) {
	if a == nil || a.hwnd == 0 {
		return
	}
	seq := a.dateSeq.Add(1)
	copyIDs := append([]uint32(nil), ids...)
	copyPaths := append([]string(nil), paths...)
	go func() {
		n := len(copyPaths)
		if len(copyIDs) < n {
			n = len(copyIDs)
		}
		cells := make([]dateCell, n)
		for i := 0; i < n; i++ {
			cell := dateCell{ID: copyIDs[i]}
			if st, err := os.Stat(copyPaths[i]); err == nil {
				cell.DateText = formatModifiedDate(st.ModTime())
				cell.SizeKnown = true
				if st.IsDir() {
					cell.SizeBytes = -1
					cell.SizeText = "—"
				} else {
					cell.SizeBytes = st.Size()
					cell.SizeText = formatFileSize(st.Size())
				}
			}
			cells[i] = cell
			if a.dateSeq.Load() != seq {
				return
			}
		}
		p := datePayload{Seq: seq, Cells: cells}
		select {
		case a.dateCh <- p:
		default:
			select {
			case <-a.dateCh:
			default:
			}
			a.dateCh <- p
		}
		postMessage(a.hwnd, wmAppDates, 0, 0)
	}()
}

func (a *WindowsApp) consumeVisibleDates() {
	if a == nil {
		return
	}
	select {
	case p := <-a.dateCh:
		if p.Seq != a.dateSeq.Load() {
			return
		}
		if a.dateTextByID == nil {
			a.dateTextByID = make(map[uint32]string)
		}
		if a.sizeTextByID == nil {
			a.sizeTextByID = make(map[uint32]string)
		}
		if a.sizeBytesByID == nil {
			a.sizeBytesByID = make(map[uint32]int64)
		}
		for _, cell := range p.Cells {
			dateText := cell.DateText
			if dateText == "" {
				dateText = "—"
			}
			a.dateTextByID[cell.ID] = dateText
			if cell.SizeKnown {
				sizeText := cell.SizeText
				if sizeText == "" {
					sizeText = "—"
				}
				a.sizeTextByID[cell.ID] = sizeText
				a.sizeBytesByID[cell.ID] = cell.SizeBytes
			} else {
				a.sizeTextByID[cell.ID] = "—"
			}
		}

		// A header sort can rearrange rows while background metadata is loading.
		// Re-sort by stable result ID when Size or Date is the active column.
		if (a.sortColumn == 2 || a.sortColumn == 3) && len(a.shownIDs) > 1 {
			a.applyVisibleSort(false)
			return
		}
		rowByID := make(map[uint32]int, len(a.shownIDs))
		for row, id := range a.shownIDs {
			rowByID[id] = row
		}
		for _, cell := range p.Cells {
			row, ok := rowByID[cell.ID]
			if !ok {
				continue
			}
			for sub, text := range map[int]string{2: a.sizeTextByID[cell.ID], 3: a.dateTextByID[cell.ID]} {
				pt := utf16Ptr(text)
				it := lvItem{Mask: lvifText, IItem: int32(row), ISubItem: int32(sub), PszText: pt}
				sendMessage(a.list, lvmSetItemW, 0, uintptr(unsafe.Pointer(&it)))
			}
		}
	default:
	}
}

// handleListCellDragMessage lets users copy the displayed Name or Path cell
// with a simple mouse drag. The normal ListView selection/scroll behavior is
// left intact; when a drag starts and ends on the same Name/Path cell, that
// cell's text is copied to the clipboard on mouse-up.
func (a *WindowsApp) handleListCellDragMessage(m *msg) {
	if a == nil || m == nil || a.list == 0 {
		return
	}
	switch m.Message {
	case wmLButtonDown:
		if m.Hwnd != a.list {
			return
		}
		row, sub, ok := a.listSubItemAtScreenPoint(m.Pt)
		if !ok || (sub != 0 && sub != 1) {
			a.listDragActive = false
			return
		}
		a.listDragActive = true
		a.listDragRow = row
		a.listDragSub = sub
		a.listDragStart = m.Pt
	case wmLButtonUp:
		if !a.listDragActive {
			return
		}
		startRow, startSub, startPt := a.listDragRow, a.listDragSub, a.listDragStart
		a.listDragActive = false
		if m.Hwnd != a.list || absI32(m.Pt.X-startPt.X) < 7 && absI32(m.Pt.Y-startPt.Y) < 7 {
			return
		}
		row, sub, ok := a.listSubItemAtScreenPoint(m.Pt)
		if !ok || row != startRow || sub != startSub {
			return
		}
		text, label, ok := a.listCellText(int(row), int(sub))
		if !ok || text == "" {
			return
		}
		if copyTextToClipboard(a.hwnd, text) {
			a.setStatus(label + " copied: " + text)
		}
	}
}

func (a *WindowsApp) listSubItemAtScreenPoint(screen point) (row, sub int32, ok bool) {
	pt := screen
	procScreenToClient.Call(a.list, uintptr(unsafe.Pointer(&pt)))
	hit := lvHitTestInfo{Pt: pt, IItem: -1, ISubItem: -1}
	r := int32(sendMessage(a.list, lvmSubItemHitTest, 0, uintptr(unsafe.Pointer(&hit))))
	if r < 0 || hit.IItem < 0 {
		return -1, -1, false
	}
	return hit.IItem, hit.ISubItem, true
}

func (a *WindowsApp) listCellText(row, sub int) (string, string, bool) {
	if a == nil || row < 0 || row >= len(a.shownIDs) || (sub != 0 && sub != 1) {
		return "", "", false
	}
	snap := a.lastSnap
	if len(a.steps) > 0 {
		snap = a.sessionSnap
	}
	if snap == nil {
		snap = a.snapshot.Load()
	}
	if snap == nil {
		return "", "", false
	}
	e, ok := snap.EntryAt(a.shownIDs[row])
	if !ok {
		return "", "", false
	}
	if sub == 0 {
		return e.Name(), "Name", true
	}
	return filepath.Dir(e.Path), "Path", true
}

func (a *WindowsApp) scheduleRememberSearch(query string) {
	query = strings.TrimSpace(query)
	if query == "" {
		return
	}
	seq := a.historySeq.Add(1)
	go func(q string, want uint64) {
		time.Sleep(1100 * time.Millisecond)
		if a.historySeq.Load() != want {
			return
		}
		a.historyMu.Lock()
		a.searchHistory = rememberSearchHistory(a.searchHistory, q, maxSearchHistory)
		items := append([]string(nil), a.searchHistory...)
		path := a.searchHistoryPath
		a.historyMu.Unlock()
		_ = saveSearchHistory(path, items)
	}(query, seq)
}

func (a *WindowsApp) showSearchHistoryMenu() {
	if a == nil || a.hwnd == 0 || a.searchHistoryBtn == 0 {
		return
	}
	a.historyMu.Lock()
	items := append([]string(nil), a.searchHistory...)
	a.historyMu.Unlock()

	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)

	if len(items) == 0 {
		label := utf16Ptr("No recent searches")
		procAppendMenuW.Call(menu, mfString|mfGrayEd, 0, uintptr(unsafe.Pointer(label)))
	} else {
		showN := len(items)
		if showN > maxSearchHistoryMenu {
			showN = maxSearchHistoryMenu
		}
		for i := 0; i < showN; i++ {
			labelText := menuSafeHistoryLabel(items[i])
			label := utf16Ptr(labelText)
			procAppendMenuW.Call(menu, mfString, uintptr(searchHistoryCommandBase+i), uintptr(unsafe.Pointer(label)))
		}
		procAppendMenuW.Call(menu, mfSeparator, 0, 0)
		clearLabel := utf16Ptr("Clear Search History")
		procAppendMenuW.Call(menu, mfString, searchHistoryClearCommand, uintptr(unsafe.Pointer(clearLabel)))
	}

	var r rect
	procGetWindowRect.Call(a.searchEdit, uintptr(unsafe.Pointer(&r)))
	var br rect
	procGetWindowRect.Call(a.searchHistoryBtn, uintptr(unsafe.Pointer(&br)))
	x := r.Left
	y := br.Bottom
	cmd, _, _ := procTrackPopupMenuEx.Call(menu, tpmLeftAlign|tpmTopAlign|tpmReturnCmd|tpmRightButton, uintptr(x), uintptr(y), a.hwnd, 0)
	if cmd == 0 {
		procSetFocus.Call(a.searchEdit)
		return
	}
	if cmd == searchHistoryClearCommand {
		a.historyMu.Lock()
		a.searchHistory = nil
		path := a.searchHistoryPath
		a.historyMu.Unlock()
		_ = saveSearchHistory(path, nil)
		procSetFocus.Call(a.searchEdit)
		return
	}
	idx := int(cmd) - searchHistoryCommandBase
	if idx >= 0 && idx < len(items) {
		q := items[idx]
		setWindowText(a.searchEdit, q)
		procSetFocus.Call(a.searchEdit)
		a.searchSeq.Add(1)
		postMessage(a.hwnd, wmAppSearch, 0, 0)
	}
}

func (a *WindowsApp) currentFilter() FilterMode {
	sel := int32(sendMessage(a.filterBox, cbGetCurSel, 0, 0))
	switch sel {
	case 1:
		return FilterFiles
	case 2:
		return FilterFolders
	default:
		return FilterAll
	}
}

func (a *WindowsApp) scheduleSearch() {
	seq := a.searchSeq.Add(1)
	go func() {
		time.Sleep(110 * time.Millisecond)
		if a.searchSeq.Load() == seq {
			postMessage(a.hwnd, wmAppSearch, 0, 0)
		}
	}()
}

func (a *WindowsApp) beginSearchFromUI() {
	raw := strings.TrimSpace(getWindowText(a.searchEdit))
	if raw == "" {
		a.cancelSearch()
		if len(a.steps) > 0 {
			a.showIDs(a.steps[len(a.steps)-1].IDs, a.sessionSnap, fmt.Sprintf("%s · %s", a.breadcrumb(), formatCount(len(a.steps[len(a.steps)-1].IDs))+" matches"))
		} else {
			a.clearList()
			snap := a.snapshot.Load()
			if snap != nil {
				a.setStatus(fmt.Sprintf("Ready · %s items indexed", formatCount(snap.Len())))
			}
		}
		return
	}

	snap := a.snapshot.Load()
	base := []uint32(nil)
	if len(a.steps) > 0 {
		snap = a.sessionSnap
		base = a.steps[len(a.steps)-1].IDs
	}
	if snap == nil {
		a.setStatus("First index is building in background. The window stays usable; search starts automatically when ready.")
		return
	}
	mode := a.currentFilter()
	a.cancelSearch()
	ctx, cancel := context.WithCancel(context.Background())
	a.searchMu.Lock()
	a.searchCancel = cancel
	a.searchMu.Unlock()
	searchBusy.Store(true)
	a.setStatus("Searching...")
	go func(s *IndexSnapshot, b []uint32, q string, m FilterMode) {
		res := Search(ctx, s, b, q, m)
		searchBusy.Store(false)
		if res.Canceled {
			return
		}
		p := searchPayload{Result: res, Snap: s, Mode: m}
		select {
		case a.resultCh <- p:
		default:
			<-a.resultCh
			a.resultCh <- p
		}
		postMessage(a.hwnd, wmAppResults, 0, 0)
	}(snap, base, raw, mode)
}

func (a *WindowsApp) cancelSearch() {
	a.searchMu.Lock()
	if a.searchCancel != nil {
		a.searchCancel()
		a.searchCancel = nil
	}
	a.searchMu.Unlock()
	searchBusy.Store(false)
}

func (a *WindowsApp) consumeResult() {
	var p searchPayload
	select {
	case p = <-a.resultCh:
	default:
		return
	}
	if strings.TrimSpace(getWindowText(a.searchEdit)) != strings.TrimSpace(p.Result.Query) {
		return
	}
	a.lastResult = p.Result
	a.lastSnap = p.Snap
	a.showIDs(p.Result.IDs, p.Snap, "")
	shown := len(p.Result.IDs)
	if shown > a.cfg.MaxDisplayResults {
		shown = a.cfg.MaxDisplayResults
	}
	status := fmt.Sprintf("%s matches · %s shown · %.1f ms · %s indexed",
		formatCount(len(p.Result.IDs)), formatCount(shown), float64(p.Result.Elapsed.Microseconds())/1000.0, formatCount(p.Snap.Len()))
	if a.indexing.Load() {
		status += " · INDEXING continues in background"
	}
	a.setStatus(status)
	a.scheduleRememberSearch(p.Result.Query)

	// If Up/Down was pressed in the search box before this search finished,
	// hand keyboard focus to the result list as soon as the matching result arrives.
	if a.pendingListNav != 0 && strings.EqualFold(strings.TrimSpace(a.pendingListNavQuery), strings.TrimSpace(p.Result.Query)) {
		dir := a.pendingListNav
		a.pendingListNav = 0
		a.pendingListNavQuery = ""
		a.focusResultListFromSearch(dir)
	}
}

func (a *WindowsApp) showIDs(ids []uint32, snap *IndexSnapshot, statusOverride string) {
	if snap == nil {
		return
	}
	if a.previewMgr != nil {
		a.previewMgr.Clear()
	}
	a.previewPath.Store("")
	setWindowText(a.previewHeader, "Preview")
	limit := len(ids)
	if limit > a.cfg.MaxDisplayResults {
		limit = a.cfg.MaxDisplayResults
	}
	a.shownIDs = a.shownIDs[:0]
	a.dateTextByID = make(map[uint32]string, limit)
	a.sizeTextByID = make(map[uint32]string, limit)
	a.sizeBytesByID = make(map[uint32]int64, limit)
	visiblePaths := make([]string, 0, limit)
	sendMessage(a.list, wmSetRedraw, 0, 0)
	sendMessage(a.list, lvmDeleteAllItems, 0, 0)
	row := 0
	for i := 0; i < len(ids) && row < limit; i++ {
		id := ids[i]
		e, ok := snap.EntryAt(id)
		if !ok || a.isSessionDeleted(e.Path) {
			continue
		}
		a.shownIDs = append(a.shownIDs, id)
		visiblePaths = append(visiblePaths, e.Path)
		a.insertRow(row, e.Name(), filepath.Dir(e.Path), "…", "…")
		row++
	}
	sendMessage(a.list, wmSetRedraw, 1, 0)
	dateIDs := append([]uint32(nil), a.shownIDs...)
	a.scheduleVisibleDates(dateIDs, visiblePaths)
	if a.sortColumn >= 0 && a.sortColumn != 2 && a.sortColumn != 3 && len(a.shownIDs) > 1 {
		a.applyVisibleSort(false)
	}
	if statusOverride != "" {
		a.setStatus(statusOverride)
	}
}

func (a *WindowsApp) insertRow(row int, name, path, size, date string) {
	pName := utf16Ptr(name)
	item := lvItem{Mask: lvifText, IItem: int32(row), ISubItem: 0, PszText: pName}
	sendMessage(a.list, lvmInsertItemW, 0, uintptr(unsafe.Pointer(&item)))
	for sub, text := range []string{path, size, date} {
		p := utf16Ptr(text)
		it := lvItem{Mask: lvifText, IItem: int32(row), ISubItem: int32(sub + 1), PszText: p}
		sendMessage(a.list, lvmSetItemW, 0, uintptr(unsafe.Pointer(&it)))
	}
}

func (a *WindowsApp) sortByColumn(column int) {
	if a == nil || column < 0 || column > 3 || len(a.shownIDs) == 0 {
		return
	}
	if a.sortColumn == column {
		a.sortAscending = !a.sortAscending
	} else {
		a.sortColumn = column
		a.sortAscending = true
	}
	a.applyVisibleSort(true)
}

func (a *WindowsApp) applyVisibleSort(showStatus bool) {
	if a == nil || a.sortColumn < 0 || a.sortColumn > 3 || len(a.shownIDs) < 2 {
		return
	}
	snap := a.currentListSnapshot()
	if snap == nil {
		return
	}

	selectedID, hadSelection := a.selectedShownID()
	items := make([]visibleSortItem, 0, len(a.shownIDs))
	for _, id := range a.shownIDs {
		e, ok := snap.EntryAt(id)
		if !ok {
			continue
		}
		date := ""
		if a.dateTextByID != nil {
			date = a.dateTextByID[id]
		}
		sizeText := ""
		if a.sizeTextByID != nil {
			sizeText = a.sizeTextByID[id]
		}
		sizeBytes, sizeKnown := int64(0), false
		if a.sizeBytesByID != nil {
			sizeBytes, sizeKnown = a.sizeBytesByID[id]
		}
		items = append(items, visibleSortItem{
			ID: id, Name: e.Name(), Path: filepath.Dir(e.Path), SizeText: sizeText, SizeBytes: sizeBytes, SizeKnown: sizeKnown, Date: date,
		})
	}
	if len(items) < 2 {
		return
	}
	sortVisibleItems(items, a.sortColumn, a.sortAscending)
	for i := range items {
		a.shownIDs[i] = items[i].ID
	}
	a.shownIDs = a.shownIDs[:len(items)]
	a.rebuildVisibleRows(snap, selectedID, hadSelection)

	if showStatus {
		dir := "ascending"
		if !a.sortAscending {
			dir = "descending"
		}
		name := []string{"Name", "Path", "Size", "Date"}[a.sortColumn]
		a.setStatus(fmt.Sprintf("Sorted by %s · %s · %s shown", name, dir, formatCount(len(a.shownIDs))))
	}
}

func (a *WindowsApp) currentListSnapshot() *IndexSnapshot {
	if a == nil {
		return nil
	}
	if len(a.steps) > 0 && a.sessionSnap != nil {
		return a.sessionSnap
	}
	if a.lastSnap != nil {
		return a.lastSnap
	}
	return a.snapshot.Load()
}

func (a *WindowsApp) selectedShownID() (uint32, bool) {
	if a == nil {
		return 0, false
	}
	row := a.selectedRowIndex()
	if row < 0 || row >= len(a.shownIDs) {
		return 0, false
	}
	return a.shownIDs[row], true
}

func (a *WindowsApp) rebuildVisibleRows(snap *IndexSnapshot, selectedID uint32, hadSelection bool) {
	if a == nil || snap == nil {
		return
	}
	sendMessage(a.list, wmSetRedraw, 0, 0)
	sendMessage(a.list, lvmDeleteAllItems, 0, 0)
	selectedRow := -1
	for row, id := range a.shownIDs {
		e, ok := snap.EntryAt(id)
		if !ok {
			continue
		}
		size := "…"
		if text, ok := a.sizeTextByID[id]; ok && text != "" {
			size = text
		}
		date := "…"
		if text, ok := a.dateTextByID[id]; ok && text != "" {
			date = text
		}
		a.insertRow(row, e.Name(), filepath.Dir(e.Path), size, date)
		if hadSelection && id == selectedID {
			selectedRow = row
		}
	}
	if selectedRow >= 0 {
		state := lvItem{State: lvisSelected | lvisFocused, StateMask: lvisSelected | lvisFocused}
		sendMessage(a.list, lvmSetItemState, uintptr(selectedRow), uintptr(unsafe.Pointer(&state)))
		sendMessage(a.list, lvmEnsureVisible, uintptr(selectedRow), 0)
	}
	sendMessage(a.list, wmSetRedraw, 1, 0)
	procInvalidateRect.Call(a.list, 0, 1)
}

func (a *WindowsApp) clearList() {
	a.dateSeq.Add(1)
	a.shownIDs = a.shownIDs[:0]
	a.dateTextByID = make(map[uint32]string)
	a.sizeTextByID = make(map[uint32]string)
	a.sizeBytesByID = make(map[uint32]int64)
	sendMessage(a.list, lvmDeleteAllItems, 0, 0)
	if a.previewMgr != nil {
		a.previewMgr.Clear()
	}
	a.previewPath.Store("")
	setWindowText(a.previewHeader, "Preview")
}

func (a *WindowsApp) narrowCurrent() {
	raw := strings.TrimSpace(getWindowText(a.searchEdit))
	if raw == "" || len(a.lastResult.IDs) == 0 || a.lastSnap == nil || strings.TrimSpace(a.lastResult.Query) != raw {
		a.setStatus("Type a search and wait for the result, then click Search Within.")
		return
	}
	if len(a.steps) == 0 {
		a.sessionSnap = a.lastSnap
	}
	if a.lastSnap != a.sessionSnap {
		a.setStatus("The index changed. Clear the search before starting a new Search Within session.")
		return
	}
	ids := append([]uint32(nil), a.lastResult.IDs...)
	a.steps = append(a.steps, SearchStep{Query: raw, IDs: ids})
	a.updateBreadcrumb()
	setWindowText(a.searchEdit, "")
	a.lastResult = SearchResult{}
	a.showIDs(ids, a.sessionSnap, fmt.Sprintf("Search Within ready · %s matches", formatCount(len(ids))))
	procSetFocus.Call(a.searchEdit)
}

func (a *WindowsApp) goBack() {
	if len(a.steps) == 0 {
		return
	}
	a.steps = a.steps[:len(a.steps)-1]
	setWindowText(a.searchEdit, "")
	a.lastResult = SearchResult{}
	if len(a.steps) == 0 {
		a.sessionSnap = nil
		a.clearList()
		a.updateBreadcrumb()
		snap := a.snapshot.Load()
		if snap != nil {
			a.setStatus(fmt.Sprintf("Ready · %s items indexed", formatCount(snap.Len())))
		}
	} else {
		a.updateBreadcrumb()
		step := a.steps[len(a.steps)-1]
		a.showIDs(step.IDs, a.sessionSnap, fmt.Sprintf("%s matches", formatCount(len(step.IDs))))
	}
	procSetFocus.Call(a.searchEdit)
}

func (a *WindowsApp) clearSession() {
	a.cancelSearch()
	a.steps = nil
	a.sessionSnap = nil
	a.lastResult = SearchResult{}
	a.lastSnap = nil
	setWindowText(a.searchEdit, "")
	a.updateBreadcrumb()
	a.clearList()
	snap := a.snapshot.Load()
	if snap != nil {
		a.setStatus(fmt.Sprintf("Ready · %s items indexed", formatCount(snap.Len())))
	}
	procSetFocus.Call(a.searchEdit)
}

func (a *WindowsApp) breadcrumb() string {
	if len(a.steps) == 0 {
		return ""
	}
	parts := make([]string, len(a.steps))
	for i, s := range a.steps {
		parts[i] = s.Query
	}
	return strings.Join(parts, "  >  ")
}

func (a *WindowsApp) updateBreadcrumb() {
	s := a.breadcrumb()
	if s == "" {
		s = "Search → Search Within → Search Within  (results get narrower and faster)"
	}
	setWindowText(a.bread, s)
}

func (a *WindowsApp) selectedEntry() (Entry, bool) {
	idx := int32(sendMessage(a.list, lvmGetNextItem, ^uintptr(0), lvniSelected))
	if idx < 0 || int(idx) >= len(a.shownIDs) {
		return Entry{}, false
	}
	snap := a.lastSnap
	if len(a.steps) > 0 {
		snap = a.sessionSnap
	}
	if snap == nil {
		snap = a.snapshot.Load()
	}
	id := a.shownIDs[idx]
	if snap == nil {
		return Entry{}, false
	}
	return snap.EntryAt(id)
}

func (a *WindowsApp) schedulePreview() {
	if a == nil || !a.previewEnabled || a.previewMgr == nil {
		return
	}
	seq := a.previewSeq.Add(1)
	go func() {
		time.Sleep(90 * time.Millisecond)
		if a.previewSeq.Load() == seq && a.hwnd != 0 {
			postMessage(a.hwnd, wmAppPreview, 0, 0)
		}
	}()
}

func (a *WindowsApp) beginPreviewFromUI() {
	if a == nil || !a.previewEnabled || a.previewMgr == nil {
		return
	}
	e, ok := a.selectedEntry()
	if !ok {
		a.previewPath.Store("")
		setWindowText(a.previewHeader, "Preview")
		a.previewMgr.Clear()
		return
	}
	a.previewPath.Store(e.Path)
	setWindowText(a.previewHeader, "Preview · "+e.Name()+"  ·  Wheel: Zoom at cursor  ·  Drag: Pan image  ·  Double-click: Open")
	a.previewMgr.Show(e.Path)
}

func (a *WindowsApp) deleteSelectedImage() {
	selectedRow := a.selectedRowIndex()
	e, ok := a.selectedEntry()
	if !ok {
		return
	}
	if e.IsDir {
		a.setStatus("Del: folders are not deleted from xFile_search.")
		return
	}
	if !isImagePreviewPath(e.Path) {
		a.setStatus("Del: image files only (JPG/PNG/GIF/BMP/WEBP/TIFF).")
		return
	}
	if _, err := os.Stat(e.Path); err != nil {
		a.markSessionDeleted(e.Path)
		a.refreshAfterDelete(selectedRow)
		a.setStatus("File is already missing; removed from current results.")
		return
	}

	msg := "선택한 이미지 파일을 휴지통으로 이동할까요?\r\n\r\n" + e.Path
	if !askYesNo(a.hwnd, "이미지 삭제", msg) {
		// Return keyboard navigation to the result list after the modal dialog.
		procSetFocus.Call(a.list)
		return
	}

	// Release any preview-handler file handle before requesting the move.
	if a.previewMgr != nil {
		a.previewMgr.Clear()
	}
	a.previewPath.Store("")
	setWindowText(a.previewHeader, "Preview")

	if err := moveFileToRecycleBin(a.hwnd, e.Path); err != nil {
		showInfo("이미지 파일을 삭제할 수 없습니다.\n\n" + e.Path + "\n\n" + err.Error())
		a.setStatus("Delete failed: " + err.Error())
		procSetFocus.Call(a.list)
		a.schedulePreview()
		return
	}

	a.markSessionDeleted(e.Path)
	a.refreshAfterDelete(selectedRow)
	a.setStatus("Moved to Recycle Bin: " + e.Path)
}

func (a *WindowsApp) loadDeletedPaths() {
	if a == nil || a.deletedPathFile == "" {
		return
	}
	b, err := os.ReadFile(a.deletedPathFile)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(b), "\n") {
		path := strings.TrimSpace(line)
		if path == "" {
			continue
		}
		// If a user restored the file from Recycle Bin, do not keep hiding it.
		if _, err := os.Stat(path); err == nil {
			continue
		}
		a.deletedPaths[strings.ToLower(filepath.Clean(path))] = struct{}{}
	}
}

func (a *WindowsApp) markSessionDeleted(path string) {
	if a == nil || path == "" {
		return
	}
	if a.deletedPaths == nil {
		a.deletedPaths = make(map[string]struct{})
	}
	key := strings.ToLower(filepath.Clean(path))
	if _, exists := a.deletedPaths[key]; exists {
		return
	}
	a.deletedPaths[key] = struct{}{}
	if a.deletedPathFile == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(a.deletedPathFile), 0o755)
	if f, err := os.OpenFile(a.deletedPathFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		_, _ = io.WriteString(f, filepath.Clean(path)+"\n")
		_ = f.Close()
	}
}

func (a *WindowsApp) isSessionDeleted(path string) bool {
	if a == nil || len(a.deletedPaths) == 0 || path == "" {
		return false
	}
	_, ok := a.deletedPaths[strings.ToLower(filepath.Clean(path))]
	return ok
}

func (a *WindowsApp) refreshAfterDelete(preferredRow int) {
	if a == nil {
		return
	}
	if len(a.steps) > 0 {
		step := a.steps[len(a.steps)-1]
		a.showIDs(step.IDs, a.sessionSnap, "")
		a.restoreListSelectionAfterDelete(preferredRow)
		return
	}
	if a.lastSnap != nil && len(a.lastResult.IDs) > 0 {
		a.showIDs(a.lastResult.IDs, a.lastSnap, "")
		a.restoreListSelectionAfterDelete(preferredRow)
		return
	}
	a.clearList()
	procSetFocus.Call(a.list)
}

func (a *WindowsApp) selectedRowIndex() int {
	if a == nil || a.list == 0 {
		return -1
	}
	return int(int32(sendMessage(a.list, lvmGetNextItem, ^uintptr(0), lvniSelected)))
}

func (a *WindowsApp) restoreListSelectionAfterDelete(preferredRow int) {
	if a == nil || a.list == 0 {
		return
	}
	row := nextSelectionAfterDelete(preferredRow, len(a.shownIDs))
	if row < 0 {
		procSetFocus.Call(a.list)
		return
	}

	state := lvItem{
		State:     lvisSelected | lvisFocused,
		StateMask: lvisSelected | lvisFocused,
	}
	sendMessage(a.list, lvmSetItemState, uintptr(row), uintptr(unsafe.Pointer(&state)))
	sendMessage(a.list, lvmEnsureVisible, uintptr(row), 0)
	procSetFocus.Call(a.list)
	// SetItemState normally emits LVN_ITEMCHANGED, but schedule explicitly too
	// so the preview always advances to the replacement row after deletion.
	a.schedulePreview()
}

func moveFileToRecycleBin(owner uintptr, path string) error {
	clean, err := filepath.Abs(path)
	if err != nil {
		clean = filepath.Clean(path)
	}
	from, err := syscall.UTF16FromString(clean)
	if err != nil {
		return err
	}
	// SHFileOperation requires a double-NUL-terminated list (PCZZWSTR).
	from = append(from, 0)
	op := shFileOpStruct{
		Hwnd:   owner,
		WFunc:  foDelete,
		PFrom:  &from[0],
		FFlags: fofAllowUndo | fofNoConfirmation | fofSilent | fofNoErrorUI,
	}
	r, _, _ := procSHFileOperationW.Call(uintptr(unsafe.Pointer(&op)))
	if r != 0 {
		return fmt.Errorf("SHFileOperation error %d", r)
	}
	if op.FAnyOperationsAborted != 0 {
		return fmt.Errorf("operation was canceled")
	}
	return nil
}

func (a *WindowsApp) openSelected() {
	if e, ok := a.selectedEntry(); ok {
		a.openPathWithAssociatedApp(e.Path)
	}
}

// handlePaneArrowKeyMessage makes Left/Right act as pane-navigation keys.
// Right moves keyboard focus from the result list into Preview. Left moves it
// back to the result list. Search-box Left/Right remain normal caret keys, and
// Up/Down keep their existing file-navigation/preview scrolling behavior.
func (a *WindowsApp) handlePaneArrowKeyMessage(m *msg) bool {
	if a == nil || m == nil || m.Message != wmKeyDown {
		return false
	}
	if m.WParam != vkLeft && m.WParam != vkRight {
		return false
	}

	inList := m.Hwnd == a.list
	inPreview := a.isPreviewWindow(m.Hwnd)
	target := paneSwitchTarget(int(m.WParam), inList, inPreview, a.previewPaneAvailable())
	switch target {
	case paneTargetPreview:
		a.focusPreviewPane()
		return true
	case paneTargetList:
		// Preview handlers (Office/PDF/HTML) may run on the dedicated STA preview
		// thread. Post the focus request back to the main UI thread instead of
		// calling SetFocus across input queues.
		postMessage(a.hwnd, wmAppFocusList, 0, 0)
		return true
	default:
		return false
	}
}

func handlePaneArrowKeyMessage(m *msg) bool {
	if winApp == nil || m == nil {
		return false
	}
	return winApp.handlePaneArrowKeyMessage(m)
}

func (a *WindowsApp) isPreviewWindow(hwnd uintptr) bool {
	if a == nil || hwnd == 0 || a.previewHost == 0 {
		return false
	}
	if hwnd == a.previewHost || hwnd == a.previewImage || hwnd == a.previewText {
		return true
	}
	r, _, _ := procIsChild.Call(a.previewHost, hwnd)
	return r != 0
}

func (a *WindowsApp) previewPaneAvailable() bool {
	if a == nil || !a.previewEnabled || a.previewHost == 0 {
		return false
	}
	visible, _, _ := procIsWindowVisible.Call(a.previewHost)
	return visible != 0
}

func (a *WindowsApp) focusPreviewPane() {
	if !a.previewPaneAvailable() {
		return
	}
	// Give the pane a deterministic focus target immediately. Rich Windows
	// preview handlers are then asked to focus their own child viewport so
	// Office/PDF scrolling still works with Up/Down/PageUp/PageDown.
	procSetFocus.Call(a.previewHost)
	if a.previewMgr != nil {
		a.previewMgr.Focus()
	}
}

func (a *WindowsApp) focusFileListPane() {
	if a == nil || a.list == 0 {
		return
	}
	row := a.selectedRowIndex()
	if row < 0 && len(a.shownIDs) > 0 {
		row = 0
	}
	if row >= 0 {
		state := lvItem{State: lvisSelected | lvisFocused, StateMask: lvisSelected | lvisFocused}
		sendMessage(a.list, lvmSetItemState, uintptr(row), uintptr(unsafe.Pointer(&state)))
		sendMessage(a.list, lvmEnsureVisible, uintptr(row), 0)
	}
	procSetFocus.Call(a.list)
}

func (a *WindowsApp) handleSearchArrowKeyMessage(m *msg) bool {
	if a == nil || m == nil || m.Message != wmKeyDown || m.Hwnd != a.searchEdit {
		return false
	}
	if m.WParam != vkUp && m.WParam != vkDown {
		return false
	}

	dir := 1
	if m.WParam == vkUp {
		dir = -1
	}
	raw := strings.TrimSpace(getWindowText(a.searchEdit))
	if raw == "" {
		return false
	}

	// If the visible result already belongs to the exact text in the search box,
	// navigation can start immediately without waiting for another search.
	if strings.EqualFold(strings.TrimSpace(a.lastResult.Query), raw) && len(a.shownIDs) > 0 {
		a.focusResultListFromSearch(dir)
		return true
	}

	// The user may press Down immediately after typing the last character, before
	// the normal 110 ms debounce fires. Remember the requested direction and start
	// this exact query now. consumeResult() will move focus when the result arrives.
	a.pendingListNav = dir
	a.pendingListNavQuery = raw
	a.searchSeq.Add(1) // invalidate a delayed search that may already be sleeping
	postMessage(a.hwnd, wmAppSearch, 0, 0)
	return true
}

func (a *WindowsApp) focusResultListFromSearch(direction int) bool {
	if a == nil || a.list == 0 || len(a.shownIDs) == 0 {
		return false
	}
	current := a.selectedRowIndex()
	target := listRowFromSearchArrow(direction, current, len(a.shownIDs))
	if target < 0 {
		return false
	}

	// Clear stale ListView selection/focus, then place a single caret on target.
	clearState := lvItem{State: 0, StateMask: lvisSelected | lvisFocused}
	sendMessage(a.list, lvmSetItemState, ^uintptr(0), uintptr(unsafe.Pointer(&clearState)))
	state := lvItem{State: lvisSelected | lvisFocused, StateMask: lvisSelected | lvisFocused}
	sendMessage(a.list, lvmSetItemState, uintptr(target), uintptr(unsafe.Pointer(&state)))
	sendMessage(a.list, lvmEnsureVisible, uintptr(target), 0)
	procSetFocus.Call(a.list)
	a.schedulePreview()
	return true
}

func (a *WindowsApp) handleDeleteKeyMessage(m *msg) bool {
	if a == nil || m == nil || m.Message != wmKeyDown || m.WParam != vkDelete {
		return false
	}
	// Delete is a file action only when focus is in the result list or preview
	// area. This intentionally does not intercept Delete while editing the
	// search box or other controls.
	inList := m.Hwnd == a.list
	inPreview := m.Hwnd == a.previewHost || m.Hwnd == a.previewImage
	if !inPreview && a.previewHost != 0 && m.Hwnd != 0 {
		if r, _, _ := procIsChild.Call(a.previewHost, m.Hwnd); r != 0 {
			inPreview = true
		}
	}
	if !inList && !inPreview {
		return false
	}
	a.deleteSelectedImage()
	return true
}

func handlePreviewInputMessage(m *msg) bool {
	if winApp == nil || m == nil {
		return false
	}
	return winApp.handlePreviewInputMessage(m)
}

func (a *WindowsApp) handlePreviewInputMessage(m *msg) bool {
	if a == nil || m == nil || !a.previewEnabled || a.previewHost == 0 {
		return false
	}

	// Once an image drag has captured the mouse, keep processing motion/up even
	// when the cursor leaves the preview pane. This makes large-image panning feel
	// continuous instead of abruptly stopping at the pane edge.
	if a.previewPanning {
		switch m.Message {
		case wmMouseMove:
			dx := m.Pt.X - a.previewPanLast.X
			dy := m.Pt.Y - a.previewPanLast.Y
			if dx != 0 || dy != 0 {
				a.previewPanLast = m.Pt
				if !a.previewPanMoved && (absI32(m.Pt.X-a.previewPanStart.X) >= 3 || absI32(m.Pt.Y-a.previewPanStart.Y) >= 3) {
					a.previewPanMoved = true
					a.resetPreviewClickTracker()
				}
				if a.previewMgr != nil {
					a.previewMgr.Pan(dx, dy)
				}
			}
			return true
		case wmLButtonUp:
			wasClick := !a.previewPanMoved
			a.previewPanning = false
			procReleaseCapture.Call()
			if wasClick {
				// A normal Preview click should make the matching result obvious.
				// This keeps the same selected row, scrolls it into view, gives it
				// the focused highlight, and makes Up/Down work immediately.
				a.focusFileListPane()
			}
			return true
		}
	}

	if !pointInWindow(a.previewHost, m.Pt) {
		return false
	}

	switch m.Message {
	case wmMouseWheel:
		// Plain mouse wheel over an image preview controls zoom. The point under
		// the cursor is kept stationary so zoom feels like a photo viewer rather
		// than always expanding from the center. PDF/Office/text keep native wheel
		// scrolling behavior.
		if a.previewMgr != nil && a.previewMgr.ImageActive() {
			delta := int(int16(hiword(m.WParam)))
			if delta != 0 {
				pt := m.Pt
				procScreenToClient.Call(a.previewHost, uintptr(unsafe.Pointer(&pt)))
				a.previewMgr.ZoomAt(delta, pt.X, pt.Y)
				return true
			}
		}
	case wmLButtonDblClk:
		a.resetPreviewClickTracker()
		a.openCurrentPreview()
		return true
	case wmLButtonDown:
		// Some child controls do not emit WM_LBUTTONDBLCLK. Detect a second
		// normal click using the Windows double-click timing as a fallback.
		if a.previewSecondClick(m.Time, m.Pt) {
			a.openCurrentPreview()
			return true
		}
		if a.previewMgr != nil && a.previewMgr.ImageActive() {
			// Drag anywhere on the visible image to pan. Capture the mouse so the
			// drag remains smooth even if the cursor briefly leaves the image.
			a.previewPanning = true
			a.previewPanMoved = false
			a.previewPanStart = m.Pt
			a.previewPanLast = m.Pt
			procSetCapture.Call(a.previewImage)
			return true
		}
	}
	return false
}

func (a *WindowsApp) previewSecondClick(now uint32, pt point) bool {
	a.previewClickMu.Lock()
	defer a.previewClickMu.Unlock()
	limit, _, _ := procGetDoubleClickTime.Call()
	if limit == 0 {
		limit = 500
	}
	isSecond := a.previewClickTime != 0 && now-a.previewClickTime <= uint32(limit) &&
		absI32(pt.X-a.previewClickPt.X) <= 6 && absI32(pt.Y-a.previewClickPt.Y) <= 6
	if isSecond {
		a.previewClickTime = 0
		return true
	}
	a.previewClickTime = now
	a.previewClickPt = pt
	return false
}

func (a *WindowsApp) resetPreviewClickTracker() {
	a.previewClickMu.Lock()
	a.previewClickTime = 0
	a.previewClickMu.Unlock()
}

func absI32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

func (a *WindowsApp) currentPreviewPath() string {
	if a == nil {
		return ""
	}
	v := a.previewPath.Load()
	if v == nil {
		return ""
	}
	path, _ := v.(string)
	return path
}

func (a *WindowsApp) openCurrentPreview() {
	path := a.currentPreviewPath()
	if path == "" {
		return
	}
	a.openPathWithAssociatedApp(path)
}

func (a *WindowsApp) openPathWithAssociatedApp(path string) {
	if path == "" {
		return
	}
	result := shellOpen(a.hwnd, path)
	if result > 32 {
		return
	}
	// ShellExecute error 31 is SE_ERR_NOASSOC: no default application is
	// associated with this file type. In that case provide practical choices
	// and offer Windows' own "Open with" selector instead of silently failing.
	if result == 31 {
		recommend := recommendedAppsForPath(path)
		text := "이 파일 형식에 연결된 기본 프로그램이 없습니다.\r\n\r\n" +
			"추천 프로그램: " + recommend + "\r\n\r\n" +
			"Windows의 '연결 프로그램(Open with)' 선택 창을 열까요?"
		if askYesNo(a.hwnd, "연결 프로그램 추천", text) {
			if shellOpenWith(a.hwnd, path) <= 32 {
				showInfo("Windows 연결 프로그램 선택 창을 열 수 없습니다.")
			}
		}
		return
	}
	showInfo(fmt.Sprintf("파일을 열 수 없습니다.\n\n%s\n\nWindows 오류 코드: %d", path, result))
}

func recommendedAppsForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".doc", ".docx":
		return "Microsoft Word / LibreOffice Writer"
	case ".xls", ".xlsx", ".xlsm", ".xlsb", ".csv":
		return "Microsoft Excel / LibreOffice Calc"
	case ".ppt", ".pptx", ".pptm":
		return "Microsoft PowerPoint / LibreOffice Impress"
	case ".pdf":
		return "Microsoft Edge / Adobe Acrobat Reader"
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp", ".tif", ".tiff":
		return "Windows Photos / Paint / IrfanView"
	case ".html", ".htm":
		return "Microsoft Edge / Chrome / Firefox"
	case ".txt", ".md", ".markdown", ".json", ".xml", ".yaml", ".yml", ".log", ".ini", ".toml":
		return "Notepad / Notepad++ / Visual Studio Code"
	default:
		return "이 파일 형식을 지원하는 앱"
	}
}

func askYesNo(owner uintptr, title, text string) bool {
	pText := utf16Ptr(text)
	pTitle := utf16Ptr(title)

	// Use YES/NO/CANCEL instead of YES/NO so Windows gives the dialog a
	// standard cancel path. This makes Esc cancel the operation immediately.
	// Both No and Cancel (including Esc) are treated as a safe cancellation;
	// only an explicit Yes proceeds.
	r, _, _ := procMessageBoxW.Call(owner, uintptr(unsafe.Pointer(pText)), uintptr(unsafe.Pointer(pTitle)), mbYesNoCancel|mbIconQuestion)
	return r == idYes
}

func (a *WindowsApp) copySelectedPath() {
	if e, ok := a.selectedEntry(); ok {
		if copyTextToClipboard(a.hwnd, e.Path) {
			a.setStatus("Path copied: " + e.Path)
		}
	}
}

func (a *WindowsApp) openIndexFolder() {
	dir := IndexFolderPath()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		a.setStatus("Cannot open index folder: " + err.Error())
		return
	}
	shellOpen(a.hwnd, dir)
	mode := "portable"
	if !UsingPortableStorage() {
		mode = "LocalAppData fallback"
	}
	a.setStatus(fmt.Sprintf("Index folder (%s): %s", mode, dir))
}

func (a *WindowsApp) setStatus(s string) { setWindowText(a.status, s) }
func (a *WindowsApp) postStatus(s string) {
	select {
	case a.statusCh <- s:
	default:
		select {
		case <-a.statusCh:
		default:
		}
		a.statusCh <- s
	}
	if a.hwnd != 0 {
		postMessage(a.hwnd, wmAppStatus, 0, 0)
	}
}
func (a *WindowsApp) consumeStatus() {
	select {
	case s := <-a.statusCh:
		a.setStatus(s)
	default:
	}
}

func (a *WindowsApp) startBackgroundLoad() {
	if _, err := os.Stat(a.indexPath); err == nil {
		a.loadIndexAsync(a.indexPath, "saved portable index mapped", true)
		return
	}
	// v0.1.4 and earlier stored the index in %LOCALAPPDATA%. Reuse it
	// immediately so the user does not have to reindex, while copying it to
	// the new transparent portable Index folder in the background.
	legacy := legacyIndexPath()
	if legacy != "" && !strings.EqualFold(filepath.Clean(legacy), filepath.Clean(a.indexPath)) {
		if _, err := os.Stat(legacy); err == nil {
			a.loadIndexAsync(legacy, "legacy index mapped", false)
			go a.migrateLegacyIndex(legacy)
			return
		}
	}
	// No saved index yet: start the low-priority worker process immediately.
	a.startReindex(false)
}

func (a *WindowsApp) loadIndexAsync(path, reason string, rebuildOnError bool) bool {
	if !a.loading.CompareAndSwap(false, true) {
		return false
	}
	go func() {
		defer a.loading.Store(false)
		snap, err := LoadIndex(path, func(done, total uint64) {
			if total > 0 && (done&0x3ffff) == 0 {
				a.postStatus(fmt.Sprintf("Loading saved index in background... %d%%", done*100/total))
			}
		})
		if err != nil {
			logf("index load failed: %v", err)
			a.postStatus("Old/invalid index detected. Rebuilding safely in background...")
			if rebuildOnError {
				legacy := path + ".legacy"
				_ = os.Remove(legacy)
				_ = os.Rename(path, legacy)
				a.startReindex(false)
			}
			return
		}
		a.postSnapshot(snap, reason)
	}()
	return true
}

func (a *WindowsApp) migrateLegacyIndex(source string) {
	if a.indexing.Load() {
		return
	}
	if _, err := os.Stat(a.indexPath); err == nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(a.indexPath), 0o755); err != nil {
		return
	}
	tmp := a.indexPath + ".migrate.tmp"
	_ = os.Remove(tmp)
	in, err := os.Open(source)
	if err != nil {
		return
	}
	defer in.Close()
	out, err := os.Create(tmp)
	if err != nil {
		return
	}
	buf := make([]byte, 4<<20)
	ok := false
	defer func() {
		_ = out.Close()
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	for {
		n, rerr := in.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return
			}
		}
		if rerr != nil {
			if rerr == io.EOF {
				break
			}
			return
		}
		if searchBusy.Load() {
			time.Sleep(8 * time.Millisecond)
		} else {
			time.Sleep(2 * time.Millisecond)
		}
	}
	if err := out.Sync(); err != nil {
		return
	}
	if err := out.Close(); err != nil {
		return
	}
	if a.indexing.Load() {
		return
	}
	if _, err := os.Stat(a.indexPath); err == nil {
		return
	}
	if err := os.Rename(tmp, a.indexPath); err != nil {
		return
	}
	ok = true
	a.postStatus("Existing index copied to the portable Index folder. Click Index Folder to view or back it up.")
}

func (a *WindowsApp) roots() []string {
	if !a.cfg.AutoRoots && len(a.cfg.Roots) > 0 {
		return append([]string(nil), a.cfg.Roots...)
	}
	return DetectDefaultRoots()
}

func (a *WindowsApp) startReindex(manual bool) {
	if !a.indexing.CompareAndSwap(false, true) {
		a.postStatus("Indexing is already running in background.")
		return
	}

	exe, err := os.Executable()
	if err != nil {
		a.indexing.Store(false)
		a.postStatus("Cannot start indexer: " + err.Error())
		return
	}
	workerExe := filepath.Join(filepath.Dir(exe), "xFile_indexer.exe")
	if _, err := os.Stat(workerExe); err != nil {
		workerExe = exe // portable fallback
	}

	roots := a.roots()
	preferredVolumes := changedDriveVolumes(DriveStatePath(), roots)
	partialPath := filepath.Join(IndexFolderPath(), fmt.Sprintf("xFile_v3.partial.%d.index", time.Now().UnixNano()))
	cleanupStalePartialIndexes(IndexFolderPath())

	_ = os.Remove(IndexerProgressPath())
	cmd := exec.Command(workerExe, "--indexer")
	cmd.Env = append(os.Environ(), "XFILE_PARTIAL_INDEX="+partialPath)
	if len(preferredVolumes) > 0 {
		cmd.Env = append(cmd.Env, "XFILE_PRIORITY_VOLUMES="+strings.Join(preferredVolumes, ";"))
	}
	if err := cmd.Start(); err != nil {
		a.indexing.Store(false)
		logf("indexer start failed: %v", err)
		a.postStatus("Cannot start background indexer: " + err.Error())
		return
	}
	a.workerMu.Lock()
	a.workerCmd = cmd
	a.workerMu.Unlock()
	postMessage(a.hwnd, wmAppIndexingState, 1, 0)
	if manual {
		a.postStatus("Reindex started in a separate background process...")
	} else {
		a.postStatus("Building first index in a separate background process...")
	}

	go a.watchIndexer(cmd, partialPath)
	go func() {
		err := cmd.Wait()
		a.workerMu.Lock()
		if a.workerCmd == cmd {
			a.workerCmd = nil
		}
		a.workerMu.Unlock()
		a.indexing.Store(false)
		postMessage(a.hwnd, wmAppIndexingState, 0, 0)
		if err != nil {
			logf("indexer exited: %v", err)
			a.postStatus("Background indexing stopped. Click Reindex to try again.")
			return
		}
		a.postStatus("Index built. Loading full index in background...")
		for {
			if a.loadIndexAsync(a.indexPath, "background index mapped", false) {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()
}

func (a *WindowsApp) watchIndexer(cmd *exec.Cmd, partialPath string) {
	progressPath := IndexerProgressPath()
	last := ""
	partialLoaded := false
	t := time.NewTicker(600 * time.Millisecond)
	defer t.Stop()
	for range t.C {
		a.workerMu.Lock()
		active := a.workerCmd == cmd
		a.workerMu.Unlock()
		if !active {
			return
		}
		if !partialLoaded && partialPath != "" && !a.loading.Load() {
			if _, err := os.Stat(partialPath); err == nil {
				if a.loadIndexAsync(partialPath, "partial index · background indexing continues", false) {
					partialLoaded = true
				}
			}
		}
		b, err := os.ReadFile(progressPath)
		if err == nil {
			msg := strings.TrimSpace(string(b))
			if msg != "" && msg != last {
				last = msg
				a.postStatus(msg)
			}
		}
	}
}

func (a *WindowsApp) stopIndexer() {
	a.workerMu.Lock()
	cmd := a.workerCmd
	a.workerCmd = nil
	a.workerMu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func (a *WindowsApp) postSnapshot(s *IndexSnapshot, reason string) {
	p := snapshotPayload{Snap: s, Reason: reason}
	select {
	case a.snapshotCh <- p:
	default:
		select {
		case <-a.snapshotCh:
		default:
		}
		a.snapshotCh <- p
	}
	if a.hwnd != 0 {
		postMessage(a.hwnd, wmAppSnapshot, 0, 0)
	}
}

func (a *WindowsApp) consumeSnapshot() {
	var p snapshotPayload
	select {
	case p = <-a.snapshotCh:
	default:
		return
	}
	a.snapshot.Store(p.Snap)
	if len(a.steps) > 0 && a.sessionSnap != p.Snap {
		a.setStatus(fmt.Sprintf("New index ready · %s items · current Search Within session keeps the previous snapshot until Clear", formatCount(p.Snap.Len())))
		return
	}
	if strings.Contains(strings.ToLower(p.Reason), "partial index") && a.indexing.Load() {
		a.setStatus(fmt.Sprintf("Partial index ready · %s items searchable · full indexing continues in background...", formatCount(p.Snap.Len())))
		if strings.TrimSpace(getWindowText(a.searchEdit)) != "" {
			a.scheduleSearch()
		}
		return
	}
	a.setStatus(fmt.Sprintf("Ready · %s items indexed · %s", formatCount(p.Snap.Len()), p.Reason))
	if needs, why := a.driveIndexNeedsRefresh(); needs && !a.indexing.Load() {
		a.setStatus("Drive/index change detected · refreshing safely in background · " + why)
		a.startReindex(false)
		return
	}
	if strings.TrimSpace(getWindowText(a.searchEdit)) != "" {
		a.scheduleSearch()
	}
}

func cleanupStalePartialIndexes(dir string) {
	if dir == "" {
		return
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "xFile_v3.partial.*.index"))
	for _, path := range matches {
		_ = os.Remove(path)
	}
}

func (a *WindowsApp) scheduleDriveRefreshCheck() {
	if a == nil || a.hwnd == 0 {
		return
	}
	seq := a.driveChangeSeq.Add(1)
	a.postStatus("Drive change detected · checking current volume...")
	go func() {
		time.Sleep(1400 * time.Millisecond)
		if a.driveChangeSeq.Load() == seq && a.hwnd != 0 {
			postMessage(a.hwnd, wmAppDriveCheck, 0, 0)
		}
	}()
}

func (a *WindowsApp) handleDriveRefreshCheck() {
	if a == nil || a.indexing.Load() {
		return
	}
	if needs, why := a.driveIndexNeedsRefresh(); needs {
		a.postStatus("Drive changed · rebuilding index in background · " + why)
		a.startReindex(false)
	}
}

func (a *WindowsApp) driveIndexNeedsRefresh() (bool, string) {
	if a == nil {
		return false, ""
	}
	match, known := driveStateMatches(DriveStatePath(), a.roots())
	if !known {
		return true, "creating volume fingerprint (one-time refresh)"
	}
	if !match {
		return true, "drive letter/volume changed"
	}
	return false, ""
}

func (a *WindowsApp) applyIndexingVisual(active bool) {
	if a == nil {
		return
	}
	if active {
		setWindowText(a.hwnd, appName+" "+appVersion+"  ·  INDEXING...")
		setWindowText(a.reindexBtn, "Indexing...")
		if a.indexProgress != 0 {
			sendMessage(a.indexProgress, pbmSetMarquee, 1, 28)
		}
	} else {
		setWindowText(a.hwnd, appName+" "+appVersion)
		setWindowText(a.reindexBtn, "Reindex")
		if a.indexProgress != 0 {
			sendMessage(a.indexProgress, pbmSetMarquee, 0, 0)
		}
	}
	a.layout()
}

func formatCount(n int) string {
	s := fmt.Sprintf("%d", n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func showFatal(text string) {
	pText := utf16Ptr(text)
	pTitle := utf16Ptr(appName)
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(pText)), uintptr(unsafe.Pointer(pTitle)), mbOK|mbIconError)
}

func showInfo(text string) {
	pText := utf16Ptr(text)
	pTitle := utf16Ptr(appName)
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(pText)), uintptr(unsafe.Pointer(pTitle)), mbOK|mbIconInfo)
}

func runtimeInfo() string {
	return fmt.Sprintf("%s %s · Go %s", appName, appVersion, runtime.Version())
}
