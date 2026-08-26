//go:build windows

package main

import (
	"path/filepath"
	"syscall"
	"unsafe"
)

var procGetLogicalDrives = kernel32.NewProc("GetLogicalDrives")

func DetectDefaultRoots() []string {
	mask, _, _ := procGetLogicalDrives.Call()
	out := make([]string, 0, 8)
	for i := 0; i < 26; i++ {
		if mask&(1<<i) == 0 {
			continue
		}
		root := string(rune('A'+i)) + `:\`
		p, _ := syscall.UTF16PtrFromString(root)
		typeID, _, _ := procGetDriveTypeW.Call(uintptr(unsafe.Pointer(p)))
		// fixed/removable RAM disks only. Exclude CD, network and unknown.
		if typeID == 2 || typeID == 3 || typeID == 6 {
			out = append(out, filepath.Clean(root))
		}
	}
	return out
}

func DriveExists(root string) bool {
	p, _ := syscall.UTF16PtrFromString(root)
	typeID, _, _ := procGetDriveTypeW.Call(uintptr(unsafe.Pointer(p)))
	return typeID != 0 && typeID != 1
}
