package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	indexHeaderCountOffset   = int64(12) // magic(8) + version(4)
	indexHeaderOffsetsOffset = int64(32) // + count(8) + built(8) + roots(4)
	indexFixedHeaderSize     = 40
)

// SaveIndex is used by tests and small helper jobs. Production indexing uses
// BuildIndexFile, which streams filesystem entries and offsets directly to disk.
func SaveIndex(path string, snap *IndexSnapshot) error {
	if snap == nil {
		return errors.New("nil snapshot")
	}
	if snap.Mapped != nil {
		return errors.New("SaveIndex does not copy an already mapped index")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	offsetsTmp := path + ".offsets.tmp"
	_ = os.Remove(tmp)
	_ = os.Remove(offsetsTmp)

	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	offFile, err := os.Create(offsetsTmp)
	if err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	ok := false
	defer func() {
		_ = f.Close()
		_ = offFile.Close()
		_ = os.Remove(offsetsTmp)
		if !ok {
			_ = os.Remove(tmp)
		}
	}()

	builtAt := snap.BuiltAt
	if builtAt.IsZero() {
		builtAt = time.Now()
	}
	w, ow, pos, err := beginIndexWrite(f, offFile, snap.Roots, builtAt)
	if err != nil {
		return err
	}
	count := uint64(0)
	for _, e := range snap.Entries {
		if err := writeIndexEntry(w, ow, &pos, e.Path, e.IsDir); err != nil {
			return err
		}
		count++
	}
	if err := finishIndexWrite(f, offFile, w, ow, pos, count); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := offFile.Close(); err != nil {
		return err
	}
	if err := replaceFile(tmp, path); err != nil {
		return err
	}
	ok = true
	return nil
}

func readIndexCountHint(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	var header [20]byte
	if _, err := io.ReadFull(f, header[:]); err != nil {
		return 0
	}
	if string(header[:len(indexMagic)]) != indexMagic {
		return 0
	}
	if binary.LittleEndian.Uint32(header[8:12]) != indexVersion {
		return 0
	}
	count := binary.LittleEndian.Uint64(header[12:20])
	if count > uint64(^uint(0)>>1) {
		return 0
	}
	return int(count)
}

func LoadIndex(path string, progress func(done, total uint64)) (*IndexSnapshot, error) {
	// v0.1.3 maps the index instead of reconstructing millions of Go objects.
	// This call therefore has near-constant startup cost and tiny heap impact.
	data, closeFn, err := mapFileReadOnly(path)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*IndexSnapshot, error) {
		_ = closeFn()
		return nil, err
	}
	if len(data) < indexFixedHeaderSize {
		return fail(fmt.Errorf("index file is truncated"))
	}
	if string(data[:len(indexMagic)]) != indexMagic {
		// Older v0.1.0/v0.1.1 indexes intentionally fail here and are rebuilt.
		return fail(fmt.Errorf("legacy or invalid index format; rebuild required"))
	}
	version := binary.LittleEndian.Uint32(data[8:12])
	if version != indexVersion {
		return fail(fmt.Errorf("unsupported index version %d", version))
	}
	count := binary.LittleEndian.Uint64(data[12:20])
	builtUnix := int64(binary.LittleEndian.Uint64(data[20:28]))
	rootCount := binary.LittleEndian.Uint32(data[28:32])
	offsetsAt := binary.LittleEndian.Uint64(data[32:40])

	if count > uint64(^uint32(0)) {
		return fail(fmt.Errorf("index contains too many items for this build: %d", count))
	}
	if offsetsAt < indexFixedHeaderSize || offsetsAt > uint64(len(data)) {
		return fail(fmt.Errorf("invalid offset table position"))
	}
	if count > 0 && offsetsAt+count*8 > uint64(len(data)) {
		return fail(fmt.Errorf("truncated offset table"))
	}

	roots := make([]string, 0, rootCount)
	cursor := uint64(indexFixedHeaderSize)
	for i := uint32(0); i < rootCount; i++ {
		if cursor+4 > offsetsAt {
			return fail(fmt.Errorf("invalid roots section"))
		}
		n := binary.LittleEndian.Uint32(data[cursor : cursor+4])
		cursor += 4
		if n > maxPathBytes || cursor+uint64(n) > offsetsAt {
			return fail(fmt.Errorf("invalid root path length"))
		}
		roots = append(roots, string(data[cursor:cursor+uint64(n)]))
		cursor += uint64(n)
	}

	mapped := &MappedIndex{
		data: data, closeMap: closeFn, count: count, offsetsAt: offsetsAt,
		builtAt: time.Unix(builtUnix, 0), roots: roots, sourcePath: path,
	}
	if count > 0 {
		// Validate the first and last records only. Full validation would defeat
		// the purpose of instant startup; malformed records are also bounds-
		// checked individually when searched/materialized.
		if _, _, _, ok := mapped.record(0); !ok {
			return fail(fmt.Errorf("invalid first index record"))
		}
		if _, _, _, ok := mapped.record(uint32(count - 1)); !ok {
			return fail(fmt.Errorf("invalid last index record"))
		}
	}
	if progress != nil {
		progress(count, count)
	}
	return &IndexSnapshot{
		Mapped: mapped, BuiltAt: mapped.builtAt, Roots: append([]string(nil), roots...), Source: "memory-mapped disk index",
	}, nil
}

func beginIndexWrite(f, offFile *os.File, roots []string, builtAt time.Time) (*bufio.Writer, *bufio.Writer, uint64, error) {
	w := bufio.NewWriterSize(f, 4<<20)
	ow := bufio.NewWriterSize(offFile, 1<<20)
	if _, err := w.WriteString(indexMagic); err != nil {
		return nil, nil, 0, err
	}
	if err := binary.Write(w, binary.LittleEndian, indexVersion); err != nil {
		return nil, nil, 0, err
	}
	if err := binary.Write(w, binary.LittleEndian, uint64(0)); err != nil { // count, patched later
		return nil, nil, 0, err
	}
	if err := binary.Write(w, binary.LittleEndian, builtAt.Unix()); err != nil {
		return nil, nil, 0, err
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(len(roots))); err != nil {
		return nil, nil, 0, err
	}
	if err := binary.Write(w, binary.LittleEndian, uint64(0)); err != nil { // offset table position, patched later
		return nil, nil, 0, err
	}
	pos := uint64(indexFixedHeaderSize)
	for _, root := range roots {
		n, err := writeRawString(w, root)
		if err != nil {
			return nil, nil, 0, err
		}
		pos += uint64(n)
	}
	return w, ow, pos, nil
}

func writeIndexEntry(w, ow *bufio.Writer, pos *uint64, path string, isDir bool) error {
	clean := filepath.Clean(path)
	b := []byte(clean)
	if len(b) > maxPathBytes {
		return fmt.Errorf("path is too long")
	}
	nameStart := 0
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] == '\\' || b[i] == '/' {
			nameStart = i + 1
			break
		}
	}
	if err := binary.Write(ow, binary.LittleEndian, *pos); err != nil {
		return err
	}
	flag := byte(0)
	if isDir {
		flag = 1
	}
	if err := w.WriteByte(flag); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(nameStart)); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(len(b))); err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	*pos += uint64(9 + len(b))
	return nil
}

func finishIndexWrite(f, offFile *os.File, w, ow *bufio.Writer, offsetTableAt, count uint64) error {
	if err := w.Flush(); err != nil {
		return err
	}
	if err := ow.Flush(); err != nil {
		return err
	}
	if err := offFile.Sync(); err != nil {
		return err
	}
	if _, err := offFile.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	if _, err := io.CopyBuffer(f, offFile, make([]byte, 1<<20)); err != nil {
		return err
	}

	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], count)
	if _, err := f.WriteAt(buf[:], indexHeaderCountOffset); err != nil {
		return err
	}
	binary.LittleEndian.PutUint64(buf[:], offsetTableAt)
	if _, err := f.WriteAt(buf[:], indexHeaderOffsetsOffset); err != nil {
		return err
	}
	return f.Sync()
}

func writeRawString(w *bufio.Writer, s string) (int, error) {
	b := []byte(s)
	if len(b) > maxPathBytes {
		return 0, fmt.Errorf("path is too long")
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(len(b))); err != nil {
		return 0, err
	}
	if _, err := w.Write(b); err != nil {
		return 0, err
	}
	return 4 + len(b), nil
}

func replaceFile(tmp, path string) error {
	if err := os.Rename(tmp, path); err == nil {
		return nil
	}
	_ = os.Remove(path)
	return os.Rename(tmp, path)
}
