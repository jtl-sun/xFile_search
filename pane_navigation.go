package main

const (
	paneTargetNone = iota
	paneTargetList
	paneTargetPreview
)

func paneSwitchTarget(key int, inList, inPreview, previewAvailable bool) int {
	if !previewAvailable { return paneTargetNone }
	if key == 0x27 && inList { return paneTargetPreview }
	if key == 0x25 && inPreview { return paneTargetList }
	return paneTargetNone
}
