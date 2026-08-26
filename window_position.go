package main

func centeredWindowPosition(left, top, right, bottom, winW, winH int32) (int32, int32) {
	workW := right - left
	workH := bottom - top
	x := left
	y := top
	if workW > winW { x = left + (workW-winW)/2 }
	if workH > winH { y = top + (workH-winH)/2 }
	return x, y
}
