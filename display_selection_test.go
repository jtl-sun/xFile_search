package main

import (
	"fmt"
	"testing"
	"time"
)

func TestSelectDisplayIDsDirectoryScopeKeepsDescendantFilesVisible(t *testing.T) {
	s := &IndexSnapshot{BuiltAt: time.Now(), Roots: []string{`D:\\`}}
	for i := 0; i < 3000; i++ {
		s.Entries = append(s.Entries, NewEntry(fmt.Sprintf(`D:\\Root\\Folder_%04d`, i), true))
	}
	for i := 0; i < 3000; i++ {
		s.Entries = append(s.Entries, NewEntry(fmt.Sprintf(`D:\\Root\\Folder_%04d\\file_%04d.jpg`, i, i), false))
	}
	ids := make([]uint32, len(s.Entries))
	for i := range ids {
		ids[i] = uint32(i)
	}

	got, mix := selectDisplayIDs(s, ids, 2000, `D:\\Root\\`, FilterAll)
	if len(got) != 2000 {
		t.Fatalf("expected 2000 visible rows, got %d", len(got))
	}
	if !mix.Balanced {
		t.Fatal("expected balanced recursive display")
	}
	if mix.Files != 1500 || mix.Folders != 500 {
		t.Fatalf("unexpected mix: files=%d folders=%d", mix.Files, mix.Folders)
	}
	for i := 0; i < 3; i++ {
		e, ok := s.EntryAt(got[i])
		if !ok || e.IsDir {
			t.Fatalf("row %d should be a descendant file, got %#v", i, e)
		}
	}
}

func TestSelectDisplayIDsFilesModeRemainsFilesOnly(t *testing.T) {
	s := &IndexSnapshot{BuiltAt: time.Now(), Roots: []string{`D:\\`}}
	for i := 0; i < 20; i++ {
		s.Entries = append(s.Entries, NewEntry(fmt.Sprintf(`D:\\Root\\Sub\\file_%02d.txt`, i), false))
	}
	ids := make([]uint32, len(s.Entries))
	for i := range ids {
		ids[i] = uint32(i)
	}
	got, mix := selectDisplayIDs(s, ids, 10, `D:\\Root\\`, FilterFiles)
	if len(got) != 10 || mix.Files != 10 || mix.Folders != 0 || mix.Balanced {
		t.Fatalf("unexpected files-only selection: len=%d mix=%+v", len(got), mix)
	}
}

func TestSelectDisplayIDsNormalSearchPreservesOrder(t *testing.T) {
	s := testSnapshot()
	ids := []uint32{4, 0, 1, 2}
	got, mix := selectDisplayIDs(s, ids, 3, "turquoise", FilterAll)
	want := []uint32{4, 0, 1}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order changed at %d: got %d want %d", i, got[i], want[i])
		}
	}
	if mix.Balanced {
		t.Fatal("normal search must not use balanced recursive display")
	}
}
