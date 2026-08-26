package main

import (
	"testing"
	"time"
)

func TestFormatResultDate(t *testing.T) {
	d := time.Date(2026, 8, 25, 18, 34, 59, 0, time.Local)
	if got := formatResultDate(d); got != "2026-08-25 18:34" {
		t.Fatalf("got %q", got)
	}
}
