package main

// listRowFromSearchArrow returns the row that should receive focus when the
// user presses Up/Down while the search edit still has keyboard focus.
// direction < 0 is Up, direction >= 0 is Down.
func listRowFromSearchArrow(direction, current, count int) int {
	if count <= 0 {
		return -1
	}
	if current < 0 || current >= count {
		if direction < 0 {
			return count - 1
		}
		return 0
	}
	if direction < 0 {
		if current > 0 {
			return current - 1
		}
		return 0
	}
	if current < count-1 {
		return current + 1
	}
	return count - 1
}
