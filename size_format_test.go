package main

import "testing"

func TestFormatFileSize(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{-1, "—"}, {0, "0 B"}, {1023, "1023 B"}, {1024, "1 KB"},
		{1536, "2 KB"}, {1024 * 1024, "1.0 MB"}, {3 * 1024 * 1024 * 1024, "3.0 GB"},
	}
	for _, tc := range cases {
		if got := formatFileSize(tc.in); got != tc.want {
			t.Fatalf("formatFileSize(%d)=%q want %q", tc.in, got, tc.want)
		}
	}
}
