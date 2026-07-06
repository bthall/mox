package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/bthall/mox/internal/history"
)

func TestRenderRecent(t *testing.T) {
	entries := []history.Entry{
		{Name: "dev", Action: history.ActionAttached, Time: fixedNow.Add(-2 * time.Minute)},
		{Name: "analytics", Action: history.ActionCreated, Time: fixedNow.Add(-time.Hour)},
		{Name: "legacy-db", Action: history.ActionCreated, Time: fixedNow.Add(-48 * time.Hour)},
	}
	running := map[string]bool{"dev": true, "analytics": true}

	var buf bytes.Buffer
	renderRecent(&buf, entries, running, fixedNow, 10)
	out := buf.String()

	header := lineContaining(t, out, "SESSION")
	for _, col := range []string{"LAST ACTION", "WHEN", "STATE"} {
		if !strings.Contains(header, col) {
			t.Errorf("header missing %q: %q", col, header)
		}
	}

	devLine := lineContaining(t, out, "dev")
	for _, want := range []string{"attached", "2m ago", "running"} {
		if !strings.Contains(devLine, want) {
			t.Errorf("dev line missing %q: %q", want, devLine)
		}
	}

	legacyLine := lineContaining(t, out, "legacy-db")
	for _, want := range []string{"created", "2d ago", "gone"} {
		if !strings.Contains(legacyLine, want) {
			t.Errorf("legacy line missing %q: %q", want, legacyLine)
		}
	}
}

func TestRenderRecent_LimitApplied(t *testing.T) {
	entries := []history.Entry{
		{Name: "a", Action: history.ActionCreated, Time: fixedNow.Add(-1 * time.Minute)},
		{Name: "b", Action: history.ActionCreated, Time: fixedNow.Add(-2 * time.Minute)},
		{Name: "c", Action: history.ActionCreated, Time: fixedNow.Add(-3 * time.Minute)},
	}
	var buf bytes.Buffer
	renderRecent(&buf, entries, nil, fixedNow, 2)
	out := buf.String()

	if strings.Contains(out, "\nc") || strings.Contains(out, " c ") {
		t.Errorf("limit=2 should drop third entry:\n%s", out)
	}
}

func TestRenderRecent_Empty(t *testing.T) {
	var buf bytes.Buffer
	renderRecent(&buf, nil, nil, fixedNow, 10)
	if !strings.Contains(buf.String(), "No recent sessions.") {
		t.Errorf("missing empty notice: %q", buf.String())
	}
}
