package main

import "math"

const (
	imageZoomMin = 0.02
	imageZoomMax = 8.0
)

// fitImageScale returns the scale (1.0 == one image pixel per screen pixel)
// that fits the whole image inside the preview viewport.
func fitImageScale(viewW, viewH, imageW, imageH int32) float64 {
	if viewW <= 0 || viewH <= 0 || imageW <= 0 || imageH <= 0 {
		return 1.0
	}
	// A small inset keeps the fitted image from touching the pane border.
	vw := float64(viewW - 8)
	if vw < 1 {
		vw = 1
	}
	vh := float64(viewH - 8)
	if vh < 1 {
		vh = 1
	}
	sx := vw / float64(imageW)
	sy := vh / float64(imageH)
	s := math.Min(sx, sy)
	if s < imageZoomMin {
		s = imageZoomMin
	}
	if s > imageZoomMax {
		s = imageZoomMax
	}
	return s
}

func scaledImageSize(imageW, imageH int32, scale float64) (int32, int32, float64) {
	if imageW <= 0 || imageH <= 0 {
		return 1, 1, 1
	}
	if scale < imageZoomMin {
		scale = imageZoomMin
	}
	if scale > imageZoomMax {
		scale = imageZoomMax
	}
	w := int32(math.Round(float64(imageW) * scale))
	h := int32(math.Round(float64(imageH) * scale))
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}

	// IShellItemImageFactory can become extremely memory hungry with giant
	// requests. 8192px still covers typical camera images at 1:1 while keeping
	// worst-case preview allocations bounded.
	const maxRenderDim int32 = 8192
	if w > maxRenderDim || h > maxRenderDim {
		factor := math.Min(float64(maxRenderDim)/float64(w), float64(maxRenderDim)/float64(h))
		w = int32(math.Max(1, math.Round(float64(w)*factor)))
		h = int32(math.Max(1, math.Round(float64(h)*factor)))
		scale *= factor
	}
	return w, h, scale
}

func centeredImagePan(viewW, viewH, imageW, imageH int32) (int32, int32) {
	return clampImagePan(viewW, viewH, imageW, imageH, (viewW-imageW)/2, (viewH-imageH)/2)
}

// clampImagePan keeps the image inside the preview pane. If an image is
// smaller than the pane on one axis it is centered on that axis; otherwise it
// can be panned until either edge reaches the pane edge.
func clampImagePan(viewW, viewH, imageW, imageH, x, y int32) (int32, int32) {
	if imageW <= viewW {
		x = (viewW - imageW) / 2
	} else {
		minX := viewW - imageW
		if x < minX {
			x = minX
		}
		if x > 0 {
			x = 0
		}
	}
	if imageH <= viewH {
		y = (viewH - imageH) / 2
	} else {
		minY := viewH - imageH
		if y < minY {
			y = minY
		}
		if y > 0 {
			y = 0
		}
	}
	return x, y
}

// zoomAnchorPan calculates the new top-left position after zoom so that the
// image point under the mouse cursor stays under that same cursor position.
func zoomAnchorPan(viewW, viewH, oldW, oldH, newW, newH, oldX, oldY, cursorX, cursorY int32) (int32, int32) {
	if oldW <= 0 || oldH <= 0 || newW <= 0 || newH <= 0 {
		return centeredImagePan(viewW, viewH, newW, newH)
	}
	rx := float64(cursorX-oldX) / float64(oldW)
	ry := float64(cursorY-oldY) / float64(oldH)
	if rx < 0 {
		rx = 0
	}
	if rx > 1 {
		rx = 1
	}
	if ry < 0 {
		ry = 0
	}
	if ry > 1 {
		ry = 1
	}
	nx := int32(math.Round(float64(cursorX) - rx*float64(newW)))
	ny := int32(math.Round(float64(cursorY) - ry*float64(newH)))
	return clampImagePan(viewW, viewH, newW, newH, nx, ny)
}
