package main

// resultColumnWidths returns widths for Name, Path, Size and Date that always
// fit inside the visible ListView area. Name and Path are deliberately elastic;
// Size and Date remain readable even when Preview makes the list narrow.
func resultColumnWidths(listWidth int32) (nameW, pathW, sizeW, dateW int32) {
	usable := listWidth - 22 // borders + typical vertical scrollbar allowance
	if usable < 280 {
		usable = 280
	}

	sizeW = 86
	dateW = 128
	remaining := usable - sizeW - dateW
	if remaining < 140 {
		// Keep all four headers visible on unusually narrow windows.
		sizeW = 72
		dateW = 108
		remaining = usable - sizeW - dateW
	}
	if remaining < 120 {
		remaining = 120
	}

	// Give Path slightly more room than Name because it usually contains more
	// context. Long Name/Path values are intentionally ellipsized by ListView.
	nameW = remaining * 42 / 100
	if nameW < 60 {
		nameW = 60
	}
	if nameW > 380 {
		nameW = 380
	}
	pathW = remaining - nameW
	if pathW < 60 {
		pathW = 60
		nameW = remaining - pathW
		if nameW < 60 {
			nameW = 60
		}
	}
	return
}
