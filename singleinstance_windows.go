//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

var procCreateMutexW = kernel32.NewProc("CreateMutexW")

const errorAlreadyExists syscall.Errno = 183

func acquireSingleInstance() (func(), bool, error) {
	name := utf16Ptr(`Local\xFile_search_UI_v013`)
	h, _, e := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		return func() {}, false, fmt.Errorf("CreateMutexW failed: %v", e)
	}
	if errno, ok := e.(syscall.Errno); ok && errno == errorAlreadyExists {
		procCloseHandle.Call(h)
		return func() {}, false, nil
	}
	release := func() { procCloseHandle.Call(h) }
	return release, true, nil
}
