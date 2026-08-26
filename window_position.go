package main

// centeredWindowPosition returns the top-left coordinate that centers a window
// inside the supplied work area. If the requested window is larger than the
// work area on either axis, it is anchored to that work-area edge so the title
// bar remains reachable.
func centeredWindowPosition(left, top, right, bottom, winW, winH int32) (int32, int32) {
	workW := right - left
	workH := bottom - top

	x := left
	y := top
	if workW > winW {
		x = left + (workW-winW)/2
	}
	if workH > winH {
		y = top + (workH-winH)/2
	}
	return x, y
}
