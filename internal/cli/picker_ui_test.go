package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// runScripted feeds a keystroke script to the picker UI and returns the
// chosen name. Output goes to a buffer (colors disabled), exercising the
// full render/redraw path.
func runScripted(t *testing.T, script string) string {
	t.Helper()
	var out bytes.Buffer
	ui := newPickerUI(orderPickerCandidates(pickerFixtures(), nil), 80, time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC))
	return ui.run(strings.NewReader(script), &out)
}

func TestPickerUI_EnterAcceptsTopCandidate(t *testing.T) {
	// Candidate order: dev, web, batch, old-thing (running by activity first).
	if got := runScripted(t, "\r"); got != "dev" {
		t.Errorf("bare Enter should pick the top candidate, got %q", got)
	}
}

func TestPickerUI_TypeToFilterThenEnter(t *testing.T) {
	if got := runScripted(t, "web\r"); got != "web" {
		t.Errorf("typed filter should select web, got %q", got)
	}
	// Subsequence match: 'oth' -> old-thing.
	if got := runScripted(t, "oth\r"); got != "old-thing" {
		t.Errorf("subsequence filter should select old-thing, got %q", got)
	}
}

func TestPickerUI_ArrowsMoveSelection(t *testing.T) {
	// Down arrow (ESC [ B) then Enter -> second candidate.
	if got := runScripted(t, "\x1b[B\r"); got != "web" {
		t.Errorf("down+enter should pick second candidate, got %q", got)
	}
	// Ctrl-N behaves like down.
	if got := runScripted(t, "\x0e\r"); got != "web" {
		t.Errorf("Ctrl-N+enter should pick second candidate, got %q", got)
	}
	// Wrap-around: up from the top lands on the last visible row.
	if got := runScripted(t, "\x1b[A\r"); got != "old-thing" {
		t.Errorf("up+enter should wrap to last candidate, got %q", got)
	}
}

func TestPickerUI_BackspaceRefilters(t *testing.T) {
	// "webz" matches nothing; backspace restores "web".
	if got := runScripted(t, "webz\x7f\r"); got != "web" {
		t.Errorf("backspace should refilter, got %q", got)
	}
}

func TestPickerUI_EnterOnNoMatchKeepsRunning(t *testing.T) {
	// Enter with zero matches is ignored; Ctrl-U clears, Enter accepts top.
	if got := runScripted(t, "zzz\r\x15\r"); got != "dev" {
		t.Errorf("enter-on-empty then ctrl-u should recover, got %q", got)
	}
}

func TestPickerUI_EscAndCtrlCCancel(t *testing.T) {
	if got := runScripted(t, "\x1b"); got != "" {
		t.Errorf("lone ESC should cancel, got %q", got)
	}
	if got := runScripted(t, "web\x03"); got != "" {
		t.Errorf("Ctrl-C should cancel, got %q", got)
	}
	// EOF (script exhausted) cancels too.
	if got := runScripted(t, "we"); got != "" {
		t.Errorf("EOF should cancel, got %q", got)
	}
}

func TestPickerUI_RenderShowsPointerAndCounter(t *testing.T) {
	var out bytes.Buffer
	ui := newPickerUI(orderPickerCandidates(pickerFixtures(), nil), 80, time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC))
	ui.run(strings.NewReader("ba"), &out) // filter to batch, then EOF-cancel
	s := out.String()

	if !strings.Contains(s, "> ba") {
		t.Errorf("output should show the typed query, got:\n%q", s)
	}
	if !strings.Contains(s, "1/4") {
		t.Errorf("output should show the match counter 1/4, got:\n%q", s)
	}
}
