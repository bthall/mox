package cli

import (
	"fmt"
	"time"
)

// relativeTime renders how long ago t was, relative to now, as a short
// human string ("just now", "2m ago", "1h ago", "3d ago"). A zero t (e.g. a
// session that isn't running, so has no activity timestamp) renders "-".
func relativeTime(now, t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	if s := relativeShort(now, t); s != "now" {
		return s + " ago"
	}
	return "just now"
}

// relativeShort is the compact form used in the inline Recent: footer
// ("now", "2m", "1h", "3d"). A zero t renders "-".
func relativeShort(now, t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := now.Sub(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
