//go:build !windows

package main

import "os"

// Tests in the Linux build environment use a normal byte slice. Production
// Windows builds use a true memory-mapped file in mmap_windows.go.
func mapFileReadOnly(path string) ([]byte, func() error, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	return b, func() error { return nil }, nil
}
