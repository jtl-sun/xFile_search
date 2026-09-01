package main

import (
	"strconv"
	"strings"
)

func estimateIndexPercent(count, expected int) int {
	if count <= 0 {
		return 0
	}
	if expected <= 0 {
		return 1
	}
	p := int((uint64(count) * 100) / uint64(expected))
	if p < 1 {
		p = 1
	}
	if p > 99 {
		p = 99
	}
	return p
}

func indexProgressText(percent int) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	return "INDEXING... " + strconv.Itoa(percent) + "%"
}

func parseIndexPercent(msg string) (int, bool) {
	marker := "INDEXING... "
	i := strings.Index(strings.ToUpper(msg), marker)
	if i < 0 {
		return 0, false
	}
	start := i + len(marker)
	end := start
	for end < len(msg) && msg[end] >= '0' && msg[end] <= '9' {
		end++
	}
	if end == start || end >= len(msg) || msg[end] != '%' {
		return 0, false
	}
	n, err := strconv.Atoi(msg[start:end])
	if err != nil || n < 0 || n > 100 {
		return 0, false
	}
	return n, true
}
