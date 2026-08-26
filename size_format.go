package main

import "fmt"

func formatFileSize(n int64) string {
	if n < 0 { return "—" }
	const unit = int64(1024)
	if n < unit { return fmt.Sprintf("%d B", n) }
	div, exp := unit, 0
	for q := n / unit; q >= unit && exp < 3; q /= unit { div *= unit; exp++ }
	value := float64(n) / float64(div)
	units := []string{"KB", "MB", "GB", "TB"}
	if value >= 10 || value == float64(int64(value)) { return fmt.Sprintf("%.0f %s", value, units[exp]) }
	return fmt.Sprintf("%.1f %s", value, units[exp])
}
