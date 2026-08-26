package main

// nextSelectionAfterDelete keeps the user's position stable after the selected
// row disappears. If there is still an item at the same row, that item is the
// natural "next" file. When the deleted row was the last one, select the new
// last row instead.
func nextSelectionAfterDelete(preferredRow, itemCount int) int {
	if itemCount <= 0 {
		return -1
	}
	if preferredRow < 0 {
		return 0
	}
	if preferredRow >= itemCount {
		return itemCount - 1
	}
	return preferredRow
}
