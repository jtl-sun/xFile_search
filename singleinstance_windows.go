//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

var procCreateMutexW = kernel32.NewProc("CreateMutexW")

const errorAlreadyExists = 183

func acquireSingleInstance() (release func(), first bool, err error) {
	name, _ := syscall.UTF16PtrFromString(`Local\xFile_search_single_instance`)
	h, _, e := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		return func() {}, false, fmt.Errorf("CreateMutexW failed: %v", e)
	}
	if e == syscall.Errno(errorAlreadyExists) {
		procCloseHandle.Call(h)
		return func() {}, false, nil
	}
	return func() { procCloseHandle.Call(h) }, true, nil
}
