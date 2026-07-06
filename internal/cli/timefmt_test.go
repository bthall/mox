package cli

import (
	"testing"
	"time"
)

func TestRelativeTime(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		t    time.Time
		want string
	}{
		{"zero", time.Time{}, "-"},
		{"just now", now.Add(-10 * time.Second), "just now"},
		{"minutes", now.Add(-2 * time.Minute), "2m ago"},
		{"hours", now.Add(-3 * time.Hour), "3h ago"},
		{"days", now.Add(-50 * time.Hour), "2d ago"},
		{"future clamps", now.Add(time.Hour), "just now"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := relativeTime(now, c.t); got != c.want {
				t.Errorf("relativeTime = %q, want %q", got, c.want)
			}
		})
	}
}

func TestRelativeShort(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		t    time.Time
		want string
	}{
		{time.Time{}, "-"},
		{now.Add(-30 * time.Second), "now"},
		{now.Add(-5 * time.Minute), "5m"},
		{now.Add(-2 * time.Hour), "2h"},
		{now.Add(-72 * time.Hour), "3d"},
	}
	for _, c := range cases {
		if got := relativeShort(now, c.t); got != c.want {
			t.Errorf("relativeShort(%v) = %q, want %q", c.t, got, c.want)
		}
	}
}
