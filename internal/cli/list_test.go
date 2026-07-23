package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bthall/mox/internal/history"
	"github.com/bthall/mox/internal/session"
)

// fixedNow is the reference time for render tests. Writing to a bytes.Buffer
// (not an *os.File) disables color, so output is deterministic and uncolored.
var fixedNow = time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)

func lineContaining(t *testing.T, out, needle string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	t.Fatalf("no line containing %q in:\n%s", needle, out)
	return ""
}

func TestRenderList(t *testing.T) {
	infos := []session.SessionInfo{
		{Name: "dev", Running: true, Managed: true, Windows: 3, Attached: true,
			LastActivity: fixedNow.Add(-2 * time.Minute), Hosts: []string{"web1", "web2", "db"}},
		{Name: "prod", Running: false, Managed: true, Hosts: []string{"p1"}},
		{Name: "scratch", Running: true, Managed: false, Windows: 1,
			LastActivity: fixedNow.Add(-72 * time.Hour)},
	}
	recent := []history.Entry{
		{Name: "dev", Action: history.ActionAttached, Time: fixedNow.Add(-2 * time.Minute)},
		{Name: "analytics", Action: history.ActionCreated, Time: fixedNow.Add(-time.Hour)},
	}

	var buf bytes.Buffer
	renderList(&buf, infos, recent, fixedNow)
	out := buf.String()

	header := lineContaining(t, out, "NAME")
	for _, col := range []string{"ORIGIN", "STATE", "WIN", "ACTIVITY", "HOSTS"} {
		if !strings.Contains(header, col) {
			t.Errorf("header missing %q: %q", col, header)
		}
	}

	devLine := lineContaining(t, out, "dev ")
	for _, want := range []string{"mox", "running", "attached", "3", "2m ago", "web1, web2, db"} {
		if !strings.Contains(devLine, want) {
			t.Errorf("dev line missing %q: %q", want, devLine)
		}
	}

	prodLine := lineContaining(t, out, "prod")
	if !strings.Contains(prodLine, "stopped") {
		t.Errorf("prod should be stopped: %q", prodLine)
	}

	scratchLine := lineContaining(t, out, "scratch")
	for _, want := range []string{"tmux", "running", "3d ago"} {
		if !strings.Contains(scratchLine, want) {
			t.Errorf("scratch line missing %q: %q", want, scratchLine)
		}
	}

	if footer := lineContaining(t, out, "Recent:"); footer != "Recent: dev (attached 2m) · analytics (created 1h)" {
		t.Errorf("unexpected footer: %q", footer)
	}
	if summary := lineContaining(t, out, "configured"); summary != "2 configured · 1 running · 1 unmanaged" {
		t.Errorf("unexpected summary: %q", summary)
	}
}

func TestRenderList_Empty(t *testing.T) {
	var buf bytes.Buffer
	renderList(&buf, nil, nil, fixedNow)
	out := buf.String()

	if !strings.Contains(out, "No sessions configured or running.") {
		t.Errorf("missing empty notice:\n%s", out)
	}
	if !strings.Contains(out, "Recent: (none)") {
		t.Errorf("missing empty recent footer:\n%s", out)
	}
	if !strings.Contains(out, "0 configured · 0 running") {
		t.Errorf("missing empty summary:\n%s", out)
	}
}

// TestNameCellGlyphs pins the colored status glyphs: ● for a running mox
// session, ◆ for a running tmux-only session, ○ stopped. /dev/null is a
// char device, so useColor treats it as a color-capable terminal.
func TestNameCellGlyphs(t *testing.T) {
	tty, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tty.Close() }()

	cases := []struct {
		info  session.SessionInfo
		glyph string
		code  string
	}{
		{session.SessionInfo{Name: "m", Running: true, Managed: true}, "●", ansiGreen},
		{session.SessionInfo{Name: "u", Running: true, Managed: false}, "◆", ansiYellow},
		{session.SessionInfo{Name: "s", Managed: true}, "○", ansiDim},
	}
	for _, c := range cases {
		got := nameCell(tty, c.info)
		if !strings.Contains(got, c.code+c.glyph) {
			t.Errorf("nameCell(%+v) = %q, want glyph %q in color %q", c.info, got, c.glyph, c.code)
		}
	}
}

func TestRenderList_HostsTruncated(t *testing.T) {
	long := []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf"}
	infos := []session.SessionInfo{
		{Name: "big", Running: true, Managed: true, Windows: 1, Hosts: long},
	}
	var buf bytes.Buffer
	renderList(&buf, infos, nil, fixedNow)
	line := lineContaining(t, buf.String(), "big")
	if !strings.Contains(line, "…") {
		t.Errorf("expected truncated hosts with ellipsis: %q", line)
	}
}
