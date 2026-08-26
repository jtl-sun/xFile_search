package main

import "testing"

func TestResultColumnWidthsFitAndStayVisible(t *testing.T) {
	for _, width := range []int32{360, 480, 720, 1200} {
		n, p, size, d := resultColumnWidths(width)
		if n <= 0 || p <= 0 || size <= 0 || d <= 0 {
			t.Fatalf("width %d produced non-positive column: %d %d %d %d", width, n, p, size, d)
		}
		if size < 72 || d < 108 {
			t.Fatalf("fixed columns became unreadable at %d: size=%d date=%d", width, size, d)
		}
		if got, max := n+p+size+d, width-22; got > max && width >= 360 {
			t.Fatalf("columns overflow list: width=%d columns=%d max=%d", width, got, max)
		}
	}
}

func TestResultColumnWidthsGrowElasticColumns(t *testing.T) {
	n1, p1, _, _ := resultColumnWidths(480)
	n2, p2, _, _ := resultColumnWidths(1000)
	if n2 <= n1 || p2 <= p1 {
		t.Fatalf("elastic columns did not grow: 480=(%d,%d) 1000=(%d,%d)", n1, p1, n2, p2)
	}
}
