package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildIndexFileProgressivePublishesSearchableCheckpoint(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "first.jpg"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "second.jpg"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}

	finalPath := filepath.Join(t.TempDir(), "final.index")
	partialPath := filepath.Join(t.TempDir(), "partial.index")
	if _, err := BuildIndexFileProgressive(context.Background(), []string{root}, finalPath, partialPath, 1, nil); err != nil {
		t.Fatal(err)
	}

	partial, err := LoadIndex(partialPath, nil)
	if err != nil {
		t.Fatalf("partial checkpoint is not a valid index: %v", err)
	}
	defer partial.Close()
	if partial.Len() == 0 {
		t.Fatal("partial checkpoint should contain searchable entries")
	}
	r := Search(context.Background(), partial, nil, "*.jpg", FilterFiles)
	if len(r.IDs) == 0 {
		t.Fatal("partial checkpoint should already be searchable")
	}

	final, err := LoadIndex(finalPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer final.Close()
	if final.Len() < partial.Len() {
		t.Fatalf("final index should not be smaller than partial: final=%d partial=%d", final.Len(), partial.Len())
	}
}
