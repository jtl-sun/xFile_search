package main

import "testing"

func TestPreviewPaneWidthsStable(t *testing.T) {
	for _, content := range []int32{760, 900, 1200, 1600, 2200, 3200} {
		listW, previewW := previewPaneWidths(content, 44, 0)
		if listW < 0 || previewW <= 0 { t.Fatalf("content=%d invalid widths list=%d preview=%d", content, listW, previewW) }
		if listW+previewW+previewSplitterWidth != content { t.Fatalf("content=%d widths do not add up: %d+%d+%d", content, listW, previewW, previewSplitterWidth) }
	}
}

func TestPreviewDragUsesExactPixels(t *testing.T) {
	content := int32(1600)
	for _, want := range []int32{300, 401, 517, 700, 900} {
		_, got := previewPaneWidths(content, 44, want)
		maxPreview := content - previewSplitterWidth - previewMinListWidth
		expected := want
		if expected < previewMinWidth { expected = previewMinWidth }
		if expected > maxPreview { expected = maxPreview }
		if got != expected { t.Fatalf("drag=%d got=%d want=%d", want, got, expected) }
	}
}

func TestPreviewPercentRoundTripDoesNotDrift(t *testing.T) {
	content := int32(1573)
	for _, pct := range []int{20, 33, 44, 57, 70} {
		_, w := previewPaneWidths(content, pct, 0)
		got := previewPercentForWidth(content, w)
		if got < previewMinPercent || got > previewMaxPercent { t.Fatalf("pct=%d roundtrip out of range: %d", pct, got) }
		_, w2 := previewPaneWidths(content, got, 0)
		d := w2 - w
		if d < -8 || d > 8 { t.Fatalf("pct=%d width drift too large: %d -> %d (delta=%d)", pct, w, w2, d) }
	}
}
