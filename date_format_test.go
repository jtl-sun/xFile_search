package main

import (
	"testing"
	"time"
)

func TestFormatModifiedDate(t *testing.T) {
	tm := time.Date(2026, 8, 26, 1, 34, 59, 0, time.Local)
	if got := formatModifiedDate(tm); got != "2026-08-26 01:34" {
		t.Fatalf("got %q", got)
	}
}
