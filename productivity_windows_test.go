//go:build windows

package main

import "testing"

func TestDirectoryScopeLabel(t *testing.T) {
	cases := []struct {
		raw  string
		want string
		ok   bool
	}{
		{`D:\pg\wk\PO\702-LIW\_Request Samples\`, `Scope: D:\pg\wk\PO\702-LIW\_Request Samples\   |   Subfolders: ON   |   Clear: Clear button`, true},
		{`"D:\Old Designs\"`, `Scope: D:\Old Designs\   |   Subfolders: ON   |   Clear: Clear button`, true},
		{`D:\pg\wk\PO\702-LIW\_Request Samples`, ``, false},
		{`*.jpg`, ``, false},
	}
	for _, tc := range cases {
		got, ok := directoryScopeLabel(tc.raw)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("directoryScopeLabel(%q) = (%q, %v), want (%q, %v)", tc.raw, got, ok, tc.want, tc.ok)
		}
	}
}

func TestCopyUTF16FixedTruncatesAndTerminates(t *testing.T) {
	buf := make([]uint16, 5)
	copyUTF16Fixed(buf, "abcdef")
	if buf[4] != 0 {
		t.Fatalf("expected NUL termination after truncation, got %#v", buf)
	}
}
