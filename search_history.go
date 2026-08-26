package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxSearchHistory          = 30
	maxSearchHistoryMenu      = 15
	searchHistoryCommandBase  = 50000
	searchHistoryClearCommand = 50999
)

func loadSearchHistory(path string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	out := make([]string, 0, limit)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		q := strings.TrimSpace(sc.Text())
		if q == "" || containsHistoryFold(out, q) {
			continue
		}
		out = append(out, q)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func rememberSearchHistory(history []string, query string, limit int) []string {
	query = strings.TrimSpace(query)
	if query == "" || limit <= 0 {
		return history
	}
	out := make([]string, 0, minInt(limit, len(history)+1))
	out = append(out, query)
	for _, old := range history {
		old = strings.TrimSpace(old)
		if old == "" || strings.EqualFold(old, query) || containsHistoryFold(out, old) {
			continue
		}
		out = append(out, old)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func saveSearchHistory(path string, history []string) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	for _, q := range history {
		q = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(q, "\r", " "), "\n", " "))
		if q == "" {
			continue
		}
		b.WriteString(q)
		b.WriteString("\r\n")
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return err
	}
	_ = os.Remove(path)
	return os.Rename(tmp, path)
}

func containsHistoryFold(items []string, q string) bool {
	for _, v := range items {
		if strings.EqualFold(strings.TrimSpace(v), strings.TrimSpace(q)) {
			return true
		}
	}
	return false
}

func menuSafeHistoryLabel(q string) string {
	q = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(q, "\r", " "), "\n", " "))
	if len([]rune(q)) > 120 {
		r := []rune(q)
		q = string(r[:117]) + "..."
	}
	return strings.ReplaceAll(q, "&", "&&")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
