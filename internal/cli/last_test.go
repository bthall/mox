package cli

import (
	"testing"
	"time"

	"github.com/bthall/mox/internal/history"
)

func TestPickLast(t *testing.T) {
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	entries := []history.Entry{
		{Name: "current", Action: history.ActionAttached, Time: now},
		{Name: "previous", Action: history.ActionAttached, Time: now.Add(-time.Hour)},
		{Name: "older", Action: history.ActionCreated, Time: now.Add(-2 * time.Hour)},
	}

	if got := pickLast(entries, "current"); got != "previous" {
		t.Errorf("inside 'current': got %q, want previous", got)
	}
	// Outside tmux (no current session) the newest entry wins.
	if got := pickLast(entries, ""); got != "current" {
		t.Errorf("no current: got %q, want current", got)
	}
	if got := pickLast(nil, "x"); got != "" {
		t.Errorf("empty history: got %q, want empty", got)
	}
	// Only entry is the session we're already in.
	if got := pickLast(entries[:1], "current"); got != "" {
		t.Errorf("only-current history: got %q, want empty", got)
	}
}
