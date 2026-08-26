//go:build windows

package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf8"
	"unsafe"
)

// xFile_search uses the same Windows Shell preview-handler infrastructure used
// by Explorer's Preview pane. That gives rich previews for Office/PDF/image
// formats whenever a preview handler is installed. Text-like formats also have
// an internal fallback so TXT/CSV/JSON/MD/HTML remain useful even if Windows
// has no handler registered for them.

const (
	previewHandlerShellExt  = `{8895b1c6-b41f-4c1c-a562-0d564250836f}`
	coinitApartmentThreaded = 0x2
	clsctxInprocServer      = 0x1
	clsctxLocalServer       = 0x4
	stgmRead                = 0x00000000
	stgmShareDenyNone       = 0x00000040
	fileAttributeNormal     = 0x00000080
	maxTextPreviewBytes     = 2 << 20 // keep large CSV/log files responsive
)

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	ole32   = syscall.NewLazyDLL("ole32.dll")
	shlwapi = syscall.NewLazyDLL("shlwapi.dll")
	atl     = syscall.NewLazyDLL("atl.dll")

	procCoInitializeEx              = ole32.NewProc("CoInitializeEx")
	procCoUninitialize              = ole32.NewProc("CoUninitialize")
	procCoCreateInstance            = ole32.NewProc("CoCreateInstance")
	procCLSIDFromString             = ole32.NewProc("CLSIDFromString")
	procSHCreateItemFromParsingName = shell32.NewProc("SHCreateItemFromParsingName")
	procSHCreateStreamOnFileEx      = shlwapi.NewProc("SHCreateStreamOnFileEx")
	procMultiByteToWideChar         = kernel32.NewProc("MultiByteToWideChar")
	procAtlAxWinInit                = atl.NewProc("AtlAxWinInit")
)

var (
	iidIPreviewHandler        = guid{0x8895b1c6, 0xb41f, 0x4c1c, [8]byte{0xa5, 0x62, 0x0d, 0x56, 0x42, 0x50, 0x83, 0x6f}}
	iidIInitializeWithFile    = guid{0xb7d14566, 0x0509, 0x4cce, [8]byte{0xa7, 0x1f, 0x0a, 0x55, 0x42, 0x33, 0xbd, 0x9b}}
	iidIInitializeWithStream  = guid{0xb824b49d, 0x22ac, 0x4161, [8]byte{0xac, 0x8a, 0x99, 0x16, 0xe8, 0xfa, 0x3f, 0x7f}}
	iidIInitializeWithItem    = guid{0x7f73be3f, 0xfb79, 0x493c, [8]byte{0xa6, 0xc7, 0x7e, 0xe1, 0x4e, 0x24, 0x58, 0x41}}
	iidIShellItem             = guid{0x43826d1e, 0xe718, 0x42ee, [8]byte{0xbc, 0x55, 0xa1, 0xe2, 0x61, 0xc3, 0x7b, 0xfe}}
	iidIShellItemImageFactory = guid{0xbcc18b79, 0xba16, 0x442f, [8]byte{0x80, 0xc4, 0x8a, 0x59, 0xc3, 0x0c, 0x46, 0x3b}}
)

type imageZoomRequest struct {
	delta int
	x     int32
	y     int32
}

type imagePanRequest struct {
	dx int32
	dy int32
}

type imageViewMode int

const (
	imageViewFit imageViewMode = iota
	imageViewOneToOne
	imageViewManual
)

type imageViewCommand int

const (
	imageCommandFit imageViewCommand = iota
	imageCommandOneToOne
)

type imagePreviewState struct {
	path      string
	originalW int32
	originalH int32
	zoom      float64
	bitmapW   int32
	bitmapH   int32
	panX      int32
	panY      int32
	mode      imageViewMode
}

type previewManager struct {
	host        uintptr
	textEdit    uintptr
	imageCtl    uintptr
	htmlCtl     uintptr
	showCh      chan string
	resizeCh    chan rect
	zoomAtCh    chan imageZoomRequest
	panCh       chan imagePanRequest
	imageCmdCh  chan imageViewCommand
	focusCh     chan struct{}
	stopCh      chan struct{}
	imageActive atomic.Bool
}

type previewSession struct {
	handler uintptr
	stream  uintptr
	item    uintptr
}

func newPreviewManager(host, textEdit, imageCtl uintptr) *previewManager {
	p := &previewManager{
		host: host, textEdit: textEdit, imageCtl: imageCtl,
		showCh: make(chan string, 1), resizeCh: make(chan rect, 1), zoomAtCh: make(chan imageZoomRequest, 2), panCh: make(chan imagePanRequest, 8), imageCmdCh: make(chan imageViewCommand, 2), focusCh: make(chan struct{}, 1), stopCh: make(chan struct{}, 1),
	}
	go p.loop()
	return p
}

func (p *previewManager) Show(path string) {
	if p == nil {
		return
	}
	select {
	case p.showCh <- path:
	default:
		select {
		case <-p.showCh:
		default:
		}
		p.showCh <- path
	}
}

func (p *previewManager) Clear() { p.Show("") }

func (p *previewManager) ImageActive() bool {
	return p != nil && p.imageActive.Load()
}

// ZoomAt changes only the built-in image preview and keeps the image point
// under (x,y) fixed beneath the mouse cursor. PDF/Office preview handlers keep
// their own native wheel behavior.
func (p *previewManager) ZoomAt(wheelDelta int, x, y int32) {
	if p == nil || wheelDelta == 0 || !p.imageActive.Load() {
		return
	}
	req := imageZoomRequest{delta: wheelDelta, x: x, y: y}
	select {
	case p.zoomAtCh <- req:
	default:
		select {
		case <-p.zoomAtCh:
		default:
		}
		p.zoomAtCh <- req
	}
}

// Pan moves a zoomed image by screen-pixel deltas. The preview thread clamps
// the final position so the image cannot be dragged completely out of view.
func (p *previewManager) Pan(dx, dy int32) {
	if p == nil || !p.imageActive.Load() || (dx == 0 && dy == 0) {
		return
	}
	select {
	case p.panCh <- imagePanRequest{dx: dx, dy: dy}:
	default:
	}
}

func (p *previewManager) FitWindow() {
	if p == nil || !p.imageActive.Load() {
		return
	}
	select {
	case p.imageCmdCh <- imageCommandFit:
	default:
	}
}

func (p *previewManager) OneToOne() {
	if p == nil || !p.imageActive.Load() {
		return
	}
	select {
	case p.imageCmdCh <- imageCommandOneToOne:
	default:
	}
}

// Focus asks the active rich preview to focus its own viewport. The COM call
// must run on the STA thread that created the preview handler, so it is queued
// instead of being invoked directly by the main UI thread.
func (p *previewManager) Focus() {
	if p == nil {
		return
	}
	select {
	case p.focusCh <- struct{}{}:
	default:
	}
}

func (p *previewManager) Resize(w, h int32) {
	if p == nil || w <= 0 || h <= 0 {
		return
	}
	r := rect{Left: 0, Top: 0, Right: w, Bottom: h}
	select {
	case p.resizeCh <- r:
	default:
		select {
		case <-p.resizeCh:
		default:
		}
		p.resizeCh <- r
	}
}

func (p *previewManager) Close() {
	if p == nil {
		return
	}
	select {
	case p.stopCh <- struct{}{}:
	default:
	}
}

func (p *previewManager) loop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hr, _, _ := procCoInitializeEx.Call(0, coinitApartmentThreaded)
	comReady := !hresultFailed(hr) || uint32(hr) == 0x80010106 // RPC_E_CHANGED_MODE: COM is already initialized
	if !hresultFailed(hr) {
		defer procCoUninitialize.Call()
	}

	currentRect := rect{Left: 0, Top: 0, Right: 500, Bottom: 500}
	var sess previewSession
	var bitmap uintptr
	var imageState imagePreviewState
	defer p.unload(&sess)
	defer p.clearBitmap(&bitmap)
	defer p.destroyHTMLControl()

	for {
		select {
		case <-p.stopCh:
			return
		default:
		}

		didWork := false
		select {
		case r := <-p.resizeCh:
			didWork = true
			currentRect = r
			newW := maxI32(1, r.Right-r.Left)
			newH := maxI32(1, r.Bottom-r.Top)
			setPos(p.textEdit, 0, 0, newW, newH)
			if p.htmlCtl != 0 {
				setPos(p.htmlCtl, 0, 0, newW, newH)
			}
			if sess.handler != 0 {
				comCall(sess.handler, 4, uintptr(unsafe.Pointer(&currentRect))) // IPreviewHandler::SetRect
				// Some Office preview handlers keep a child viewport at the old size even
				// after SetRect. Stretch only the direct preview children so Excel/PPT/PDF
				// stay attached to the right pane and grow with the splitter/window.
				stretchPreviewChildren(p.host, newW, newH)
			}
			if imageState.path != "" && bitmap != 0 {
				if imageState.mode == imageViewFit {
					imageState.zoom = fitImageScale(newW, newH, imageState.originalW, imageState.originalH)
					if hbmp, bw, bh, effective, ok := loadShellImageScale(imageState.path, imageState.originalW, imageState.originalH, imageState.zoom); ok {
						p.clearBitmap(&bitmap)
						bitmap = hbmp
						imageState.zoom = effective
						imageState.bitmapW, imageState.bitmapH = bw, bh
						imageState.panX, imageState.panY = centeredImagePan(newW, newH, bw, bh)
						p.showBitmap(bitmap, imageState.panX, imageState.panY, bw, bh)
					}
				} else {
					imageState.panX, imageState.panY = clampImagePan(newW, newH, imageState.bitmapW, imageState.bitmapH, imageState.panX, imageState.panY)
					p.positionBitmap(imageState.panX, imageState.panY, imageState.bitmapW, imageState.bitmapH)
				}
			} else {
				setPos(p.imageCtl, 0, 0, newW, newH)
			}
		default:
		}

		select {
		case <-p.focusCh:
			didWork = true
			if sess.handler != 0 {
				// IPreviewHandler::SetFocus (vtable slot 7). This lets native
				// Office/PDF handlers receive their normal Up/Down/Page keys.
				comCall(sess.handler, 7)
			} else if p.htmlCtl != 0 {
				procSetFocus.Call(p.htmlCtl)
			}
		default:
		}

		select {
		case req := <-p.zoomAtCh:
			didWork = true
			if imageState.path != "" && bitmap != 0 {
				newZoom := imageState.zoom
				if req.delta > 0 {
					newZoom *= 1.25
				} else {
					newZoom *= 0.8
				}
				if newZoom < imageZoomMin {
					newZoom = imageZoomMin
				}
				if newZoom > imageZoomMax {
					newZoom = imageZoomMax
				}
				if hbmp, bw, bh, effective, ok := loadShellImageScale(imageState.path, imageState.originalW, imageState.originalH, newZoom); ok {
					viewW := maxI32(1, currentRect.Right-currentRect.Left)
					viewH := maxI32(1, currentRect.Bottom-currentRect.Top)
					nx, ny := zoomAnchorPan(viewW, viewH, imageState.bitmapW, imageState.bitmapH, bw, bh, imageState.panX, imageState.panY, req.x, req.y)
					p.clearBitmap(&bitmap)
					bitmap = hbmp
					imageState.zoom = effective
					imageState.bitmapW, imageState.bitmapH = bw, bh
					imageState.panX, imageState.panY = nx, ny
					imageState.mode = imageViewManual
					p.showBitmap(bitmap, nx, ny, bw, bh)
				}
			}
		default:
		}

		select {
		case req := <-p.panCh:
			didWork = true
			if imageState.path != "" && bitmap != 0 {
				viewW := maxI32(1, currentRect.Right-currentRect.Left)
				viewH := maxI32(1, currentRect.Bottom-currentRect.Top)
				imageState.panX, imageState.panY = clampImagePan(viewW, viewH, imageState.bitmapW, imageState.bitmapH, imageState.panX+req.dx, imageState.panY+req.dy)
				p.positionBitmap(imageState.panX, imageState.panY, imageState.bitmapW, imageState.bitmapH)
			}
		default:
		}

		select {
		case cmd := <-p.imageCmdCh:
			didWork = true
			if imageState.path != "" && bitmap != 0 {
				viewW := maxI32(1, currentRect.Right-currentRect.Left)
				viewH := maxI32(1, currentRect.Bottom-currentRect.Top)
				targetZoom := imageState.zoom
				targetMode := imageState.mode
				switch cmd {
				case imageCommandFit:
					targetZoom = fitImageScale(viewW, viewH, imageState.originalW, imageState.originalH)
					targetMode = imageViewFit
				case imageCommandOneToOne:
					targetZoom = 1.0
					targetMode = imageViewOneToOne
				}
				if hbmp, bw, bh, effective, ok := loadShellImageScale(imageState.path, imageState.originalW, imageState.originalH, targetZoom); ok {
					p.clearBitmap(&bitmap)
					bitmap = hbmp
					imageState.zoom = effective
					imageState.bitmapW, imageState.bitmapH = bw, bh
					imageState.panX, imageState.panY = centeredImagePan(viewW, viewH, bw, bh)
					imageState.mode = targetMode
					p.showBitmap(bitmap, imageState.panX, imageState.panY, bw, bh)
				}
			}
		default:
		}

		select {
		case path := <-p.showCh:
			didWork = true
			p.unload(&sess)
			p.clearBitmap(&bitmap)
			p.clearHost(currentRect)
			imageState = imagePreviewState{}
			p.imageActive.Store(false)
			if path == "" {
				p.showFallback("Select a file to preview.", currentRect)
				continue
			}
			info, err := os.Stat(path)
			if err != nil {
				p.showFallback("Preview unavailable:\r\n"+err.Error(), currentRect)
				continue
			}
			if info.IsDir() {
				p.showFallback("Folder preview is not available.\r\n\r\n"+path, currentRect)
				continue
			}

			// Images use the built-in bitmap preview first so xFile_search can
			// provide deterministic mouse-wheel zoom independent of third-party
			// Shell preview handlers.
			if isImagePreviewPath(path) {
				viewW := maxI32(1, currentRect.Right-currentRect.Left)
				viewH := maxI32(1, currentRect.Bottom-currentRect.Top)
				originalW, originalH, sizeOK := imagePixelSize(path)
				if !sizeOK {
					// Unknown image formats can still use the Shell image factory. Use
					// the current viewport as a safe pseudo-native size; JPG/PNG/GIF/
					// BMP/WEBP/TIFF normally resolve their real pixel dimensions above.
					originalW = maxI32(64, viewW-8)
					originalH = maxI32(64, viewH-8)
				}
				zoom := fitImageScale(viewW, viewH, originalW, originalH)
				if hbmp, bw, bh, effective, ok := loadShellImageScale(path, originalW, originalH, zoom); ok {
					bitmap = hbmp
					px, py := centeredImagePan(viewW, viewH, bw, bh)
					imageState = imagePreviewState{path: path, originalW: originalW, originalH: originalH, zoom: effective, bitmapW: bw, bitmapH: bh, panX: px, panY: py, mode: imageViewFit}
					p.imageActive.Store(true)
					p.showBitmap(bitmap, px, py, bw, bh)
					continue
				}
			}

			// HTML/HTM is hosted in-process with Windows ATL ActiveX containment.
			// This avoids spawning hidden browser processes or invoking scripts.
			// AtlAxWin80 accepts a URL as the hosted control text and renders it
			// with the Windows WebBrowser/MSHTML control when available.
			if isHTMLPreviewPath(path) {
				if p.showHTMLInProcess(path, currentRect) {
					continue
				}
				p.showFallback("HTML preview is unavailable on this PC. Double-click to open it in your default browser.", currentRect)
				continue
			}

			// First try the native Windows preview handler. v0.1.11 retries the
			// initialization methods because old .xls/.ppt handlers are inconsistent:
			// some files only work through IInitializeWithFile while others prefer stream.
			if comReady {
				if clsid := findPreviewHandlerCLSID(path); clsid != "" {
					if s, ok := p.startShellPreview(path, clsid, currentRect); ok {
						sess = s
						continue
					}
				}
			}

			// Security-safe build: do not launch PowerShell/Office automation to
			// manufacture a preview. If no registered Windows preview handler can
			// display an Office file, show a clear message and let double-click open it.
			if isOfficeFallbackPath(path) {
				p.showFallback("Windows could not preview this Office file.\r\n\r\nDouble-click to open it in the associated Office application.", currentRect)
				continue
			}

			if isTextPreviewPath(path) {
				text, err := readTextPreview(path)
				if err != nil {
					p.showFallback("Preview unavailable:\r\n"+err.Error(), currentRect)
				} else {
					p.showFallback(text, currentRect)
				}
				continue
			}

			p.showFallback("No Windows preview handler is installed for this file type.\r\n\r\nDouble-click the result to open the file.", currentRect)
		default:
		}

		// STA COM components and some in-process preview handlers expect the
		// creating thread to pump Windows messages. Keep this lightweight pump
		// running even though preview commands arrive through Go channels.
		var m msg
		for {
			r, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmRemove)
			if r == 0 {
				break
			}
			if m.Message == wmQuit {
				return
			}
			if handlePaneArrowKeyMessage(&m) {
				didWork = true
				continue
			}
			if handlePreviewInputMessage(&m) {
				didWork = true
				continue
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
			didWork = true
		}
		if !didWork {
			time.Sleep(8 * time.Millisecond)
		}
	}
}

func (p *previewManager) clearHost(r rect) {
	p.destroyHTMLControl()
	showWindow(p.textEdit, false)
	showWindow(p.imageCtl, false)
	setWindowText(p.textEdit, "")
	setPos(p.textEdit, 0, 0, maxI32(1, r.Right-r.Left), maxI32(1, r.Bottom-r.Top))
	setPos(p.imageCtl, 0, 0, maxI32(1, r.Right-r.Left), maxI32(1, r.Bottom-r.Top))
	invalidateWindow(p.host)
}

func (p *previewManager) showFallback(text string, r rect) {
	showWindow(p.imageCtl, false)
	setPos(p.textEdit, 0, 0, maxI32(1, r.Right-r.Left), maxI32(1, r.Bottom-r.Top))
	setWindowText(p.textEdit, text)
	showWindow(p.textEdit, true)
	invalidateWindow(p.textEdit)
}

func (p *previewManager) showBitmap(hbmp uintptr, x, y, w, h int32) {
	showWindow(p.textEdit, false)
	sendMessage(p.imageCtl, stmSetImage, imageBitmap, hbmp)
	p.positionBitmap(x, y, w, h)
	showWindow(p.imageCtl, true)
	invalidateWindow(p.host)
	invalidateWindow(p.imageCtl)
}

func (p *previewManager) positionBitmap(x, y, w, h int32) {
	if p == nil || p.imageCtl == 0 {
		return
	}
	setPos(p.imageCtl, x, y, maxI32(1, w), maxI32(1, h))
	invalidateWindow(p.host)
}

func (p *previewManager) clearBitmap(hbmp *uintptr) {
	if p.imageCtl != 0 {
		old := sendMessage(p.imageCtl, stmSetImage, imageBitmap, 0)
		if old != 0 {
			procDeleteObject.Call(old)
		}
	}
	if hbmp != nil {
		*hbmp = 0
	}
}

func (p *previewManager) unload(s *previewSession) {
	if s == nil {
		return
	}
	if s.handler != 0 {
		comCall(s.handler, 6) // IPreviewHandler::Unload
		comRelease(s.handler)
		s.handler = 0
	}
	if s.stream != 0 {
		comRelease(s.stream)
		s.stream = 0
	}
	if s.item != 0 {
		comRelease(s.item)
		s.item = 0
	}
}

func (p *previewManager) startShellPreview(path, clsidText string, r rect) (previewSession, bool) {
	for _, mode := range previewInitOrder(path) {
		if sess, ok := p.startShellPreviewMode(path, clsidText, r, mode); ok {
			return sess, true
		}
	}
	return previewSession{}, false
}

type previewInitMode int

const (
	previewInitFile previewInitMode = iota
	previewInitItem
	previewInitStream
)

func previewInitOrder(path string) []previewInitMode {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".xls", ".ppt", ".doc":
		// Legacy Office handlers are most reliable when given the real file path.
		return []previewInitMode{previewInitFile, previewInitItem, previewInitStream}
	default:
		return []previewInitMode{previewInitStream, previewInitFile, previewInitItem}
	}
}

func (p *previewManager) startShellPreviewMode(path, clsidText string, r rect, mode previewInitMode) (previewSession, bool) {
	var clsid guid
	ps := utf16Ptr(clsidText)
	hr, _, _ := procCLSIDFromString.Call(uintptr(unsafe.Pointer(ps)), uintptr(unsafe.Pointer(&clsid)))
	if hresultFailed(hr) {
		return previewSession{}, false
	}

	var handler uintptr
	hr, _, _ = procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsid)), 0, clsctxInprocServer|clsctxLocalServer,
		uintptr(unsafe.Pointer(&iidIPreviewHandler)), uintptr(unsafe.Pointer(&handler)),
	)
	if hresultFailed(hr) || handler == 0 {
		return previewSession{}, false
	}

	sess := previewSession{handler: handler}
	if !initializePreviewHandlerMode(handler, path, &sess, mode) {
		p.unload(&sess)
		return previewSession{}, false
	}

	showWindow(p.textEdit, false)
	hr = comCall(handler, 3, p.host, uintptr(unsafe.Pointer(&r))) // SetWindow
	if hresultFailed(hr) {
		p.unload(&sess)
		return previewSession{}, false
	}
	hr = comCall(handler, 5) // DoPreview
	if hresultFailed(hr) {
		p.unload(&sess)
		return previewSession{}, false
	}
	stretchPreviewChildren(p.host, maxI32(1, r.Right-r.Left), maxI32(1, r.Bottom-r.Top))
	return sess, true
}

func initializePreviewHandlerMode(handler uintptr, path string, sess *previewSession, mode previewInitMode) bool {
	switch mode {
	case previewInitFile:
		if init, ok := comQueryInterface(handler, &iidIInitializeWithFile); ok {
			pp := utf16Ptr(path)
			hr := comCall(init, 3, uintptr(unsafe.Pointer(pp)), stgmRead|stgmShareDenyNone)
			comRelease(init)
			return !hresultFailed(hr)
		}
	case previewInitItem:
		if init, ok := comQueryInterface(handler, &iidIInitializeWithItem); ok {
			var item uintptr
			pp := utf16Ptr(path)
			hr, _, _ := procSHCreateItemFromParsingName.Call(
				uintptr(unsafe.Pointer(pp)), 0, uintptr(unsafe.Pointer(&iidIShellItem)), uintptr(unsafe.Pointer(&item)),
			)
			if !hresultFailed(hr) && item != 0 {
				hr = comCall(init, 3, item, stgmRead|stgmShareDenyNone)
				if !hresultFailed(hr) {
					sess.item = item
					comRelease(init)
					return true
				}
				comRelease(item)
			}
			comRelease(init)
		}
	case previewInitStream:
		if init, ok := comQueryInterface(handler, &iidIInitializeWithStream); ok {
			var stream uintptr
			pp := utf16Ptr(path)
			hr, _, _ := procSHCreateStreamOnFileEx.Call(
				uintptr(unsafe.Pointer(pp)), stgmRead|stgmShareDenyNone, fileAttributeNormal, 0, 0,
				uintptr(unsafe.Pointer(&stream)),
			)
			if !hresultFailed(hr) && stream != 0 {
				hr = comCall(init, 3, stream, stgmRead|stgmShareDenyNone)
				if !hresultFailed(hr) {
					sess.stream = stream
					comRelease(init)
					return true
				}
				comRelease(stream)
			}
			comRelease(init)
		}
	}
	return false
}

func comQueryInterface(obj uintptr, iid *guid) (uintptr, bool) {
	if obj == 0 || iid == nil {
		return 0, false
	}
	var out uintptr
	hr := comCall(obj, 0, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))
	return out, !hresultFailed(hr) && out != 0
}

func comRelease(obj uintptr) {
	if obj != 0 {
		comCall(obj, 2)
	}
}

func comCall(obj uintptr, method int, args ...uintptr) uintptr {
	if obj == 0 {
		return ^uintptr(0)
	}
	vtbl := *(*uintptr)(unsafe.Pointer(obj))
	fn := *(*uintptr)(unsafe.Pointer(vtbl + uintptr(method)*unsafe.Sizeof(uintptr(0))))
	callArgs := make([]uintptr, 0, len(args)+1)
	callArgs = append(callArgs, obj)
	callArgs = append(callArgs, args...)
	r1, _, _ := syscall.SyscallN(fn, callArgs...)
	return r1
}

func hresultFailed(hr uintptr) bool { return int32(uint32(hr)) < 0 }

func findPreviewHandlerCLSID(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return ""
	}

	var keys []string
	if progID, _ := regString(syscall.HKEY_CLASSES_ROOT, ext, ""); progID != "" {
		keys = append(keys, progID+`\shellex\`+previewHandlerShellExt)
	}
	keys = append(keys,
		`SystemFileAssociations\`+ext+`\shellex\`+previewHandlerShellExt,
		ext+`\shellex\`+previewHandlerShellExt,
	)
	if perceived, _ := regString(syscall.HKEY_CLASSES_ROOT, ext, "PerceivedType"); perceived != "" {
		keys = append(keys, `SystemFileAssociations\`+perceived+`\shellex\`+previewHandlerShellExt)
	}
	keys = append(keys,
		`*\shellex\`+previewHandlerShellExt,
		`AllFileSystemObjects\shellex\`+previewHandlerShellExt,
	)

	seen := map[string]bool{}
	for _, k := range keys {
		kl := strings.ToLower(k)
		if seen[kl] {
			continue
		}
		seen[kl] = true
		if v, _ := regString(syscall.HKEY_CLASSES_ROOT, k, ""); strings.HasPrefix(strings.TrimSpace(v), "{") {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func regString(root syscall.Handle, subKey, valueName string) (string, error) {
	var h syscall.Handle
	if err := syscall.RegOpenKeyEx(root, utf16Ptr(subKey), 0, syscall.KEY_READ, &h); err != nil {
		return "", err
	}
	defer syscall.RegCloseKey(h)

	var name *uint16
	if valueName != "" {
		name = utf16Ptr(valueName)
	}
	var typ uint32
	var n uint32
	if err := syscall.RegQueryValueEx(h, name, nil, &typ, nil, &n); err != nil {
		return "", err
	}
	if n == 0 {
		return "", nil
	}
	buf := make([]byte, n)
	if err := syscall.RegQueryValueEx(h, name, nil, &typ, &buf[0], &n); err != nil {
		return "", err
	}
	if typ != syscall.REG_SZ && typ != syscall.REG_EXPAND_SZ {
		return "", fmt.Errorf("registry value is not a string")
	}
	u := make([]uint16, 0, len(buf)/2)
	for i := 0; i+1 < int(n); i += 2 {
		v := binary.LittleEndian.Uint16(buf[i : i+2])
		if v == 0 {
			break
		}
		u = append(u, v)
	}
	return syscall.UTF16ToString(u), nil
}

func isTextPreviewPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".txt", ".csv", ".json", ".md", ".markdown", ".html", ".htm", ".xml", ".log", ".ini", ".yaml", ".yml", ".toml":
		return true
	default:
		return false
	}
}

func isImagePreviewPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp", ".tif", ".tiff":
		return true
	default:
		return false
	}
}

func loadShellImageScale(path string, originalW, originalH int32, scale float64) (uintptr, int32, int32, float64, bool) {
	targetW, targetH, effective := scaledImageSize(originalW, originalH, scale)
	hbmp, bw, bh, ok := loadShellImageTarget(path, targetW, targetH)
	return hbmp, bw, bh, effective, ok
}

func loadShellImageTarget(path string, w, h int32) (uintptr, int32, int32, bool) {
	var factory uintptr
	pp := utf16Ptr(path)
	hr, _, _ := procSHCreateItemFromParsingName.Call(
		uintptr(unsafe.Pointer(pp)), 0,
		uintptr(unsafe.Pointer(&iidIShellItemImageFactory)),
		uintptr(unsafe.Pointer(&factory)),
	)
	if hresultFailed(hr) || factory == 0 {
		return 0, 0, 0, false
	}
	defer comRelease(factory)
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	packedSize := uintptr(uint64(uint32(w)) | (uint64(uint32(h)) << 32))
	var hbmp uintptr
	// SIIGBF_SCALEUP keeps the preview responsive for small source images while
	// still allowing 1:1 and manual zoom requests.
	hr = comCall(factory, 3, packedSize, 0x100, uintptr(unsafe.Pointer(&hbmp))) // IShellItemImageFactory::GetImage
	if hresultFailed(hr) || hbmp == 0 {
		return 0, 0, 0, false
	}
	bw, bh, ok := bitmapDimensions(hbmp)
	if !ok {
		procDeleteObject.Call(hbmp)
		return 0, 0, 0, false
	}
	return hbmp, bw, bh, true
}

func bitmapDimensions(hbmp uintptr) (int32, int32, bool) {
	if hbmp == 0 {
		return 0, 0, false
	}
	var bm bitmapObject
	r, _, _ := procGetObjectW.Call(hbmp, unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm)))
	if r == 0 || bm.Width <= 0 || bm.Height <= 0 {
		return 0, 0, false
	}
	return bm.Width, bm.Height, true
}

func (p *previewManager) destroyHTMLControl() {
	if p == nil || p.htmlCtl == 0 {
		return
	}
	procDestroyWindow.Call(p.htmlCtl)
	p.htmlCtl = 0
}

func (p *previewManager) showHTMLInProcess(path string, r rect) bool {
	if p == nil || p.host == 0 {
		return false
	}
	p.destroyHTMLControl()
	ok, _, _ := procAtlAxWinInit.Call()
	if ok == 0 {
		return false
	}
	u, err := fileURL(path)
	if err != nil {
		return false
	}
	showWindow(p.textEdit, false)
	showWindow(p.imageCtl, false)
	class := utf16Ptr("AtlAxWin80")
	text := utf16Ptr(u)
	const wsClipSiblings = 0x04000000
	h, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(class)),
		uintptr(unsafe.Pointer(text)),
		uintptr(wsChild|wsVisible|wsClipChildren|wsClipSiblings),
		0, 0,
		uintptr(maxI32(1, r.Right-r.Left)),
		uintptr(maxI32(1, r.Bottom-r.Top)),
		p.host, 0, 0, 0,
	)
	if h == 0 {
		return false
	}
	p.htmlCtl = h
	showWindow(h, true)
	return true
}

func isHTMLPreviewPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".html", ".htm":
		return true
	default:
		return false
	}
}

func previewCacheDir() string {
	d := filepath.Join(os.TempDir(), "xFile_search_preview_cache")
	_ = os.MkdirAll(d, 0o755)
	return d
}

func previewCacheKey(path, suffix string) string {
	st, _ := os.Stat(path)
	stamp := int64(0)
	size := int64(0)
	if st != nil {
		stamp = st.ModTime().UnixNano()
		size = st.Size()
	}
	sum := sha1.Sum([]byte(strings.ToLower(path) + "|" + fmt.Sprint(size) + "|" + fmt.Sprint(stamp) + "|" + suffix))
	return fmt.Sprintf("%x", sum[:10])
}

func fileURL(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	filePath := filepath.ToSlash(abs)
	if len(filePath) >= 2 && filePath[1] == ':' {
		filePath = "/" + filePath
	}
	return (&url.URL{Scheme: "file", Path: filePath}).String(), nil
}

func isOfficeFallbackPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".xls", ".xlsx", ".xlsm", ".xlsb", ".ppt", ".pptx", ".pptm":
		return true
	default:
		return false
	}
}

func stretchPreviewChildren(host uintptr, w, h int32) {
	if host == 0 || w <= 0 || h <= 0 {
		return
	}
	child, _, _ := procGetWindow.Call(host, 5) // GW_CHILD
	for child != 0 {
		vis, _, _ := procIsWindowVisible.Call(child)
		if vis != 0 {
			setPos(child, 0, 0, w, h)
		}
		next, _, _ := procGetWindow.Call(child, 2) // GW_HWNDNEXT
		child = next
	}
}

func readTextPreview(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	info, _ := f.Stat()
	b, err := io.ReadAll(io.LimitReader(f, maxTextPreviewBytes+1))
	if err != nil {
		return "", err
	}
	truncated := len(b) > maxTextPreviewBytes
	if truncated {
		b = b[:maxTextPreviewBytes]
	}
	text := decodeTextBytes(b)

	if strings.EqualFold(filepath.Ext(path), ".json") {
		var out bytes.Buffer
		if json.Indent(&out, []byte(text), "", "  ") == nil {
			text = out.String()
		}
	}
	if truncated || (info != nil && info.Size() > maxTextPreviewBytes) {
		text += fmt.Sprintf("\r\n\r\n--- Preview truncated after %s ---", formatByteSize(maxTextPreviewBytes))
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.ReplaceAll(text, "\n", "\r\n"), nil
}

func decodeTextBytes(b []byte) string {
	if len(b) >= 3 && bytes.Equal(b[:3], []byte{0xEF, 0xBB, 0xBF}) {
		return string(b[3:])
	}
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
		return decodeUTF16(b[2:], binary.LittleEndian)
	}
	if len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF {
		return decodeUTF16(b[2:], binary.BigEndian)
	}
	if utf8.Valid(b) {
		return string(b)
	}
	// Legacy ANSI/DBCS files (including Korean system-codepage text) are common
	// in old archives. Let Windows decode them using the current ANSI codepage.
	if s := decodeACP(b); s != "" {
		return s
	}
	return string(bytes.ToValidUTF8(b, []byte("�")))
}

func decodeUTF16(b []byte, order binary.ByteOrder) string {
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		u = append(u, order.Uint16(b[i:i+2]))
	}
	return syscall.UTF16ToString(u)
}

func decodeACP(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	n, _, _ := procMultiByteToWideChar.Call(0, 0, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), 0, 0)
	if n == 0 {
		return ""
	}
	u := make([]uint16, int(n))
	got, _, _ := procMultiByteToWideChar.Call(0, 0, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), uintptr(unsafe.Pointer(&u[0])), n)
	if got == 0 {
		return ""
	}
	return syscall.UTF16ToString(u[:got])
}

func formatByteSize(n int) string {
	if n >= 1<<20 {
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	}
	if n >= 1<<10 {
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", n)
}

func maxI32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}
