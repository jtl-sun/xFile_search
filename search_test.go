package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testSnapshot() *IndexSnapshot {
	paths := []struct {
		p string
		d bool
	}{
		{`C:\\Design\\2019\\turquoise_necklace.jpg`, false},
		{`C:\\Design\\2020\\turquoise_ring.png`, false},
		{`C:\\Design\\2020\\ring_front.jpg`, false},
		{`C:\\Costing\\Dillards\\costing.xlsx`, false},
		{`C:\\Design\\2019`, true},
		{`S:\\Archive\\old_necklace.jpg`, false},
		{`S:\\Archive\\notes.txt`, false},
		{`D:\\Video\\sample.mpg`, false},
	}
	s := &IndexSnapshot{BuiltAt: time.Now(), Roots: []string{`C:\\`, `S:\\`, `D:\\`}}
	for _, x := range paths {
		s.Entries = append(s.Entries, NewEntry(x.p, x.d))
	}
	return s
}

func TestSearchTermsAndExtension(t *testing.T) {
	s := testSnapshot()
	r := Search(context.Background(), s, nil, "turquoise ext:jpg", FilterAll)
	if len(r.IDs) != 1 {
		t.Fatalf("expected 1 result, got %d", len(r.IDs))
	}
	if got := s.Entries[r.IDs[0]].Name(); got != "turquoise_necklace.jpg" {
		t.Fatalf("unexpected %s", got)
	}
}

func TestDriveScopedExtensionGlob(t *testing.T) {
	s := testSnapshot()
	cases := []struct {
		query string
		want  int
	}{
		{`C:\\*.jpg`, 2},
		{`S:\\*.jpg`, 1},
		{`D:\\*.mpg`, 1},
		{`C:\\Design\\2019\\*.jpg`, 1},
		{`C:\\Design\\*.png`, 1},
	}
	for _, tc := range cases {
		r := Search(context.Background(), s, nil, tc.query, FilterAll)
		if len(r.IDs) != tc.want {
			t.Fatalf("%s: expected %d, got %d", tc.query, tc.want, len(r.IDs))
		}
	}
}

func TestDriveScopedPlainTerm(t *testing.T) {
	s := testSnapshot()
	r := Search(context.Background(), s, nil, `S:\\jpg`, FilterAll)
	if len(r.IDs) != 1 {
		t.Fatalf("expected 1 result, got %d", len(r.IDs))
	}
	if got := s.Entries[r.IDs[0]].Name(); got != "old_necklace.jpg" {
		t.Fatalf("unexpected %s", got)
	}
}

func TestDriveScopedFilenameGlob(t *testing.T) {
	s := testSnapshot()
	r := Search(context.Background(), s, nil, `C:\\ring*.jpg`, FilterAll)
	if len(r.IDs) != 1 {
		t.Fatalf("expected 1 result, got %d", len(r.IDs))
	}
	if got := s.Entries[r.IDs[0]].Name(); got != "ring_front.jpg" {
		t.Fatalf("unexpected %s", got)
	}
}

func TestQuotedPathGlob(t *testing.T) {
	q := ParseQuery(`"C:\\Old Designs\\*.jpg"`)
	if len(q.PathPrefixes) != 1 || q.PathPrefixes[0] != `c:\\old designs\\` {
		t.Fatalf("unexpected path prefixes: %#v", q.PathPrefixes)
	}
	if _, ok := q.Extensions["jpg"]; !ok {
		t.Fatalf("jpg extension not parsed")
	}
}

func TestSearchWithin(t *testing.T) {
	s := testSnapshot()
	r1 := Search(context.Background(), s, nil, "turquoise", FilterAll)
	if len(r1.IDs) != 2 {
		t.Fatalf("expected 2, got %d", len(r1.IDs))
	}
	r2 := Search(context.Background(), s, r1.IDs, "2019", FilterAll)
	if len(r2.IDs) != 1 {
		t.Fatalf("expected 1, got %d", len(r2.IDs))
	}
}

func TestMappedDriveScopedExtensionGlob(t *testing.T) {
	s := testSnapshot()
	p := filepath.Join(t.TempDir(), "xFile.index")
	if err := SaveIndex(p, s); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadIndex(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Close()
	r := Search(context.Background(), loaded, nil, `S:\\*.jpg`, FilterAll)
	if len(r.IDs) != 1 {
		t.Fatalf("expected 1 result, got %d", len(r.IDs))
	}
	e, ok := loaded.EntryAt(r.IDs[0])
	if !ok || e.Name() != "old_necklace.jpg" {
		t.Fatalf("unexpected mapped result: %#v", e)
	}
}

func TestStoreRoundtrip(t *testing.T) {
	s := testSnapshot()
	p := filepath.Join(t.TempDir(), "xFile.index")
	if err := SaveIndex(p, s); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadIndex(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Close()
	if loaded.Len() != len(s.Entries) {
		t.Fatalf("count mismatch: got %d want %d", loaded.Len(), len(s.Entries))
	}
	e, ok := loaded.EntryAt(0)
	if !ok || e.Path != s.Entries[0].Path {
		t.Fatalf("path mismatch")
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal(err)
	}
}
