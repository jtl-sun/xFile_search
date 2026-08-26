package main

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestRememberSearchHistoryNewestFirstDedup(t *testing.T) {
	h := []string{`D:\\*.jpg`, `S:\\my_programs`, `*.pdf`}
	got := rememberSearchHistory(h, `d:\\*.JPG`, 5)
	want := []string{`d:\\*.JPG`, `S:\\my_programs`, `*.pdf`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestRememberSearchHistoryLimit(t *testing.T) {
	h := []string{"b", "c", "d"}
	got := rememberSearchHistory(h, "a", 3)
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestSearchHistorySaveLoad(t *testing.T) {
	p := filepath.Join(t.TempDir(), "SearchHistory.txt")
	want := []string{`D:\\*.jpg`, `F:\\old photos\\*.png`, `ring *.xlsx`}
	if err := saveSearchHistory(p, want); err != nil {
		t.Fatal(err)
	}
	got := loadSearchHistory(p, 30)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestMenuSafeHistoryLabel(t *testing.T) {
	got := menuSafeHistoryLabel("A&B")
	if got != "A&&B" {
		t.Fatalf("got %q", got)
	}
}
