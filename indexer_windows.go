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
	driveFingerprint := currentDriveStateText(roots)
	partialPath := strings.TrimSpace(os.Getenv("XFILE_PARTIAL_INDEX"))
	progressPath := IndexerProgressPath()
	_ = os.MkdirAll(filepath.Dir(progressPath), 0o755)
	_ = os.Remove(progressPath)
	writeIndexerProgress(progressPath, "INDEXING · starting background index...")
	logf("worker reindex start roots=%v", roots)

	count, err := BuildIndexFileProgressive(context.Background(), roots, indexPath, partialPath, 5000, func(p ScanProgress) {
		root := p.Root
		if root == "" && len(roots) > 0 {
			root = roots[0]
		}
		current := compactProgressPath(p.Current, 72)
		msg := fmt.Sprintf("INDEXING · %s · %s items · %d skipped", root, formatCount(p.Count), p.Errors)
		if current != "" {
			msg += " · " + current
		}
		writeIndexerProgress(progressPath, msg)
	})
	if err != nil {
		logf("worker reindex failed: %v", err)
		writeIndexerProgress(progressPath, "ERROR: "+err.Error())
		return 2
	}
	if err := writeDriveStateText(DriveStatePath(), driveFingerprint); err != nil {
		logf("drive-state fingerprint save failed: %v", err)
	}
	logf("worker reindex complete count=%d", count)
	writeIndexerProgress(progressPath, fmt.Sprintf("DONE · %s items indexed", formatCount(count)))
	// Give the UI enough time to observe the final status before it removes it.
	time.Sleep(150 * time.Millisecond)
	return 0
}

func indexerRoots(cfg Config) []string {
	var roots []string
	if !cfg.AutoRoots && len(cfg.Roots) > 0 {
		roots = append([]string(nil), cfg.Roots...)
	} else {
		roots = DetectDefaultRoots()
	}
	preferred := splitRootList(os.Getenv("XFILE_PRIORITY_VOLUMES"))
	return prioritizeIndexRoots(roots, preferred)
}

func splitRootList(raw string) []string {
	var out []string
	for _, p := range strings.Split(raw, ";") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func writeIndexerProgress(path, msg string) {
	tmp := path + ".tmp"
	_ = os.WriteFile(tmp, []byte(strings.TrimSpace(msg)), 0o644)
	_ = os.Rename(tmp, path)
}

func compactProgressPath(path string, maxRunes int) string {
	path = strings.TrimSpace(path)
	if path == "" || maxRunes <= 0 {
		return ""
	}
	r := []rune(path)
	if len(r) <= maxRunes {
		return path
	}
	if maxRunes <= 3 {
		return string(r[len(r)-maxRunes:])
	}
	return "..." + string(r[len(r)-(maxRunes-3):])
}
