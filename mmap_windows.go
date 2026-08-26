//go:build windows

package main

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

var (
	procCreateFileMappingW = kernel32.NewProc("CreateFileMappingW")
	procMapViewOfFile      = kernel32.NewProc("MapViewOfFile")
	procUnmapViewOfFile    = kernel32.NewProc("UnmapViewOfFile")
	procCloseHandle        = kernel32.NewProc("CloseHandle")
)

const (
	pageReadOnly = 0x02
	fileMapRead  = 0x0004
)

func mapFileReadOnly(path string) ([]byte, func() error, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	size := st.Size()
	if size <= 0 {
		_ = f.Close()
		return nil, nil, fmt.Errorf("empty index file")
	}
	hMap, _, e := procCreateFileMappingW.Call(f.Fd(), 0, pageReadOnly, 0, 0, 0)
	if hMap == 0 {
		_ = f.Close()
		return nil, nil, fmt.Errorf("CreateFileMappingW failed: %v", e)
	}
	addr, _, e := procMapViewOfFile.Call(hMap, fileMapRead, 0, 0, 0)
	// The mapping/view keep the underlying section alive; the file and mapping
	// handles can be closed immediately after MapViewOfFile succeeds.
	procCloseHandle.Call(hMap)
	_ = f.Close()
	if addr == 0 {
		return nil, nil, fmt.Errorf("MapViewOfFile failed: %v", e)
	}
	if uint64(size) > uint64(^uint(0)) {
		procUnmapViewOfFile.Call(addr)
		return nil, nil, fmt.Errorf("index too large for this process")
	}
	data := unsafe.Slice((*byte)(unsafe.Pointer(addr)), int(size))
	closeFn := func() error {
		r, _, e := procUnmapViewOfFile.Call(addr)
		if r == 0 {
			return fmt.Errorf("UnmapViewOfFile failed: %v", e)
		}
		return nil
	}
	return data, closeFn, nil
}

// Ensure syscall remains referenced for Windows builds on older Go toolchains
// where os.File.Fd is a syscall.Handle under the hood.
var _ syscall.Handle
