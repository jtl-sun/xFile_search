//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procGetLogicalDrives = kernel32.NewProc("GetLogicalDrives")
	procGetDriveTypeW    = kernel32.NewProc("GetDriveTypeW")
)

const (
	driveRemovable = 2
	driveFixed     = 3
)

func DetectDefaultRoots() []string {
	mask, _, _ := procGetLogicalDrives.Call()
	roots := make([]string, 0, 4)
	for i := 0; i < 26; i++ {
		if mask&(1<<i) == 0 {
			continue
		}
		root := fmt.Sprintf("%c:\\", 'A'+i)
		p, _ := syscall.UTF16PtrFromString(root)
		typ, _, _ := procGetDriveTypeW.Call(uintptr(unsafe.Pointer(p)))
		// Auto mode indexes only fixed local drives. Removable media can be
		// added explicitly in xFile_search.ini; auto-scanning a slow USB drive
		// can otherwise make the whole PC feel stalled during the first index.
		if typ == driveFixed {
			roots = append(roots, root)
		}
	}
	if len(roots) == 0 {
		roots = []string{"C:\\"}
	}
	return roots
}
