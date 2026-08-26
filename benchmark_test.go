package main

import (
	"context"
	"fmt"
	"testing"
)

func BenchmarkSearch1M(b *testing.B) {
	s := &IndexSnapshot{Entries: make([]Entry, 1_000_000)}
	for i := 0; i < len(s.Entries); i++ {
		name := fmt.Sprintf(`/archive/%04d/design_%07d.jpg`, i%20+2000, i)
		if i%5000 == 0 {
			name = fmt.Sprintf(`/archive/2019/turquoise_necklace_%07d.jpg`, i)
		}
		s.Entries[i] = NewEntry(name, false)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Search(context.Background(), s, nil, "turquoise necklace", FilterAll)
	}
}
