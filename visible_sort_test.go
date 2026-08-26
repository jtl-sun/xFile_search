package main

import "testing"

func TestSortVisibleItemsByNameToggle(t *testing.T) {
	base := []visibleSortItem{
		{ID: 1, Name: "z.JPG", Path: `D:\b`, SizeBytes: 3000, SizeKnown: true, Date: "2026-08-25 10:00"},
		{ID: 2, Name: "A.jpg", Path: `D:\c`, SizeBytes: 1000, SizeKnown: true, Date: "2026-08-24 10:00"},
		{ID: 3, Name: "m.jpg", Path: `D:\a`, SizeBytes: 2000, SizeKnown: true, Date: "2026-08-26 10:00"},
	}
	sortVisibleItems(base, 0, true)
	if got := []uint32{base[0].ID, base[1].ID, base[2].ID}; got[0] != 2 || got[1] != 3 || got[2] != 1 {
		t.Fatalf("ascending name sort = %v", got)
	}
	sortVisibleItems(base, 0, false)
	if got := []uint32{base[0].ID, base[1].ID, base[2].ID}; got[0] != 1 || got[1] != 3 || got[2] != 2 {
		t.Fatalf("descending name sort = %v", got)
	}
}

func TestSortVisibleItemsColumns(t *testing.T) {
	items := []visibleSortItem{
		{ID: 1, Name: "b", Path: `D:\z`, SizeBytes: 3000, SizeKnown: true, Date: "2026-01-01 10:00"},
		{ID: 2, Name: "a", Path: `C:\a`, SizeBytes: 1000, SizeKnown: true, Date: "2026-03-01 10:00"},
		{ID: 3, Name: "c", Path: `E:\m`, SizeBytes: 2000, SizeKnown: true, Date: "2026-02-01 10:00"},
	}
	sortVisibleItems(items, 1, true)
	if items[0].ID != 2 || items[1].ID != 1 || items[2].ID != 3 {
		t.Fatalf("path order: %+v", items)
	}
	sortVisibleItems(items, 2, true)
	if items[0].ID != 2 || items[1].ID != 3 || items[2].ID != 1 {
		t.Fatalf("size order: %+v", items)
	}
	sortVisibleItems(items, 3, false)
	if items[0].ID != 2 || items[1].ID != 3 || items[2].ID != 1 {
		t.Fatalf("date order: %+v", items)
	}
}

func TestSortVisibleItemsMissingMetadataAlwaysLast(t *testing.T) {
	for _, asc := range []bool{true, false} {
		items := []visibleSortItem{{ID: 1, Name: "a", SizeKnown: false, Date: "…"}, {ID: 2, Name: "b", SizeBytes: 10, SizeKnown: true, Date: "2026-01-01 10:00"}}
		sortVisibleItems(items, 2, asc)
		if items[1].ID != 1 {
			t.Fatalf("missing size should be last, asc=%v: %+v", asc, items)
		}
		sortVisibleItems(items, 3, asc)
		if items[1].ID != 1 {
			t.Fatalf("missing date should be last, asc=%v: %+v", asc, items)
		}
	}
}
