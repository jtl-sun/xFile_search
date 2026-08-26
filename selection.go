package main

func nextSelectionAfterDelete(deletedIndex, remainingCount int) int {
	if remainingCount <= 0 { return -1 }
	if deletedIndex < 0 { return 0 }
	if deletedIndex >= remainingCount { return remainingCount - 1 }
	return deletedIndex
}
