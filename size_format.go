package main

import "fmt"

func formatFileSize(bytes int64) string {
	if bytes < 0 {
		return "—"
	}
	const (
		KB = int64(1024)
		MB = 1024 * KB
		GB = 1024 * MB
		TB = 1024 * GB
	)
	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.1f TB", float64(bytes)/float64(TB))
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%d KB", (bytes+KB-1)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
