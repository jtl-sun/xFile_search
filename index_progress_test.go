package main

import "testing"

func TestEstimateIndexPercent(t *testing.T) {
	cases := []struct {
		count, expected, want int
	}{
		{0, 1000, 0},
		{1, 1000, 1},
		{500, 1000, 50},
		{999, 1000, 99},
		{1000, 1000, 99},
		{2000, 1000, 99},
		{100, 0, 1},
	}
	for _, tc := range cases {
		if got := estimateIndexPercent(tc.count, tc.expected); got != tc.want {
			t.Fatalf("count=%d expected=%d: got %d want %d", tc.count, tc.expected, got, tc.want)
		}
	}
}

func TestParseIndexPercent(t *testing.T) {
	got, ok := parseIndexPercent("INDEXING... 37% - 10000 items")
	if !ok || got != 37 {
		t.Fatalf("got %d ok=%v", got, ok)
	}
	if _, ok := parseIndexPercent("DONE - 100%"); ok {
		t.Fatal("DONE text must not be treated as an active INDEXING percentage")
	}
}

func TestIndexProgressText(t *testing.T) {
	if got := indexProgressText(42); got != "INDEXING... 42%" {
		t.Fatalf("unexpected %q", got)
	}
	if got := indexProgressText(120); got != "INDEXING... 100%" {
		t.Fatalf("unexpected clamp %q", got)
	}
}
