package main

import (
	"math"
	"testing"
)

func TestFitImageScale(t *testing.T) {
	s := fitImageScale(1000, 800, 2000, 1000)
	if math.Abs(s-0.496) > 0.01 { // inset makes it just under 0.5
		t.Fatalf("unexpected fit scale: %v", s)
	}
}

func TestClampImagePanCentersSmallImage(t *testing.T) {
	x, y := clampImagePan(1000, 800, 400, 300, -99, 500)
	if x != 300 || y != 250 {
		t.Fatalf("got %d,%d", x, y)
	}
}

func TestClampImagePanBoundsLargeImage(t *testing.T) {
	x, y := clampImagePan(1000, 800, 1600, 1200, -900, 100)
	if x != -600 || y != 0 {
		t.Fatalf("got %d,%d", x, y)
	}
}

func TestZoomAnchorKeepsCursorPoint(t *testing.T) {
	// Cursor is at image center before zoom. It should still be at image center.
	x, y := zoomAnchorPan(1000, 800, 800, 600, 1600, 1200, 100, 100, 500, 400)
	if x != -300 || y != -200 {
		t.Fatalf("got %d,%d", x, y)
	}
}

func TestScaledImageSizeSafetyCap(t *testing.T) {
	w, h, s := scaledImageSize(10000, 5000, 1)
	if w != 8192 || h != 4096 || s >= 1 {
		t.Fatalf("got %dx%d scale=%f", w, h, s)
	}
}
