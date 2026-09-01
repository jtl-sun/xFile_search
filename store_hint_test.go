package main

import (
	"path/filepath"
	"testing"
)

func TestReadIndexCountHint(t *testing.T) {
	s := testSnapshot()
	p := filepath.Join(t.TempDir(), "count.index")
	if err := SaveIndex(p, s); err != nil {
		t.Fatal(err)
	}
	if got := readIndexCountHint(p); got != len(s.Entries) {
		t.Fatalf("got %d want %d", got, len(s.Entries))
	}
	if got := readIndexCountHint(filepath.Join(t.TempDir(), "missing.index")); got != 0 {
		t.Fatalf("missing index hint = %d, want 0", got)
	}
}
