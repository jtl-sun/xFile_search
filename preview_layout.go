package main

const (
	previewDefaultPercent = 44
	previewMinPercent     = 20
	previewMaxPercent     = 70
	previewMinWidth       = int32(280)
	previewMinListWidth   = int32(360)
	previewSplitterWidth  = int32(7)
)

func clampPreviewPercent(p int) int {
	if p < previewMinPercent || p > previewMaxPercent { return previewDefaultPercent }
	return p
}

func previewPaneWidths(contentW int32, percent int, dragPreviewW int32) (listW, previewW int32) {
	if contentW <= previewSplitterWidth { return 0, 0 }
	if dragPreviewW > 0 { previewW = dragPreviewW } else { percent = clampPreviewPercent(percent); previewW = contentW * int32(percent) / 100 }
	if previewW < previewMinWidth { previewW = previewMinWidth }
	maxPreview := contentW - previewSplitterWidth - previewMinListWidth
	if previewW > maxPreview { previewW = maxPreview }
	if previewW < 1 { previewW = 1 }
	listW = contentW - previewSplitterWidth - previewW
	if listW < 0 { listW = 0 }
	return listW, previewW
}

func previewPercentForWidth(contentW, previewW int32) int {
	if contentW <= 0 { return previewDefaultPercent }
	p := int((previewW*100 + contentW/2) / contentW)
	if p < previewMinPercent { p = previewMinPercent }
	if p > previewMaxPercent { p = previewMaxPercent }
	return p
}
