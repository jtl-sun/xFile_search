package main

import "testing"

func TestListRowFromSearchArrow(t *testing.T) {
	tests := []struct {
		name                      string
		dir, current, count, want int
	}{
		{"down enters first result", 1, -1, 20, 0},
		{"up enters last result", -1, -1, 20, 19},
		{"down continues from selected", 1, 4, 20, 5},
		{"up continues from selected", -1, 4, 20, 3},
		{"down clamps at last", 1, 19, 20, 19},
		{"up clamps at first", -1, 0, 20, 0},
		{"empty result", 1, -1, 0, -1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := listRowFromSearchArrow(tc.dir, tc.current, tc.count); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}
