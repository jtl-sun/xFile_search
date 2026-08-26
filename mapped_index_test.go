package main

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

func TestMappedIndexLargeSearch(t *testing.T) {
	const n = 100_000
	s := &IndexSnapshot{Entries: make([]Entry, n), Roots: []string{`C:\\`}}
	for i := 0; i < n; i++ {
		p := fmt.Sprintf(`C:\\Archive\\%04d\\design_%06d.jpg`, 2000+i%25, i)
		if i%10_000 == 0 {
			p = fmt.Sprintf(`C:\\Archive\\2019\\turquoise_necklace_%06d.jpg`, i)
		}
		s.Entries[i] = NewEntry(p, false)
	}
	path := filepath.Join(t.TempDir(), "xFile_v2.index")
	if err := SaveIndex(path, s); err != nil {
		t.Fatal(err)
	}
	mapped, err := LoadIndex(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mapped.Close()
	if mapped.Len() != n {
		t.Fatalf("mapped count %d want %d", mapped.Len(), n)
	}
	r := Search(context.Background(), mapped, nil, "turquoise necklace", FilterAll)
	if got, want := len(r.IDs), n/10_000; got != want {
		t.Fatalf("matches %d want %d", got, want)
	}
}
