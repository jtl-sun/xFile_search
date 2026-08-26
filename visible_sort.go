package main

import (
	"sort"
	"strings"
)

// visibleSortItem is intentionally small: xFile_search sorts only the rows
// currently materialized in the ListView (MaxDisplayResults), not millions of
// memory-mapped records. Size/Date values are loaded in the background.
type visibleSortItem struct {
	ID        uint32
	Name      string
	Path      string
	SizeText  string
	SizeBytes int64
	SizeKnown bool
	Date      string
}

func sortVisibleItems(items []visibleSortItem, column int, ascending bool) {
	if len(items) < 2 || column < 0 || column > 3 {
		return
	}
	cmpFold := func(a, b string) int {
		return strings.Compare(strings.ToLower(a), strings.ToLower(b))
	}
	compareTie := func(a, b visibleSortItem) int {
		if c := cmpFold(a.Name, b.Name); c != 0 {
			return c
		}
		if c := cmpFold(a.Path, b.Path); c != 0 {
			return c
		}
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		var c int
		switch column {
		case 0:
			c = cmpFold(a.Name, b.Name)
		case 1:
			c = cmpFold(a.Path, b.Path)
		case 2:
			// Unknown sizes remain at the bottom until background metadata arrives.
			if a.SizeKnown != b.SizeKnown {
				return a.SizeKnown
			}
			if a.SizeKnown {
				if a.SizeBytes < b.SizeBytes {
					c = -1
				} else if a.SizeBytes > b.SizeBytes {
					c = 1
				}
			}
		case 3:
			aMissing := a.Date == "" || a.Date == "…" || a.Date == "—"
			bMissing := b.Date == "" || b.Date == "…" || b.Date == "—"
			if aMissing != bMissing {
				return !aMissing
			}
			c = cmpFold(a.Date, b.Date)
		}
		if c == 0 {
			c = compareTie(a, b)
		}
		if ascending {
			return c < 0
		}
		return c > 0
	})
}
