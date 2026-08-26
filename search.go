package main

import (
	"context"
	"runtime"
	"sync"
	"time"
)

func Search(ctx context.Context, snap *IndexSnapshot, base []uint32, raw string, mode FilterMode) SearchResult {
	started := time.Now()
	if snap == nil || ParseQuery(raw).Empty() {
		return SearchResult{Query: raw, Elapsed: time.Since(started)}
	}
	q := ParseQuery(raw)

	total := snap.Len()
	if base != nil {
		total = len(base)
	}
	workers := runtime.GOMAXPROCS(0)
	// Search stays quick while leaving enough CPU for the Windows message loop
	// and the user's other applications.
	if workers > 6 {
		workers = 6
	}
	if total < 300_000 || workers < 2 {
		ids, scanned, canceled := searchSequential(ctx, snap, base, q, mode)
		return SearchResult{IDs: ids, Query: raw, Elapsed: time.Since(started), Scanned: scanned, Canceled: canceled}
	}

	if workers > total/100_000 {
		workers = total / 100_000
		if workers < 2 {
			workers = 2
		}
	}
	parts := make([][]uint32, workers)
	scannedParts := make([]int, workers)
	var wg sync.WaitGroup
	wg.Add(workers)

	for w := 0; w < workers; w++ {
		w := w
		start := total * w / workers
		end := total * (w + 1) / workers
		go func() {
			defer wg.Done()
			out := make([]uint32, 0, (end-start)/32+8)
			scanned := 0
			for i := start; i < end; i++ {
				if scanned&4095 == 0 {
					select {
					case <-ctx.Done():
						return
					default:
					}
				}
				var id uint32
				if base == nil {
					id = uint32(i)
				} else {
					id = base[i]
				}
				scanned++
				if snap.MatchAt(id, q, mode) {
					out = append(out, id)
				}
			}
			parts[w] = out
			scannedParts[w] = scanned
		}()
	}
	wg.Wait()
	canceled := false
	select {
	case <-ctx.Done():
		canceled = true
	default:
	}

	totalMatches := 0
	scanned := 0
	for i := range parts {
		totalMatches += len(parts[i])
		scanned += scannedParts[i]
	}
	ids := make([]uint32, 0, totalMatches)
	for _, p := range parts {
		ids = append(ids, p...)
	}
	return SearchResult{IDs: ids, Query: raw, Elapsed: time.Since(started), Scanned: scanned, Canceled: canceled}
}

func searchSequential(ctx context.Context, snap *IndexSnapshot, base []uint32, q Query, mode FilterMode) ([]uint32, int, bool) {
	n := snap.Len()
	if base != nil {
		n = len(base)
	}
	out := make([]uint32, 0, n/32+8)
	scanned := 0
	for i := 0; i < n; i++ {
		if i&4095 == 0 {
			select {
			case <-ctx.Done():
				return nil, scanned, true
			default:
			}
		}
		var id uint32
		if base == nil {
			id = uint32(i)
		} else {
			id = base[i]
		}
		scanned++
		if snap.MatchAt(id, q, mode) {
			out = append(out, id)
		}
	}
	return out, scanned, false
}
