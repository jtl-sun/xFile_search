package main

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

type ScanProgress struct {
	Root    string
	Current string
	Count   int
	Errors  int
}

var searchBusy atomic.Bool

// BuildIndex is retained for tests and small in-process jobs.
func BuildIndex(ctx context.Context, roots []string, progress func(ScanProgress)) (*IndexSnapshot, error) {
	if len(roots) == 0 {
		return nil, errors.New("no index roots")
	}
	entries := make([]Entry, 0, 250_000)
	errs := 0
	count := 0
	lastReport := time.Now()

	for _, root := range roots {
		root = filepath.Clean(root)
		if _, err := os.Stat(root); err != nil {
			errs++
			continue
		}
		entries = append(entries, NewEntry(root, true))
		count++
		stack := []string{root}
		for len(stack) > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
			dir := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			f, err := os.Open(dir)
			if err != nil {
				errs++
				continue
			}
			for {
				batch, err := f.ReadDir(512)
				for _, d := range batch {
					p := filepath.Join(dir, d.Name())
					isDir := d.IsDir()
					if shouldSkipRuntimePath(p) {
						continue
					}
					entries = append(entries, NewEntry(p, isDir))
					count++
					if isDir && d.Type()&os.ModeSymlink == 0 {
						stack = append(stack, p)
					}
					throttleIndexer(count)
				}
				if err != nil {
					if !errors.Is(err, io.EOF) {
						errs++
					}
					break
				}
			}
			_ = f.Close()
			if progress != nil && (count&0x1fff == 0 || time.Since(lastReport) > 750*time.Millisecond) {
				progress(ScanProgress{Root: root, Current: dir, Count: count, Errors: errs})
				lastReport = time.Now()
			}
		}
	}
	if progress != nil {
		progress(ScanProgress{Count: count, Errors: errs})
	}
	return &IndexSnapshot{Entries: entries, BuiltAt: time.Now(), Roots: append([]string(nil), roots...), Source: "background scan"}, nil
}

// BuildIndexFile streams both path records and a compact offset table to disk.
// The UI later memory-maps this file; it never rebuilds millions of Entry
// objects in RAM at startup.
func BuildIndexFile(ctx context.Context, roots []string, path string, progress func(ScanProgress)) (int, error) {
	return buildIndexFile(ctx, roots, path, "", 0, progress)
}

// BuildIndexFileProgressive writes the normal final index and, once enough
// entries are available, also publishes one valid read-only partial index.
// The UI can memory-map that partial checkpoint immediately while the worker
// keeps scanning the remaining directories in the background.
func BuildIndexFileProgressive(ctx context.Context, roots []string, path, partialPath string, partialAfter int, progress func(ScanProgress)) (int, error) {
	return buildIndexFile(ctx, roots, path, partialPath, partialAfter, progress)
}

func buildIndexFile(ctx context.Context, roots []string, path, partialPath string, partialAfter int, progress func(ScanProgress)) (int, error) {
	if len(roots) == 0 {
		return 0, errors.New("no index roots")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, err
	}

	tmp := path + ".tmp"
	offsetsTmp := path + ".offsets.tmp"
	_ = os.Remove(tmp)
	_ = os.Remove(offsetsTmp)
	f, err := os.Create(tmp)
	if err != nil {
		return 0, err
	}
	offFile, err := os.Create(offsetsTmp)
	if err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return 0, err
	}
	committed := false
	defer func() {
		_ = f.Close()
		_ = offFile.Close()
		_ = os.Remove(offsetsTmp)
		if !committed {
			_ = os.Remove(tmp)
		}
	}()

	builtAt := time.Now()
	w, ow, pos, err := beginIndexWrite(f, offFile, roots, builtAt)
	if err != nil {
		return 0, err
	}

	errs := 0
	count := 0
	lastReport := time.Now()
	partialPublished := false

	publishPartial := func() {
		if partialPublished || partialPath == "" || count <= 0 {
			return
		}
		if err := w.Flush(); err != nil {
			return
		}
		if err := ow.Flush(); err != nil {
			return
		}
		if err := f.Sync(); err != nil {
			return
		}
		if err := offFile.Sync(); err != nil {
			return
		}
		if err := writeIndexCheckpoint(tmp, offsetsTmp, partialPath, pos, uint64(count)); err == nil {
			partialPublished = true
		}
	}

	for _, root := range roots {
		root = filepath.Clean(root)
		if _, err := os.Stat(root); err != nil {
			errs++
			continue
		}
		if err := writeIndexEntry(w, ow, &pos, root, true); err != nil {
			return count, err
		}
		count++
		stack := []string{root}
		for len(stack) > 0 {
			select {
			case <-ctx.Done():
				return count, ctx.Err()
			default:
			}
			dir := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			df, err := os.Open(dir)
			if err != nil {
				errs++
				continue
			}
			for {
				batch, readErr := df.ReadDir(512)
				for _, d := range batch {
					p := filepath.Join(dir, d.Name())
					isDir := d.IsDir()
					if shouldSkipRuntimePath(p) {
						continue
					}
					if err := writeIndexEntry(w, ow, &pos, p, isDir); err != nil {
						_ = df.Close()
						return count, err
					}
					count++
					if isDir && d.Type()&os.ModeSymlink == 0 {
						stack = append(stack, p)
					}
					throttleIndexer(count)
				}
				if !partialPublished && partialAfter > 0 && count >= partialAfter {
					publishPartial()
				}
				if progress != nil && (count&0x1fff == 0 || time.Since(lastReport) > 750*time.Millisecond) {
					progress(ScanProgress{Root: root, Current: dir, Count: count, Errors: errs})
					lastReport = time.Now()
				}
				if readErr != nil {
					if !errors.Is(readErr, io.EOF) {
						errs++
					}
					break
				}
			}
			_ = df.Close()
			if progress != nil && (count&0x1fff == 0 || time.Since(lastReport) > 750*time.Millisecond) {
				progress(ScanProgress{Root: root, Current: dir, Count: count, Errors: errs})
				lastReport = time.Now()
			}
		}
		// If the first root completed before reaching the normal checkpoint
		// threshold, publish it now. This is especially useful for a swapped
		// external drive that contains fewer than partialAfter entries.
		if !partialPublished && partialPath != "" && count > 0 {
			publishPartial()
		}
	}

	if err := finishIndexWrite(f, offFile, w, ow, pos, uint64(count)); err != nil {
		return count, err
	}
	_ = f.Close()
	_ = offFile.Close()
	if err := replaceFile(tmp, path); err != nil {
		return count, err
	}
	committed = true
	if progress != nil {
		progress(ScanProgress{Count: count, Errors: errs})
	}
	return count, nil
}

func writeIndexCheckpoint(recordsPath, offsetsPath, outPath string, offsetTableAt, count uint64) error {
	if outPath == "" || count == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	tmp := outPath + ".tmp"
	_ = os.Remove(tmp)

	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = out.Close()
		if !ok {
			_ = os.Remove(tmp)
		}
	}()

	records, err := os.Open(recordsPath)
	if err != nil {
		return err
	}
	if _, err := io.CopyBuffer(out, records, make([]byte, 1<<20)); err != nil {
		_ = records.Close()
		return err
	}
	_ = records.Close()

	offsets, err := os.Open(offsetsPath)
	if err != nil {
		return err
	}
	if _, err := io.CopyBuffer(out, offsets, make([]byte, 1<<20)); err != nil {
		_ = offsets.Close()
		return err
	}
	_ = offsets.Close()

	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], count)
	if _, err := out.WriteAt(buf[:], indexHeaderCountOffset); err != nil {
		return err
	}
	binary.LittleEndian.PutUint64(buf[:], offsetTableAt)
	if _, err := out.WriteAt(buf[:], indexHeaderOffsetsOffset); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := replaceFile(tmp, outPath); err != nil {
		return err
	}
	ok = true
	return nil
}

func throttleIndexer(count int) {
	if count&0x7ff != 0 {
		return
	}
	if searchBusy.Load() {
		time.Sleep(8 * time.Millisecond)
	} else if runtime.GOOS == "windows" {
		// Separate process + below-normal priority + a small periodic yield.
		time.Sleep(2 * time.Millisecond)
	}
}

// Keep xFile_search's own generated index/log/backup data out of the index.
// This avoids indexing a file while the indexer is actively rewriting it.
func shouldSkipRuntimePath(path string) bool {
	dataDir, indexPath, _, logPath := DataPaths()
	dirs := []string{filepath.Dir(indexPath), filepath.Dir(logPath), filepath.Join(dataDir, "Backup")}
	clean := filepath.Clean(path)
	cleanLower := strings.ToLower(clean)
	for _, dir := range dirs {
		dir = filepath.Clean(dir)
		dirLower := strings.ToLower(dir)
		if cleanLower == dirLower || strings.HasPrefix(cleanLower, dirLower+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}
