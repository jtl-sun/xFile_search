package main

import "testing"

func TestPaneSwitchTarget(t *testing.T) {
	tests := []struct {
		name             string
		key              int
		inList           bool
		inPreview        bool
		previewAvailable bool
		want             int
	}{
		{"right list to preview", 0x27, true, false, true, paneTargetPreview},
		{"left preview to list", 0x25, false, true, true, paneTargetList},
		{"left in list stays list", 0x25, true, false, true, paneTargetNone},
		{"right in preview stays preview", 0x27, false, true, true, paneTargetNone},
		{"no preview blocks right", 0x27, true, false, false, paneTargetNone},
		{"other key ignored", 0x26, true, false, true, paneTargetNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := paneSwitchTarget(tt.key, tt.inList, tt.inPreview, tt.previewAvailable); got != tt.want {
				t.Fatalf("paneSwitchTarget()=%d, want %d", got, tt.want)
			}
		})
	}
}
