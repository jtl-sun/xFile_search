package main

import (
	"encoding/binary"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
)

func imagePixelSize(path string) (int32, int32, bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()
	if cfg, format, err := image.DecodeConfig(f); err == nil && cfg.Width > 0 && cfg.Height > 0 {
		w, h := int32(cfg.Width), int32(cfg.Height)
		if format == "jpeg" {
			if _, err := f.Seek(0, io.SeekStart); err == nil {
				buf := make([]byte, 256*1024)
				n, _ := io.ReadFull(f, buf)
				if orientation := jpegEXIFOrientation(buf[:n]); orientation >= 5 && orientation <= 8 {
					w, h = h, w
				}
			}
		}
		return w, h, true
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0, 0, false
	}
	buf := make([]byte, 256*1024)
	n, _ := io.ReadFull(f, buf)
	buf = buf[:n]
	if w, h, ok := bmpPixelSize(buf); ok {
		return w, h, true
	}
	if w, h, ok := webpPixelSize(buf); ok {
		return w, h, true
	}
	if w, h, ok := tiffPixelSize(buf); ok {
		return w, h, true
	}
	return 0, 0, false
}

func bmpPixelSize(b []byte) (int32, int32, bool) {
	if len(b) < 26 || b[0] != 'B' || b[1] != 'M' {
		return 0, 0, false
	}
	w := int32(binary.LittleEndian.Uint32(b[18:22]))
	h := int32(binary.LittleEndian.Uint32(b[22:26]))
	if h < 0 {
		h = -h
	}
	if w <= 0 || h <= 0 {
		return 0, 0, false
	}
	return w, h, true
}

func webpPixelSize(b []byte) (int32, int32, bool) {
	if len(b) < 30 || string(b[:4]) != "RIFF" || string(b[8:12]) != "WEBP" {
		return 0, 0, false
	}
	chunk := string(b[12:16])
	switch chunk {
	case "VP8X":
		if len(b) < 30 {
			return 0, 0, false
		}
		w := 1 + int32(uint32(b[24])|uint32(b[25])<<8|uint32(b[26])<<16)
		h := 1 + int32(uint32(b[27])|uint32(b[28])<<8|uint32(b[29])<<16)
		return w, h, w > 0 && h > 0
	case "VP8L":
		if len(b) < 25 || b[20] != 0x2f {
			return 0, 0, false
		}
		bits := binary.LittleEndian.Uint32(b[21:25])
		w := int32(bits&0x3fff) + 1
		h := int32((bits>>14)&0x3fff) + 1
		return w, h, true
	case "VP8 ":
		// Lossy VP8 frame header: 3-byte frame tag, then start code 9d 01 2a,
		// followed by 14-bit width and height.
		if len(b) < 30 || b[23] != 0x9d || b[24] != 0x01 || b[25] != 0x2a {
			return 0, 0, false
		}
		w := int32(binary.LittleEndian.Uint16(b[26:28]) & 0x3fff)
		h := int32(binary.LittleEndian.Uint16(b[28:30]) & 0x3fff)
		return w, h, w > 0 && h > 0
	}
	return 0, 0, false
}

func tiffPixelSize(b []byte) (int32, int32, bool) {
	if len(b) < 8 {
		return 0, 0, false
	}
	var order binary.ByteOrder
	switch string(b[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 0, 0, false
	}
	if order.Uint16(b[2:4]) != 42 {
		return 0, 0, false
	}
	off := int(order.Uint32(b[4:8]))
	if off < 0 || off+2 > len(b) {
		return 0, 0, false
	}
	count := int(order.Uint16(b[off : off+2]))
	pos := off + 2
	var w, h int32
	for i := 0; i < count && pos+12 <= len(b); i, pos = i+1, pos+12 {
		tag := order.Uint16(b[pos : pos+2])
		if tag != 256 && tag != 257 {
			continue
		}
		typ := order.Uint16(b[pos+2 : pos+4])
		cnt := order.Uint32(b[pos+4 : pos+8])
		if cnt != 1 {
			continue
		}
		var v uint32
		switch typ {
		case 3: // SHORT, packed in the first two bytes of the value field
			v = uint32(order.Uint16(b[pos+8 : pos+10]))
		case 4: // LONG
			v = order.Uint32(b[pos+8 : pos+12])
		default:
			continue
		}
		if tag == 256 {
			w = int32(v)
		} else {
			h = int32(v)
		}
		if w > 0 && h > 0 {
			return w, h, true
		}
	}
	return 0, 0, false
}

func jpegEXIFOrientation(b []byte) int {
	if len(b) < 4 || b[0] != 0xff || b[1] != 0xd8 {
		return 1
	}
	pos := 2
	for pos+4 <= len(b) {
		if b[pos] != 0xff {
			pos++
			continue
		}
		marker := b[pos+1]
		pos += 2
		if marker == 0xd9 || marker == 0xda {
			break
		}
		if marker >= 0xd0 && marker <= 0xd7 || marker == 0x01 {
			continue
		}
		if pos+2 > len(b) {
			break
		}
		segLen := int(binary.BigEndian.Uint16(b[pos : pos+2]))
		if segLen < 2 || pos+segLen > len(b) {
			break
		}
		payload := b[pos+2 : pos+segLen]
		if marker == 0xe1 && len(payload) >= 14 && string(payload[:6]) == "Exif\x00\x00" {
			if o := tiffOrientation(payload[6:]); o != 0 {
				return o
			}
		}
		pos += segLen
	}
	return 1
}

func tiffOrientation(b []byte) int {
	if len(b) < 8 {
		return 0
	}
	var order binary.ByteOrder
	switch string(b[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 0
	}
	if order.Uint16(b[2:4]) != 42 {
		return 0
	}
	off := int(order.Uint32(b[4:8]))
	if off < 0 || off+2 > len(b) {
		return 0
	}
	count := int(order.Uint16(b[off : off+2]))
	pos := off + 2
	for i := 0; i < count && pos+12 <= len(b); i, pos = i+1, pos+12 {
		if order.Uint16(b[pos:pos+2]) != 0x0112 {
			continue
		}
		typ := order.Uint16(b[pos+2 : pos+4])
		cnt := order.Uint32(b[pos+4 : pos+8])
		if typ == 3 && cnt == 1 {
			return int(order.Uint16(b[pos+8 : pos+10]))
		}
	}
	return 0
}
