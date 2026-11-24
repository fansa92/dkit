package cli

import (
	"fmt"
	"io"
	"time"
)

// HumanDuration renders a signed duration the way a person would say it:
// "2h ago", "in 3d", "now". Used for token and certificate expiry.
func HumanDuration(d time.Duration) string {
	neg := d < 0
	if neg {
		d = -d
	}
	var s string
	switch {
	case d < time.Second:
		return "now"
	case d < time.Minute:
		s = fmt.Sprintf("%ds", int64(d.Seconds()))
	case d < time.Hour:
		s = fmt.Sprintf("%dm", int64(d.Minutes()))
	case d < 24*time.Hour:
		s = fmt.Sprintf("%dh", int64(d.Hours()))
	case d < 30*24*time.Hour:
		s = fmt.Sprintf("%dd", int64(d.Hours())/24)
	case d < 365*24*time.Hour:
		s = fmt.Sprintf("%dmo", int64(d.Hours())/24/30)
	default:
		s = fmt.Sprintf("%dy", int64(d.Hours())/24/365)
	}
	if neg {
		return s + " ago"
	}
	return "in " + s
}

// KV prints an aligned two-column block, indented by two spaces.
func KV(w io.Writer, rows [][2]string) {
	width := 0
	for _, r := range rows {
		if len(r[0]) > width {
			width = len(r[0])
		}
	}
	for _, r := range rows {
		fmt.Fprintf(w, "  %s%s  %s\n", Dim(r[0]), spaces(width-len(r[0])), r[1])
	}
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}
