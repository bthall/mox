package cli

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bthall/mox/internal/config"
	"github.com/bthall/mox/internal/session"
)

var testNow = time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)

func newTestModel(t *testing.T) pickerModel {
	t.Helper()
	sessions := map[string]*config.Session{
		"dev": {Hosts: []string{"web1", "web2", "db"}, Sync: true, Arrange: "tiled"},
		"web": {Connect: "ssh -p 2222 ops@{{host}}", Hosts: []string{"host1", "host2"}},
	}
	m := newPickerModel(orderPickerCandidates(pickerFixtures(), nil), sessions, testNow)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return next.(pickerModel)
}

func send(t *testing.T, m pickerModel, keys ...tea.KeyMsg) pickerModel {
	t.Helper()
	for _, k := range keys {
		next, _ := m.Update(k)
		m = next.(pickerModel)
	}
	return m
}

func runes(s string) []tea.KeyMsg {
	msgs := make([]tea.KeyMsg, 0, len(s))
	for _, r := range s {
		msgs = append(msgs, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return msgs
}

func manyCandidates(n int) []session.SessionInfo {
	out := make([]session.SessionInfo, n)
	for i := range out {
		out[i] = session.SessionInfo{Name: fmt.Sprintf("sess-%d", i), Managed: true}
	}
	return out
}

var (
	keyEnter = tea.KeyMsg{Type: tea.KeyEnter}
	keyEsc   = tea.KeyMsg{Type: tea.KeyEsc}
	keyDown  = tea.KeyMsg{Type: tea.KeyDown}
	keyUp    = tea.KeyMsg{Type: tea.KeyUp}
	keyCtrlC = tea.KeyMsg{Type: tea.KeyCtrlC}
	keyBksp  = tea.KeyMsg{Type: tea.KeyBackspace}
)

func TestPickerModel_EnterAcceptsTopCandidate(t *testing.T) {
	// Candidate order: dev, web, batch, old-thing (running by activity first).
	m := send(t, newTestModel(t), keyEnter)
	if m.choice != "dev" {
		t.Errorf("bare Enter should pick the top candidate, got %q", m.choice)
	}
}

func TestPickerModel_TypeToFilterThenEnter(t *testing.T) {
	m := send(t, newTestModel(t), append(runes("web"), keyEnter)...)
	if m.choice != "web" {
		t.Errorf("typed filter should select web, got %q", m.choice)
	}
	// Fuzzy subsequence: 'ot' matches old-thing.
	m = send(t, newTestModel(t), append(runes("ot"), keyEnter)...)
	if m.choice != "old-thing" {
		t.Errorf("fuzzy filter should select old-thing, got %q", m.choice)
	}
}

func TestPickerModel_ArrowsMoveWithoutWrapping(t *testing.T) {
	m := send(t, newTestModel(t), keyDown, keyEnter)
	if m.choice != "web" {
		t.Errorf("down+enter should pick the second candidate, got %q", m.choice)
	}
	// No wrap: up from the top stays on the top.
	m = send(t, newTestModel(t), keyUp, keyUp, keyEnter)
	if m.choice != "dev" {
		t.Errorf("up at top should clamp, got %q", m.choice)
	}
	// No wrap: down past the end stays on the last.
	m = send(t, newTestModel(t), keyDown, keyDown, keyDown, keyDown, keyDown, keyEnter)
	if m.choice != "old-thing" {
		t.Errorf("down past end should clamp to last, got %q", m.choice)
	}
}

func TestPickerModel_BackspaceRefilters(t *testing.T) {
	m := send(t, newTestModel(t), append(runes("webz"), keyBksp, keyEnter)...)
	if m.choice != "web" {
		t.Errorf("backspace should refilter, got %q", m.choice)
	}
}

func TestPickerModel_EnterOnNoMatchCancels(t *testing.T) {
	m := send(t, newTestModel(t), append(runes("zzz"), keyEnter)...)
	if m.choice != "" {
		t.Errorf("enter with no matches should quit with no choice, got %q", m.choice)
	}
}

func TestPickerModel_EscClearsFilterThenCancels(t *testing.T) {
	m := send(t, newTestModel(t), append(runes("web"), keyEsc)...)
	if len(m.query) != 0 {
		t.Errorf("first esc should clear the query, still %q", string(m.query))
	}
	m = send(t, m, keyEnter)
	if m.choice != "dev" {
		t.Errorf("after clearing, enter should pick the top of the full list, got %q", m.choice)
	}
	m2 := send(t, newTestModel(t), keyEsc)
	if m2.choice != "" {
		t.Errorf("esc on unfiltered list should cancel, got %q", m2.choice)
	}
}

func TestPickerModel_CtrlCCancels(t *testing.T) {
	m := send(t, newTestModel(t), append(runes("we"), keyCtrlC)...)
	if m.choice != "" {
		t.Errorf("ctrl-c should cancel, got %q", m.choice)
	}
}

func TestPickerModel_ViewTwoPane(t *testing.T) {
	view := newTestModel(t).View()
	for _, want := range []string{
		"sessions", // left title
		"▸",        // search prompt
		"dev",      // top candidate (also right pane title)
		"state",    // preview keys
		"running",  // preview state
		"web1",     // preview hosts from config
		"sync",     // config detail
		"↵ attach", // footer hints
		"╭─", "╰─", // panel borders
	} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
}

func TestPickerModel_ViewPreviewFollowsSelection(t *testing.T) {
	m := send(t, newTestModel(t), keyDown) // select web
	view := m.View()
	if !strings.Contains(view, "ssh -p 2222 ops@{{host}}") {
		t.Errorf("preview should show web's connect template:\n%s", view)
	}
}

func TestPickerModel_ViewNarrowDropsPreview(t *testing.T) {
	m := newTestModel(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 34, Height: 30})
	view := next.(pickerModel).View()
	if strings.Contains(view, "state") {
		t.Errorf("narrow view should drop the preview pane:\n%s", view)
	}
	if !strings.Contains(view, "dev") {
		t.Errorf("narrow view should still list sessions:\n%s", view)
	}
}

func TestPickerModel_ScrollKeepsSelectionVisible(t *testing.T) {
	// 30 candidates against a short terminal forces scrolling.
	m := newPickerModel(manyCandidates(30), nil, testNow)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 12})
	m = next.(pickerModel)
	for i := 0; i < 29; i++ {
		m = send(t, m, keyDown)
	}
	if m.selected != 29 {
		t.Fatalf("selected = %d, want 29", m.selected)
	}
	if m.offset == 0 {
		t.Errorf("offset should have scrolled, still 0")
	}
	m = send(t, m, keyEnter)
	if m.choice != "sess-29" {
		t.Errorf("scrolled enter should pick sess-29, got %q", m.choice)
	}
}

func TestPickerCtrlERequestsEdit(t *testing.T) {
	candidates := []session.SessionInfo{
		{Name: "managed", Managed: true},
		{Name: "foreign", Running: true, Managed: false},
	}
	sessions := map[string]*config.Session{"managed": {Root: "/tmp"}}
	m := newPickerModel(candidates, sessions, time.Now())

	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	pm := nm.(pickerModel)
	if !isQuit(cmd) || pm.choice != "managed" || !pm.editReq {
		t.Fatalf("ctrl+e on managed: choice=%q editReq=%v", pm.choice, pm.editReq)
	}

	// on a session with no config entry, ctrl+e is a no-op
	m = newPickerModel(candidates, sessions, time.Now())
	m.selected = 1
	nm, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	pm = nm.(pickerModel)
	if cmd != nil || pm.editReq {
		t.Fatal("ctrl+e acted on an unmanaged session")
	}
}
