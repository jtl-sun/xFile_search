package main

const (
	paneTargetNone = iota
	paneTargetList
	paneTargetPreview
)

// Keep the pure pane-switch decision separate from Win32 so it can be tested
// on any build host. key is the Windows VK value for Left (0x25) or Right
// (0x27), supplied by the Windows event adapter.
func paneSwitchTarget(key int, inList, inPreview, previewAvailable bool) int {
	if !previewAvailable {
		return paneTargetNone
	}
	if key == 0x27 && inList { // Right: file list -> Preview
		return paneTargetPreview
	}
	if key == 0x25 && inPreview { // Left: Preview -> file list
		return paneTargetList
	}
	return paneTargetNone
}
