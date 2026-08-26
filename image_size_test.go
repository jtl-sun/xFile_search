package main

import (
	"encoding/binary"
	"testing"
)

func TestBMPPixelSize(t *testing.T) {
	b := make([]byte, 26)
	copy(b[:2], "BM")
	binary.LittleEndian.PutUint32(b[18:22], 1920)
	binary.LittleEndian.PutUint32(b[22:26], 1080)
	w, h, ok := bmpPixelSize(b)
	if !ok || w != 1920 || h != 1080 {
		t.Fatalf("got %d x %d ok=%v", w, h, ok)
	}
}

func TestWebPVP8XPixelSize(t *testing.T) {
	b := make([]byte, 30)
	copy(b[:4], "RIFF")
	copy(b[8:12], "WEBP")
	copy(b[12:16], "VP8X")
	w, h := uint32(640-1), uint32(480-1)
	b[24], b[25], b[26] = byte(w), byte(w>>8), byte(w>>16)
	b[27], b[28], b[29] = byte(h), byte(h>>8), byte(h>>16)
	gotW, gotH, ok := webpPixelSize(b)
	if !ok || gotW != 640 || gotH != 480 {
		t.Fatalf("got %d x %d ok=%v", gotW, gotH, ok)
	}
}

func TestTIFFPixelSize(t *testing.T) {
	b := make([]byte, 8+2+24)
	copy(b[:2], "II")
	binary.LittleEndian.PutUint16(b[2:4], 42)
	binary.LittleEndian.PutUint32(b[4:8], 8)
	binary.LittleEndian.PutUint16(b[8:10], 2)
	// Width tag 256, LONG, count 1, value 1200
	binary.LittleEndian.PutUint16(b[10:12], 256)
	binary.LittleEndian.PutUint16(b[12:14], 4)
	binary.LittleEndian.PutUint32(b[14:18], 1)
	binary.LittleEndian.PutUint32(b[18:22], 1200)
	// Height tag 257
	binary.LittleEndian.PutUint16(b[22:24], 257)
	binary.LittleEndian.PutUint16(b[24:26], 4)
	binary.LittleEndian.PutUint32(b[26:30], 1)
	binary.LittleEndian.PutUint32(b[30:34], 800)
	w, h, ok := tiffPixelSize(b)
	if !ok || w != 1200 || h != 800 {
		t.Fatalf("got %d x %d ok=%v", w, h, ok)
	}
}
