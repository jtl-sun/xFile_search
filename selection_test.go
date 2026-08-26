package main

import "testing"

func TestNextSelectionAfterDelete(t *testing.T) {
	tests := []struct {
		name  string
		row   int
		count int
		want  int
	}{
		{"middle row selects replacement at same index", 5, 10, 5},
		{"first row selects new first row", 0, 9, 0},
		{"deleted last row selects previous row", 9, 9, 8},
		{"single item leaves no selection", 0, 0, -1},
		{"missing prior selection chooses first row", -1, 4, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextSelectionAfterDelete(tc.row, tc.count); got != tc.want {
				t.Fatalf("nextSelectionAfterDelete(%d,%d)=%d, want %d", tc.row, tc.count, got, tc.want)
			}
		})
	}
}
