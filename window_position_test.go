package main

import "testing"

func TestCenteredWindowPosition(t *testing.T) {
	tests := []struct {
		name                     string
		left, top, right, bottom int32
		winW, winH               int32
		wantX, wantY             int32
	}{
		{"1080p", 0, 0, 1920, 1040, 1200, 720, 360, 160},
		{"taskbar-left", 80, 0, 1920, 1080, 1200, 720, 400, 180},
		{"window-wider-than-area", 0, 0, 1024, 768, 1200, 720, 0, 24},
		{"window-taller-than-area", 0, 0, 1920, 700, 1200, 720, 360, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x, y := centeredWindowPosition(tt.left, tt.top, tt.right, tt.bottom, tt.winW, tt.winH)
			if x != tt.wantX || y != tt.wantY {
				t.Fatalf("got (%d,%d), want (%d,%d)", x, y, tt.wantX, tt.wantY)
			}
		})
	}
}
