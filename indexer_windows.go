//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	procGetCurrentProcess = kernel32.NewProc("GetCurrentProcess")
	procSetPriorityClass  = kernel32.NewProc("SetPriorityClass")
)

const (
	belowNormalPriorityClass   = 0x00004000
	processModeBackgroundBegin = 0x00100000
)

func runIndexerChild() int {
	// Keep the worker friendly to the desktop. Search/UI must always win.
	h, _, _ := procGetCurrentProcess.Call()
	if h != 0 {
		// PROCESS_MODE_BACKGROUND_BEGIN lowers CPU, memory and I/O scheduling
		// priority for this worker so the desktop/UI remains responsive. Fall
		// back to BELOW_NORMAL if background mode is unavailable.
		if r, _, _ := procSetPriorityClass.Call(h, processModeBackgroundBegin); r == 0 {
			procSetPriorityClass.Call(h, belowNormalPriorityClass)
		}
	}

	_, indexPath, cfgPath, logPath := DataPaths()
	setLogPath(logPath)
	_ = EnsureConfig(cfgPath)
	cfg := LoadConfig(cfgPath)
	roots := indexerRoots(cfg)
	progressPath := IndexerProgressPath()
	_ = os.MkdirAll(filepath.Dir(progressPath), 0o755)
	writeIndexerProgress(progressPath, "Starting background index...")
	logf("worker reindex start roots=%v", roots)

	count, err := BuildIndexFile(context.Background(), roots, indexPath, func(p ScanProgress) {
		root := p.Root
		if root == "" && len(roots) > 0 {
			root = roots[0]
		}
		msg := fmt.Sprintf("Indexing %s · %s items · %d skipped", root, formatCount(p.Count), p.Errors)
		writeIndexerProgress(progressPath, msg)
	})
	if err != nil {
		logf("worker reindex failed: %v", err)
		writeIndexerProgress(progressPath, "ERROR: "+err.Error())
		return 2
	}
	logf("worker reindex complete count=%d", count)
	writeIndexerProgress(progressPath, fmt.Sprintf("DONE · %s items indexed", formatCount(count)))
	// Give the UI enough time to observe the final status before it removes it.
	time.Sleep(150 * time.Millisecond)
	return 0
}

func indexerRoots(cfg Config) []string {
	if !cfg.AutoRoots && len(cfg.Roots) > 0 {
		return append([]string(nil), cfg.Roots...)
	}
	return DetectDefaultRoots()
}

func writeIndexerProgress(path, msg string) {
	tmp := path + ".tmp"
	_ = os.WriteFile(tmp, []byte(strings.TrimSpace(msg)), 0o644)
	_ = os.Rename(tmp, path)
}
